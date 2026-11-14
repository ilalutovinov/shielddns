// Package dnscore — ядро фильтрации: whitelist → blacklist → категории → апстрим + кэш.
package dnscore

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"

	"shielddns/internal/model"
)

type ProfileSource interface {
	Get(ctx context.Context, clientID string) (*model.Profile, error)
}
type Matcher interface {
	Match(domain string, slugs []string) (string, bool)
}
type Upstream interface {
	Exchange(ctx context.Context, m *dns.Msg) (*dns.Msg, error)
}
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration)
}
type Limiter interface {
	Allow(ctx context.Context, p *model.Profile) bool
}

type Resolver struct {
	Profiles ProfileSource
	Lists    Matcher
	Upstream Upstream
	Cache    Cache          // может быть nil
	Limiter  Limiter        // может быть nil
	Events   chan<- model.Event // может быть nil; отправка неблокирующая
}

func NormalizeName(n string) string { return strings.ToLower(strings.TrimSuffix(n, ".")) }

func fail(req *dns.Msg, rcode int) *dns.Msg {
	m := new(dns.Msg)
	m.SetRcode(req, rcode)
	m.RecursionAvailable = true
	return m
}

// Resolve обрабатывает один запрос клиента clientID.
func (r *Resolver) Resolve(ctx context.Context, clientID string, req *dns.Msg) *dns.Msg {
	if len(req.Question) != 1 {
		return fail(req, dns.RcodeFormatError)
	}
	q := req.Question[0]
	domain := NormalizeName(q.Name)

	p, err := r.Profiles.Get(ctx, clientID)
	if err != nil {
		return fail(req, dns.RcodeServerFailure)
	}
	if p == nil {
		return fail(req, dns.RcodeRefused)
	}
	if r.Limiter != nil && !r.Limiter.Allow(ctx, p) {
		return fail(req, dns.RcodeRefused) // исчерпан лимит Free
	}

	status, reason := r.decide(p, domain)
	if status == "blocked" {
		r.emit(p, domain, q.Qtype, status, reason)
		return blockedResponse(req, q, p.BlockMode)
	}
	resp := r.forward(ctx, req, q)
	r.emit(p, domain, q.Qtype, status, reason)
	return resp
}

func (r *Resolver) decide(p *model.Profile, domain string) (status, reason string) {
	switch {
	case p.IsAllowed(domain):
		return "allowed", "whitelist"
	case p.IsBlocked(domain):
		return "blocked", "blacklist"
	}
	if r.Lists != nil {
		if slug, ok := r.Lists.Match(domain, p.Lists); ok {
			return "blocked", "list:" + slug
		}
	}
	return "allowed", ""
}

func (r *Resolver) emit(p *model.Profile, domain string, qtype uint16, status, reason string) {
	if r.Events == nil || p.ID == "" {
		return
	}
	select {
	case r.Events <- model.Event{TS: time.Now(), ProfileID: p.ID, Domain: domain, QType: dns.TypeToString[qtype],
		Status: status, Reason: reason, Logging: p.Logging}:
	default: // буфер полон — лучше потерять событие, чем замедлить DNS
	}
}

func blockedResponse(req *dns.Msg, q dns.Question, mode string) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(req)
	m.RecursionAvailable = true
	if mode == "nxdomain" {
		m.Rcode = dns.RcodeNameError
		return m
	}
	hdr := dns.RR_Header{Name: q.Name, Rrtype: q.Qtype, Class: dns.ClassINET, Ttl: 60}
	switch q.Qtype {
	case dns.TypeA:
		m.Answer = append(m.Answer, &dns.A{Hdr: hdr, A: net.IPv4zero.To4()})
	case dns.TypeAAAA:
		m.Answer = append(m.Answer, &dns.AAAA{Hdr: hdr, AAAA: net.IPv6zero})
	}
	return m
}

func (r *Resolver) forward(ctx context.Context, req *dns.Msg, q dns.Question) *dns.Msg {
	key := fmt.Sprintf("%s|%d|%t", NormalizeName(q.Name), q.Qtype, hasDO(req))
	if r.Cache != nil {
		if b, ok := r.Cache.Get(ctx, key); ok {
			m := new(dns.Msg)
			if m.Unpack(b) == nil {
				m.Id = req.Id
				return m
			}
		}
	}
	resp, err := r.Upstream.Exchange(ctx, req)
	if err != nil || resp == nil {
		return fail(req, dns.RcodeServerFailure)
	}
	resp.Id = req.Id
	if r.Cache != nil && !resp.Truncated && (resp.Rcode == dns.RcodeSuccess || resp.Rcode == dns.RcodeNameError) {
		resp.Compress = true
		if b, err := resp.Pack(); err == nil {
			r.Cache.Set(ctx, key, b, TTLOf(resp))
		}
	}
	return resp
}

func hasDO(m *dns.Msg) bool {
	if o := m.IsEdns0(); o != nil {
		return o.Do()
	}
	return false
}

// TTLOf — минимальный TTL ответа, ограниченный диапазоном 30с…1ч.
func TTLOf(m *dns.Msg) time.Duration {
	min := uint32(0)
	for _, rrs := range [][]dns.RR{m.Answer, m.Ns} {
		for _, rr := range rrs {
			if t := rr.Header().Ttl; min == 0 || t < min {
				min = t
			}
		}
	}
	d := time.Duration(min) * time.Second
	if d < 30*time.Second {
		d = 30 * time.Second
	}
	if d > time.Hour {
		d = time.Hour
	}
	return d
}

// UDPUpstream пересылает запрос по очереди на каждый апстрим; при усечении повторяет по TCP.
type UDPUpstream struct{ Servers []string }

func NewUDPUpstream(servers []string) *UDPUpstream {
	out := make([]string, len(servers))
	for i, s := range servers {
		if _, _, err := net.SplitHostPort(s); err != nil {
			s = net.JoinHostPort(s, "53")
		}
		out[i] = s
	}
	return &UDPUpstream{Servers: out}
}

func (u *UDPUpstream) Exchange(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	last := errors.New("нет апстримов")
	for _, s := range u.Servers {
		for _, network := range []string{"udp", "tcp"} {
			c := &dns.Client{Net: network, Timeout: 2 * time.Second}
			resp, _, err := c.ExchangeContext(ctx, m, s)
			if err != nil {
				last = err
				break
			}
			if resp.Truncated && network == "udp" {
				continue
			}
			return resp, nil
		}
	}
	return nil, last
}
