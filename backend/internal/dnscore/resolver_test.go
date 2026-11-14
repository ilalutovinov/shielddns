package dnscore

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"

	"shielddns/internal/blocklist"
	"shielddns/internal/model"
)

type fakeUp struct{ calls int32 }

func (f *fakeUp) Exchange(_ context.Context, m *dns.Msg) (*dns.Msg, error) {
	atomic.AddInt32(&f.calls, 1)
	r := new(dns.Msg)
	r.SetReply(m)
	r.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: m.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 120},
		A: net.ParseIP("93.184.216.34")}}
	return r, nil
}

type memCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (c *memCache) Get(_ context.Context, k string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.m[k]
	return b, ok
}
func (c *memCache) Set(_ context.Context, k string, v []byte, _ time.Duration) {
	c.mu.Lock()
	c.m[k] = v
	c.mu.Unlock()
}

type profiles map[string]*model.Profile

func (p profiles) Get(_ context.Context, id string) (*model.Profile, error) {
	pr := p[id]
	if pr != nil {
		pr.Prepare()
	}
	return pr, nil
}

type denyAll struct{}

func (denyAll) Allow(context.Context, *model.Profile) bool { return false }

func setup(p *model.Profile) (*Resolver, *fakeUp, chan model.Event) {
	lists := blocklist.NewManager()
	lists.Set("ads", map[string]struct{}{"ads.example.com": {}, "tracker.net": {}})
	up := &fakeUp{}
	ev := make(chan model.Event, 16)
	return &Resolver{Profiles: profiles{"abc123": p}, Lists: lists, Upstream: up,
		Cache: &memCache{m: map[string][]byte{}}, Events: ev}, up, ev
}

func query(name string, t uint16) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), t)
	return m
}

func base() *model.Profile {
	return &model.Profile{ID: "p1", ClientID: "abc123", BlockMode: "null_ip", Logging: true, Plan: "pro", Lists: []string{"ads"}}
}

func TestCategoryBlocksSubdomain(t *testing.T) {
	r, up, ev := setup(base())
	resp := r.Resolve(context.Background(), "abc123", query("x.tracker.net", dns.TypeA))
	a, ok := resp.Answer[0].(*dns.A)
	if !ok || !a.A.IsUnspecified() {
		t.Fatalf("ожидали 0.0.0.0, получили %v", resp.Answer)
	}
	if up.calls != 0 {
		t.Fatal("заблокированный запрос не должен уходить в апстрим")
	}
	if e := <-ev; e.Status != "blocked" || e.Reason != "list:ads" {
		t.Fatalf("неверное событие: %+v", e)
	}
}

func TestBlockedAAAAandOtherTypes(t *testing.T) {
	r, _, _ := setup(base())
	resp := r.Resolve(context.Background(), "abc123", query("tracker.net", dns.TypeAAAA))
	if aaaa, ok := resp.Answer[0].(*dns.AAAA); !ok || !aaaa.AAAA.IsUnspecified() {
		t.Fatal("ожидали ::")
	}
	resp = r.Resolve(context.Background(), "abc123", query("tracker.net", dns.TypeMX))
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 0 {
		t.Fatal("для MX ожидали пустой NOERROR")
	}
}

func TestWhitelistBeatsBlocklist(t *testing.T) {
	p := base()
	p.Allow = []string{"ads.example.com"}
	r, up, ev := setup(p)
	resp := r.Resolve(context.Background(), "abc123", query("ads.example.com", dns.TypeA))
	if a := resp.Answer[0].(*dns.A); a.A.IsUnspecified() {
		t.Fatal("whitelist должен иметь приоритет")
	}
	if up.calls != 1 {
		t.Fatal("ожидали 1 запрос к апстриму")
	}
	if e := <-ev; e.Status != "allowed" || e.Reason != "whitelist" {
		t.Fatalf("неверное событие: %+v", e)
	}
}

func TestBlacklistAndNXDOMAIN(t *testing.T) {
	p := base()
	p.Block = []string{"bad.org"}
	p.BlockMode = "nxdomain"
	r, _, ev := setup(p)
	resp := r.Resolve(context.Background(), "abc123", query("www.bad.org", dns.TypeA))
	if resp.Rcode != dns.RcodeNameError {
		t.Fatalf("ожидали NXDOMAIN, получили %d", resp.Rcode)
	}
	if e := <-ev; e.Reason != "blacklist" {
		t.Fatalf("причина %q", e.Reason)
	}
}

func TestForwardAndCache(t *testing.T) {
	r, up, _ := setup(base())
	q1 := query("example.org", dns.TypeA)
	q1.Id = 111
	r.Resolve(context.Background(), "abc123", q1)
	q2 := query("example.org", dns.TypeA)
	q2.Id = 222
	resp := r.Resolve(context.Background(), "abc123", q2)
	if up.calls != 1 {
		t.Fatalf("второй запрос должен прийти из кэша, вызовов апстрима: %d", up.calls)
	}
	if resp.Id != 222 || len(resp.Answer) != 1 {
		t.Fatalf("ID должен совпасть с запросом: %d", resp.Id)
	}
}

func TestUnknownProfileRefused(t *testing.T) {
	r, _, _ := setup(base())
	if resp := r.Resolve(context.Background(), "nope00", query("example.org", dns.TypeA)); resp.Rcode != dns.RcodeRefused {
		t.Fatal("неизвестный профиль → REFUSED")
	}
}

func TestLimitReached(t *testing.T) {
	r, up, _ := setup(base())
	r.Limiter = denyAll{}
	if resp := r.Resolve(context.Background(), "abc123", query("example.org", dns.TypeA)); resp.Rcode != dns.RcodeRefused || up.calls != 0 {
		t.Fatal("при исчерпанном лимите → REFUSED без апстрима")
	}
}

func TestClientIDFromHost(t *testing.T) {
	cases := map[string]string{
		"a7f9k2.dns.localhost":          "a7f9k2",
		"a7f9k2.dns.example.com:443":    "a7f9k2",
		"A7F9K2.DNS.example.com.":       "a7f9k2",
		"dns.example.com":               "",
		"evil.example.com":              "",
		"x.dns.example.com":             "", // слишком короткий id
		"a7f9k2.notdns.example.com":     "",
	}
	for in, want := range cases {
		if got := ClientIDFromHost(in); got != want {
			t.Errorf("%q: получили %q, ожидали %q", in, got, want)
		}
	}
}
