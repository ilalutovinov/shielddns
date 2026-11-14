package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

type point struct {
	T       time.Time `json:"t"`
	Total   int64     `json:"total"`
	Blocked int64     `json:"blocked"`
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request, u sess) {
	rng := r.URL.Query().Get("range")
	bucket, step, span, days := "hour", time.Hour, 24*time.Hour, 1
	switch rng {
	case "7d":
		bucket, step, span, days = "day", 24*time.Hour, 7*24*time.Hour, 7
	case "30d":
		bucket, step, span, days = "day", 24*time.Hour, 30*24*time.Hour, 30
	default:
		rng = "24h"
	}
	now := time.Now().UTC()
	start := now.Add(-span).Truncate(step)
	rows, err := s.DB.Query(r.Context(), `
SELECT date_trunc($2, hour) AS b, SUM(total)::bigint, SUM(blocked)::bigint
FROM stats_hourly WHERE profile_id=$1 AND hour >= $3 GROUP BY b ORDER BY b`, u.ProfileID, bucket, start)
	if err != nil {
		httpErr(w, 500, "Ошибка сервера")
		return
	}
	have := map[int64]point{}
	for rows.Next() {
		var p point
		if rows.Scan(&p.T, &p.Total, &p.Blocked) == nil {
			have[p.T.Unix()] = p
		}
	}
	rows.Close()

	series := []point{}
	var total, blocked int64
	for t := start; !t.After(now); t = t.Add(step) { // заполняем пропуски нулями — график без дыр
		p, ok := have[t.Unix()]
		if !ok {
			p = point{T: t}
		}
		series = append(series, p)
		total += p.Total
		blocked += p.Blocked
	}
	pct := 0.0
	if total > 0 {
		pct = float64(blocked) * 100 / float64(total)
	}
	writeJSON(w, 200, map[string]any{
		"range": rng, "bucket": bucket,
		"totals":     map[string]any{"total": total, "blocked": blocked, "allowed": total - blocked, "percent": pct},
		"series":     series,
		"topBlocked": s.top(r, u, "blocked", days),
		"topAllowed": s.top(r, u, "allowed", days),
	})
}

func (s *Server) top(r *http.Request, u sess, status string, days int) []map[string]any {
	out := []map[string]any{}
	rows, err := s.DB.Query(r.Context(), `
SELECT domain, SUM(hits)::bigint h FROM stats_domains_daily
WHERE profile_id=$1 AND status=$2 AND day >= (now() AT TIME ZONE 'utc')::date - $3::int
GROUP BY domain ORDER BY h DESC LIMIT 10`, u.ProfileID, status, days)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		var h int64
		if rows.Scan(&d, &h) == nil {
			out = append(out, map[string]any{"domain": d, "hits": h})
		}
	}
	return out
}

// logs — живая лента последних запросов из Redis (обновляется воркером раз в несколько секунд).
func (s *Server) logs(w http.ResponseWriter, r *http.Request, u sess) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 50
	}
	items, _ := s.R.LRange(r.Context(), "recent:"+u.ProfileID, 0, int64(limit-1)).Result()
	out := make([]json.RawMessage, 0, len(items))
	for _, it := range items {
		out = append(out, json.RawMessage(it))
	}
	writeJSON(w, 200, out)
}
