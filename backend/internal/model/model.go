// Package model содержит общие типы DNS-ядра и API.
package model

import (
	"strings"
	"time"
)

const (
	FreeMonthlyLimit = 50000 // запросов в месяц на тарифе Free
	MaxFreeRules     = 10    // кастомных правил на Free
	AnonClientID     = "_default"
)

// Profile — настройки фильтрации одного клиента (кэшируется в Redis как JSON).
type Profile struct {
	ID        string   `json:"id"`
	ClientID  string   `json:"clientId"`
	Name      string   `json:"name"`
	BlockMode string   `json:"blockMode"` // null_ip | nxdomain
	Logging   bool     `json:"logging"`
	Plan      string   `json:"plan"` // free | pro
	Lists     []string `json:"lists"`
	Allow     []string `json:"allow"`
	Block     []string `json:"block"`

	allow, block map[string]struct{}
}

// Prepare строит множества для быстрого поиска. Идемпотентна.
func (p *Profile) Prepare() {
	p.allow = toSet(p.Allow)
	p.block = toSet(p.Block)
}

func (p *Profile) IsAllowed(domain string) bool { return SuffixMatch(p.allow, domain) }
func (p *Profile) IsBlocked(domain string) bool { return SuffixMatch(p.block, domain) }

func toSet(items []string) map[string]struct{} {
	s := make(map[string]struct{}, len(items))
	for _, d := range items {
		s[strings.ToLower(strings.TrimSuffix(d, "."))] = struct{}{}
	}
	return s
}

// SuffixMatch проверяет домен и всех его родителей (a.ads.example.com → ads.example.com → example.com).
func SuffixMatch(set map[string]struct{}, domain string) bool {
	if len(set) == 0 {
		return false
	}
	for {
		if _, ok := set[domain]; ok {
			return true
		}
		i := strings.IndexByte(domain, '.')
		if i < 0 {
			return false
		}
		domain = domain[i+1:]
	}
}

// Event — запись статистики: летит из DNS-пути в фоновый воркер.
type Event struct {
	TS        time.Time
	ProfileID string
	Domain    string
	QType     string
	Status    string // allowed | blocked
	Reason    string // whitelist | blacklist | list:<slug> | ""
	Logging   bool
}
