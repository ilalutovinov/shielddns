// Package api — REST API личного кабинета.
package api

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"shielddns/internal/config"
)

type Server struct {
	DB  *pgxpool.Pool
	R   *redis.Client
	Cfg config.Config
	rl  limiter
}

func New(db *pgxpool.Pool, r *redis.Client, cfg config.Config) *Server {
	return &Server{DB: db, R: r, Cfg: cfg, rl: limiter{hits: map[string][]time.Time{}}}
}

type sess struct{ UserID, ProfileID, ClientID, Plan string }

func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	m.HandleFunc("POST /api/auth/register", s.register)
	m.HandleFunc("POST /api/auth/login", s.login)
	m.HandleFunc("GET /api/me", s.auth(s.me))
	m.HandleFunc("PATCH /api/profile", s.auth(s.patchProfile))
	m.HandleFunc("GET /api/profile/filters", s.auth(s.getFilters))
	m.HandleFunc("PUT /api/profile/filters", s.auth(s.putFilters))
	m.HandleFunc("GET /api/profile/rules", s.auth(s.listRules))
	m.HandleFunc("POST /api/profile/rules", s.auth(s.addRule))
	m.HandleFunc("DELETE /api/profile/rules/{id}", s.auth(s.delRule))
	m.HandleFunc("GET /api/profile/mobileconfig", s.auth(s.mobileconfig))
	m.HandleFunc("POST /api/profile/link-ip", s.auth(s.linkIP))
	m.HandleFunc("POST /api/subscription", s.auth(s.setPlan))
	m.HandleFunc("GET /api/stats", s.auth(s.stats))
	m.HandleFunc("GET /api/logs", s.auth(s.logs))
	return s.cors(m)
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func httpErr(w http.ResponseWriter, code int, msg string) { writeJSON(w, code, map[string]string{"error": msg}) }

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		httpErr(w, 400, "Некорректный JSON")
		return false
	}
	return true
}

func clientIP(r *http.Request) string {
	if x := r.Header.Get("X-Forwarded-For"); x != "" { // API стоит за Caddy
		return strings.TrimSpace(strings.Split(x, ",")[0])
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

func isUnique(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

const idAlphabet = "abcdefghjkmnpqrstuvwxyz23456789" // 31 символ без похожих (i, l, o, 0, 1)

// newClientID — 8 символов из crypto/rand (~39 бит): идентификатор в поддомене не угадать перебором.
func newClientID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = idAlphabet[int(b[i])%len(idAlphabet)]
	}
	return string(b)
}

var domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9-]{2,63}$`)

// normDomain приводит ввод пользователя (в т.ч. URL) к чистому домену.
func normDomain(in string) (string, bool) {
	d := strings.ToLower(strings.TrimSpace(in))
	d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
	if i := strings.IndexAny(d, "/?#"); i >= 0 {
		d = d[:i]
	}
	d = strings.TrimPrefix(strings.TrimSuffix(d, "."), "*.")
	return d, len(d) <= 253 && domainRe.MatchString(d)
}

type limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

// allow: не более 10 попыток в минуту с одного IP.
func (l *limiter) allow(k string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.hits) > 10000 {
		l.hits = map[string][]time.Time{}
	}
	cut := time.Now().Add(-time.Minute)
	var keep []time.Time
	for _, t := range l.hits[k] {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}
	if len(keep) >= 10 {
		l.hits[k] = keep
		return false
	}
	l.hits[k] = append(keep, time.Now())
	return true
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", s.Cfg.FrontendOrigin)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------- auth ----------

func (s *Server) issue(userID string) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
	}).SignedString([]byte(s.Cfg.JWTSecret))
}

func (s *Server) auth(h func(http.ResponseWriter, *http.Request, sess)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		claims := &jwt.RegisteredClaims{}
		t, err := jwt.ParseWithClaims(tok, claims, func(*jwt.Token) (any, error) { return []byte(s.Cfg.JWTSecret), nil },
			jwt.WithValidMethods([]string{"HS256"}))
		if err != nil || !t.Valid || claims.Subject == "" {
			httpErr(w, 401, "Требуется вход")
			return
		}
		u := sess{UserID: claims.Subject}
		err = s.DB.QueryRow(r.Context(), `
SELECT p.id::text, p.client_id,
  CASE WHEN sub.plan='pro' AND (sub.expires_at IS NULL OR sub.expires_at > now()) THEN 'pro' ELSE 'free' END
FROM profiles p JOIN subscriptions sub ON sub.user_id=p.user_id WHERE p.user_id=$1`, u.UserID).
			Scan(&u.ProfileID, &u.ClientID, &u.Plan)
		if err != nil {
			httpErr(w, 401, "Требуется вход")
			return
		}
		h(w, r, u)
	}
}

type creds struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if !s.rl.allow(clientIP(r)) {
		httpErr(w, 429, "Слишком много попыток, подождите минуту")
		return
	}
	var in creds
	if !readJSON(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email || len(email) > 254 {
		httpErr(w, 400, "Некорректный email")
		return
	}
	if len(in.Password) < 8 || len(in.Password) > 72 {
		httpErr(w, 400, "Пароль: от 8 до 72 символов")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		httpErr(w, 500, "Ошибка сервера")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		httpErr(w, 500, "Ошибка сервера")
		return
	}
	defer tx.Rollback(ctx)

	var uid, pid string
	if err = tx.QueryRow(ctx, `INSERT INTO users(email, password_hash) VALUES($1,$2) RETURNING id::text`, email, string(hash)).Scan(&uid); err != nil {
		if isUnique(err) {
			httpErr(w, 409, "Этот email уже зарегистрирован")
			return
		}
		httpErr(w, 500, "Ошибка сервера")
		return
	}
	if _, err = tx.Exec(ctx, `INSERT INTO subscriptions(user_id) VALUES($1)`, uid); err != nil {
		httpErr(w, 500, "Ошибка сервера")
		return
	}
	for i := 0; i < 5 && pid == ""; i++ {
		err = tx.QueryRow(ctx, `INSERT INTO profiles(user_id, client_id) VALUES($1,$2)
			ON CONFLICT (client_id) DO NOTHING RETURNING id::text`, uid, newClientID()).Scan(&pid)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			httpErr(w, 500, "Ошибка сервера")
			return
		}
	}
	if pid == "" {
		httpErr(w, 500, "Не удалось создать профиль")
		return
	}
	if _, err = tx.Exec(ctx, `INSERT INTO profile_blocklists(profile_id, blocklist_id)
		SELECT $1::uuid, id FROM blocklists WHERE slug IN ('ads','malware')`, pid); err != nil {
		httpErr(w, 500, "Ошибка сервера")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		httpErr(w, 500, "Ошибка сервера")
		return
	}
	tok, _ := s.issue(uid)
	writeJSON(w, 201, map[string]string{"token": tok})
}

// dummyHash выравнивает время ответа для несуществующих email.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.rl.allow(clientIP(r)) {
		httpErr(w, 429, "Слишком много попыток, подождите минуту")
		return
	}
	var in creds
	if !readJSON(w, r, &in) {
		return
	}
	var uid string
	var hash []byte
	err := s.DB.QueryRow(r.Context(), `SELECT id::text, password_hash FROM users WHERE email=$1`,
		strings.ToLower(strings.TrimSpace(in.Email))).Scan(&uid, &hash)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(in.Password))
		httpErr(w, 401, "Неверный email или пароль")
		return
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(in.Password)) != nil {
		httpErr(w, 401, "Неверный email или пароль")
		return
	}
	tok, _ := s.issue(uid)
	writeJSON(w, 200, map[string]string{"token": tok})
}
