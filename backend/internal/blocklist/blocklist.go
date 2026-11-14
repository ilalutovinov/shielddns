// Package blocklist хранит глобальные списки блокировок в памяти.
// Поиск: проверка домена и всех родителей по map — O(число меток), т.е. фактически O(1).
package blocklist

import (
	"bufio"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"shielddns/internal/model"
)

type Manager struct {
	mu   sync.RWMutex
	sets map[string]map[string]struct{}
}

func NewManager() *Manager { return &Manager{sets: map[string]map[string]struct{}{}} }

func (m *Manager) Set(slug string, s map[string]struct{}) {
	m.mu.Lock()
	m.sets[slug] = s
	m.mu.Unlock()
}

func (m *Manager) Size(slug string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sets[slug])
}

// Match возвращает slug первого списка из slugs, в котором найден домен.
func (m *Manager) Match(domain string, slugs []string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, slug := range slugs {
		if model.SuffixMatch(m.sets[slug], domain) {
			return slug, true
		}
	}
	return "", false
}

// Parse понимает hosts ("0.0.0.0 domain"), списки доменов и adblock ("||domain^").
func Parse(r io.Reader) map[string]struct{} {
	out := make(map[string]struct{}, 1<<16)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == '!' || line[0] == '[' {
			continue
		}
		var d string
		if strings.HasPrefix(line, "||") {
			d = line[2:]
			if i := strings.IndexAny(d, "^/$"); i >= 0 {
				d = d[:i]
			}
		} else {
			if i := strings.Index(line, "#"); i >= 0 {
				line = strings.TrimSpace(line[:i])
			}
			f := strings.Fields(line)
			switch len(f) {
			case 0:
				continue
			case 1:
				d = f[0]
			default:
				if net.ParseIP(f[0]) == nil {
					continue
				}
				d = f[1]
			}
		}
		d = strings.TrimPrefix(strings.ToLower(strings.TrimSuffix(d, ".")), "*.")
		if validHost(d) {
			out[d] = struct{}{}
		}
	}
	return out
}

func validHost(d string) bool {
	if !strings.Contains(d, ".") || len(d) > 253 || strings.ContainsAny(d, " /:*@") || net.ParseIP(d) != nil {
		return false
	}
	return d != "localhost.localdomain"
}

// Refresh скачивает все включённые списки; при ошибке старая версия списка остаётся в работе.
func (m *Manager) Refresh(ctx context.Context, pool *pgxpool.Pool) {
	rows, err := pool.Query(ctx, `SELECT slug, source_url FROM blocklists WHERE enabled`)
	if err != nil {
		log.Printf("blocklists: query: %v", err)
		return
	}
	type src struct{ slug, url string }
	var list []src
	for rows.Next() {
		var s src
		if rows.Scan(&s.slug, &s.url) == nil {
			list = append(list, s)
		}
	}
	rows.Close()
	client := &http.Client{Timeout: 90 * time.Second}
	for _, s := range list {
		set, err := fetch(ctx, client, s.url)
		if err != nil || len(set) == 0 {
			log.Printf("blocklists: %s: %v (записей: %d), оставляю прежний список", s.slug, err, len(set))
			continue
		}
		m.Set(s.slug, set)
		_, _ = pool.Exec(ctx, `UPDATE blocklists SET domain_count=$2, updated_at=now() WHERE slug=$1`, s.slug, len(set))
		log.Printf("blocklists: %s загружен, %d доменов", s.slug, len(set))
	}
}

func fetch(ctx context.Context, c *http.Client, url string) (map[string]struct{}, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "ShieldDNS/1.0")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, io.ErrUnexpectedEOF
	}
	return Parse(io.LimitReader(resp.Body, 64<<20)), nil
}

// Run сразу обновляет списки (в фоне, DNS стартует без ожидания) и затем повторяет по таймеру.
func (m *Manager) Run(ctx context.Context, pool *pgxpool.Pool, every time.Duration) {
	m.Refresh(ctx, pool)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.Refresh(ctx, pool)
		}
	}
}
