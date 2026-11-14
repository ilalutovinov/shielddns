// Package store — доступ DNS-ядра к профилям (память → Redis → Postgres), кэшу ответов и лимитам.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"shielddns/internal/model"
)

const InvalidateChannel = "profile:invalidate"

const loadProfileSQL = `
SELECT p.id::text, p.client_id, p.name, p.block_mode, p.logging,
  CASE WHEN s.plan='pro' AND (s.expires_at IS NULL OR s.expires_at > now()) THEN 'pro' ELSE 'free' END,
  COALESCE((SELECT array_agg(b.slug) FROM profile_blocklists pb JOIN blocklists b ON b.id=pb.blocklist_id
            WHERE pb.profile_id=p.id AND b.enabled), '{}'),
  COALESCE((SELECT array_agg(domain) FROM custom_rules WHERE profile_id=p.id AND action='allow'), '{}'),
  COALESCE((SELECT array_agg(domain) FROM custom_rules WHERE profile_id=p.id AND action='block'), '{}')
FROM profiles p JOIN subscriptions s ON s.user_id=p.user_id
WHERE p.client_id=$1`

func LoadProfile(ctx context.Context, db *pgxpool.Pool, clientID string) (*model.Profile, error) {
	p := &model.Profile{}
	err := db.QueryRow(ctx, loadProfileSQL, clientID).
		Scan(&p.ID, &p.ClientID, &p.Name, &p.BlockMode, &p.Logging, &p.Plan, &p.Lists, &p.Allow, &p.Block)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.Prepare()
	return p, nil
}

// Invalidate сбрасывает профиль в Redis и во всех DNS-процессах (pub/sub).
func Invalidate(ctx context.Context, r *redis.Client, clientID string) {
	pipe := r.Pipeline()
	pipe.Del(ctx, "profile:"+clientID)
	pipe.Publish(ctx, InvalidateChannel, clientID)
	_, _ = pipe.Exec(ctx)
}

type entry struct {
	p   *model.Profile
	id  string
	exp time.Time
}

type ProfileStore struct {
	DB        *pgxpool.Pool
	R         *redis.Client
	AllowAnon bool

	mu    sync.RWMutex
	local map[string]entry
	ips   map[string]entry
}

func NewProfileStore(db *pgxpool.Pool, r *redis.Client, allowAnon bool) *ProfileStore {
	return &ProfileStore{DB: db, R: r, AllowAnon: allowAnon, local: map[string]entry{}, ips: map[string]entry{}}
}

func (s *ProfileStore) anon() *model.Profile {
	p := &model.Profile{ClientID: model.AnonClientID, Name: "Default", BlockMode: "null_ip", Plan: "pro",
		Lists: []string{"ads", "oisd", "malware"}}
	p.Prepare()
	return p
}

func (s *ProfileStore) Get(ctx context.Context, id string) (*model.Profile, error) {
	if id == "" {
		return nil, nil
	}
	if id == model.AnonClientID {
		if s.AllowAnon {
			return s.anon(), nil
		}
		return nil, nil
	}
	s.mu.RLock()
	e, ok := s.local[id]
	s.mu.RUnlock()
	if ok && time.Now().Before(e.exp) {
		return e.p, nil
	}
	p, err := s.fetch(ctx, id)
	if err != nil {
		return nil, err
	}
	ttl := 15 * time.Second
	if p == nil {
		ttl = 10 * time.Second // негативный кэш: защита БД от случайных SNI
	}
	s.mu.Lock()
	if len(s.local) > 50000 {
		s.local = map[string]entry{}
	}
	s.local[id] = entry{p: p, exp: time.Now().Add(ttl)}
	s.mu.Unlock()
	return p, nil
}

func (s *ProfileStore) fetch(ctx context.Context, id string) (*model.Profile, error) {
	if b, err := s.R.Get(ctx, "profile:"+id).Bytes(); err == nil {
		p := &model.Profile{}
		if json.Unmarshal(b, p) == nil {
			p.Prepare()
			return p, nil
		}
	}
	p, err := LoadProfile(ctx, s.DB, id)
	if err != nil || p == nil {
		return nil, err
	}
	if b, err := json.Marshal(p); err == nil {
		s.R.Set(ctx, "profile:"+id, b, 5*time.Minute)
	}
	return p, nil
}

// Subscribe слушает события сброса профилей от API.
func (s *ProfileStore) Subscribe(ctx context.Context) {
	sub := s.R.Subscribe(ctx, InvalidateChannel)
	defer sub.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-sub.Channel():
			if !ok {
				return
			}
			s.mu.Lock()
			delete(s.local, m.Payload)
			s.mu.Unlock()
		}
	}
}

// ClientForIP — привязка обычного DNS к профилю по IP клиента (ключ ip:<адрес> в Redis ставит API).
func (s *ProfileStore) ClientForIP(ctx context.Context, ip string) string {
	s.mu.RLock()
	e, ok := s.ips[ip]
	s.mu.RUnlock()
	if ok && time.Now().Before(e.exp) {
		return e.id
	}
	id, _ := s.R.Get(ctx, "ip:"+ip).Result()
	s.mu.Lock()
	if len(s.ips) > 50000 {
		s.ips = map[string]entry{}
	}
	s.ips[ip] = entry{id: id, exp: time.Now().Add(30 * time.Second)}
	s.mu.Unlock()
	return id
}

// RedisCache — общий кэш ответов апстрима (ключ: имя|тип|DO).
type RedisCache struct{ R *redis.Client }

func (c RedisCache) Get(ctx context.Context, key string) ([]byte, bool) {
	ctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	b, err := c.R.Get(ctx, "dnsc:"+key).Bytes()
	return b, err == nil
}

func (c RedisCache) Set(ctx context.Context, key string, val []byte, ttl time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	c.R.Set(ctx, "dnsc:"+key, val, ttl)
}

// RedisLimiter — месячный лимит запросов тарифа Free. При недоступности Redis пропускает трафик.
type RedisLimiter struct{ R *redis.Client }

func (l RedisLimiter) Allow(ctx context.Context, p *model.Profile) bool {
	if p.Plan == "pro" || p.ID == "" {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	key := fmt.Sprintf("usage:%s:%s", p.ID, time.Now().UTC().Format("200601"))
	n, err := l.R.Incr(ctx, key).Result()
	if err != nil {
		return true
	}
	if n == 1 {
		l.R.Expire(ctx, key, 40*24*time.Hour)
	}
	return n <= model.FreeMonthlyLimit
}
