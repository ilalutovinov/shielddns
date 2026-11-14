package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"shielddns/internal/mobileconfig"
	"shielddns/internal/model"
	"shielddns/internal/store"
)

func (s *Server) invalidate(r *http.Request, u sess) { store.Invalidate(r.Context(), s.R, u.ClientID) }

func (s *Server) me(w http.ResponseWriter, r *http.Request, u sess) {
	var email, name, mode string
	var logging bool
	var expires *time.Time
	err := s.DB.QueryRow(r.Context(), `
SELECT u.email::text, sub.expires_at, p.name, p.block_mode, p.logging
FROM users u JOIN subscriptions sub ON sub.user_id=u.id JOIN profiles p ON p.user_id=u.id WHERE u.id=$1`, u.UserID).
		Scan(&email, &expires, &name, &mode, &logging)
	if err != nil {
		httpErr(w, 500, "Ошибка сервера")
		return
	}
	var usage int64
	_ = s.DB.QueryRow(r.Context(), `SELECT COALESCE(SUM(queries),0) FROM usage_monthly
		WHERE profile_id=$1 AND month=date_trunc('month', now() AT TIME ZONE 'utc')::date`, u.ProfileID).Scan(&usage)
	if v, err := s.R.Get(r.Context(), fmt.Sprintf("usage:%s:%s", u.ProfileID, time.Now().UTC().Format("200601"))).Int64(); err == nil && v > usage {
		usage = v // счётчик Redis свежее (БД обновляется пачками)
	}
	var limit any
	if u.Plan == "free" {
		limit = model.FreeMonthlyLimit
	}
	host := u.ClientID + ".dns." + s.Cfg.BaseDomain
	resp := map[string]any{
		"email": email, "plan": u.Plan, "planExpires": nil, "usage": usage, "limit": limit,
		"profile":   map[string]any{"clientId": u.ClientID, "name": name, "blockMode": mode, "logging": logging},
		"endpoints": map[string]string{"doh": "https://" + host + "/dns-query", "dot": host, "baseDomain": s.Cfg.BaseDomain},
	}
	if expires != nil && u.Plan == "pro" {
		resp["planExpires"] = expires
	}
	writeJSON(w, 200, resp)
}

func (s *Server) patchProfile(w http.ResponseWriter, r *http.Request, u sess) {
	var in struct {
		Name      *string `json:"name"`
		BlockMode *string `json:"blockMode"`
		Logging   *bool   `json:"logging"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Name != nil && (len(*in.Name) == 0 || len([]rune(*in.Name)) > 40) {
		httpErr(w, 400, "Название: от 1 до 40 символов")
		return
	}
	if in.BlockMode != nil && *in.BlockMode != "null_ip" && *in.BlockMode != "nxdomain" {
		httpErr(w, 400, "blockMode: null_ip или nxdomain")
		return
	}
	if _, err := s.DB.Exec(r.Context(), `UPDATE profiles SET name=COALESCE($2,name), block_mode=COALESCE($3,block_mode),
		logging=COALESCE($4,logging) WHERE id=$1`, u.ProfileID, in.Name, in.BlockMode, in.Logging); err != nil {
		httpErr(w, 500, "Ошибка сервера")
		return
	}
	s.invalidate(r, u)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- фильтры (категории) ----------

func (s *Server) getFilters(w http.ResponseWriter, r *http.Request, u sess) {
	rows, err := s.DB.Query(r.Context(), `
SELECT b.slug, b.category, b.title, b.domain_count,
  EXISTS(SELECT 1 FROM profile_blocklists pb WHERE pb.profile_id=$1 AND pb.blocklist_id=b.id)
FROM blocklists b WHERE b.enabled ORDER BY b.id`, u.ProfileID)
	if err != nil {
		httpErr(w, 500, "Ошибка сервера")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var slug, cat, title string
		var cnt int
		var on bool
		if rows.Scan(&slug, &cat, &title, &cnt, &on) == nil {
			out = append(out, map[string]any{"slug": slug, "category": cat, "title": title, "domains": cnt, "enabled": on})
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) putFilters(w http.ResponseWriter, r *http.Request, u sess) {
	var in struct {
		Lists []string `json:"lists"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		httpErr(w, 500, "Ошибка сервера")
		return
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM profile_blocklists WHERE profile_id=$1`, u.ProfileID); err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO profile_blocklists(profile_id, blocklist_id)
			SELECT $1::uuid, id FROM blocklists WHERE enabled AND slug = ANY($2::text[])`, u.ProfileID, in.Lists)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		httpErr(w, 500, "Ошибка сервера")
		return
	}
	s.invalidate(r, u)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- whitelist / blacklist ----------

func (s *Server) listRules(w http.ResponseWriter, r *http.Request, u sess) {
	rows, err := s.DB.Query(r.Context(), `SELECT id, domain, action FROM custom_rules WHERE profile_id=$1 ORDER BY created_at DESC`, u.ProfileID)
	if err != nil {
		httpErr(w, 500, "Ошибка сервера")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var d, a string
		if rows.Scan(&id, &d, &a) == nil {
			out = append(out, map[string]any{"id": id, "domain": d, "action": a})
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) addRule(w http.ResponseWriter, r *http.Request, u sess) {
	var in struct{ Domain, Action string }
	if !readJSON(w, r, &in) {
		return
	}
	d, ok := normDomain(in.Domain)
	if !ok {
		httpErr(w, 400, "Некорректный домен")
		return
	}
	if in.Action != "allow" && in.Action != "block" {
		httpErr(w, 400, "action: allow или block")
		return
	}
	ctx := r.Context()
	if u.Plan == "free" {
		var n int
		var exists bool
		_ = s.DB.QueryRow(ctx, `SELECT count(*), COALESCE(bool_or(domain=$2), false) FROM custom_rules WHERE profile_id=$1`, u.ProfileID, d).Scan(&n, &exists)
		if !exists && n >= model.MaxFreeRules {
			httpErr(w, 402, "Лимит Free — 10 правил. Перейдите на Pro")
			return
		}
	}
	if _, err := s.DB.Exec(ctx, `INSERT INTO custom_rules(profile_id, domain, action) VALUES($1,$2,$3)
		ON CONFLICT (profile_id, domain) DO UPDATE SET action=EXCLUDED.action`, u.ProfileID, d, in.Action); err != nil {
		httpErr(w, 500, "Ошибка сервера")
		return
	}
	s.invalidate(r, u)
	writeJSON(w, 201, map[string]string{"domain": d, "action": in.Action})
}

func (s *Server) delRule(w http.ResponseWriter, r *http.Request, u sess) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpErr(w, 400, "Некорректный id")
		return
	}
	if _, err = s.DB.Exec(r.Context(), `DELETE FROM custom_rules WHERE id=$1 AND profile_id=$2`, id, u.ProfileID); err != nil {
		httpErr(w, 500, "Ошибка сервера")
		return
	}
	s.invalidate(r, u)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- подписка (мок оплаты), iOS-профиль, привязка IP ----------

func (s *Server) setPlan(w http.ResponseWriter, r *http.Request, u sess) {
	var in struct {
		Plan string `json:"plan"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	var err error
	switch in.Plan {
	case "pro": // заглушка оплаты: просто активируем Pro на 30 дней
		_, err = s.DB.Exec(r.Context(), `UPDATE subscriptions SET plan='pro', started_at=now(), expires_at=now()+interval '30 days'
			WHERE user_id=$1`, u.UserID)
	case "free":
		_, err = s.DB.Exec(r.Context(), `UPDATE subscriptions SET plan='free', started_at=now(), expires_at=NULL WHERE user_id=$1`, u.UserID)
	default:
		httpErr(w, 400, "plan: free или pro")
		return
	}
	if err != nil {
		httpErr(w, 500, "Ошибка сервера")
		return
	}
	s.invalidate(r, u)
	writeJSON(w, 200, map[string]string{"plan": in.Plan})
}

func (s *Server) mobileconfig(w http.ResponseWriter, r *http.Request, u sess) {
	proto := r.URL.Query().Get("proto")
	if proto != "tls" {
		proto = "https"
	}
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="shielddns-%s-%s.mobileconfig"`, u.ClientID, proto))
	_, _ = w.Write(mobileconfig.Generate(u.ClientID, s.Cfg.BaseDomain, proto))
}

// linkIP привязывает внешний IP клиента к профилю для обычного DNS (UDP/TCP :53) — удобно для роутеров.
func (s *Server) linkIP(w http.ResponseWriter, r *http.Request, u sess) {
	ip := clientIP(r)
	if ip == "" {
		httpErr(w, 400, "Не удалось определить IP")
		return
	}
	s.R.Set(r.Context(), "ip:"+ip, u.ClientID, 30*24*time.Hour)
	_, _ = s.DB.Exec(r.Context(), `UPDATE profiles SET last_known_ip=$2::inet WHERE id=$1`, u.ProfileID, ip)
	writeJSON(w, 200, map[string]string{"ip": ip})
}
