package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/miekg/dns"

	"shielddns/internal/blocklist"
	"shielddns/internal/config"
	"shielddns/internal/db"
	"shielddns/internal/dnscore"
	"shielddns/internal/model"
	"shielddns/internal/stats"
	"shielddns/internal/store"
)

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()
	rdb, err := db.Redis(cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer rdb.Close()

	lists := blocklist.NewManager()
	go lists.Run(ctx, pool, cfg.BlocklistRefresh)

	events := make(chan model.Event, 20000)
	ps := store.NewProfileStore(pool, rdb, cfg.AllowAnonDefault)
	go ps.Subscribe(ctx)
	worker := &stats.Worker{DB: pool, R: rdb, In: events, Every: cfg.StatsFlush}
	workerDone := make(chan struct{})
	go func() { worker.Run(ctx); close(workerDone) }()

	res := &dnscore.Resolver{
		Profiles: ps, Lists: lists, Upstream: dnscore.NewUDPUpstream(cfg.Upstreams),
		Cache: store.RedisCache{R: rdb}, Limiter: store.RedisLimiter{R: rdb}, Events: events,
	}
	srv := &dnscore.Server{Res: res, IPs: ps, DefaultID: cfg.DefaultClientID, Anon: cfg.AllowAnonDefault}

	udp := &dns.Server{Addr: cfg.DNSAddr, Net: "udp", Handler: srv}
	tcp := &dns.Server{Addr: cfg.DNSAddr, Net: "tcp", Handler: srv}
	for _, s := range []*dns.Server{udp, tcp} {
		go func(s *dns.Server) {
			log.Printf("DNS %s %s", s.Net, s.Addr)
			if err := s.ListenAndServe(); err != nil {
				log.Fatalf("dns %s: %v", s.Net, err)
			}
		}(s)
	}

	tlsCfg, err := dnscore.LoadTLS(cfg.CertFile, cfg.KeyFile, cfg.BaseDomain)
	if err != nil {
		log.Fatalf("tls: %v", err)
	}
	go func() {
		log.Printf("DoT %s", cfg.DoTAddr)
		if err := srv.ServeDoT(ctx, cfg.DoTAddr, tlsCfg); err != nil {
			log.Fatalf("dot: %v", err)
		}
	}()

	doh := &http.Server{Addr: cfg.DoHAddr, Handler: srv.DoH(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("DoH (http, за Caddy) %s", cfg.DoHAddr)
		if err := doh.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("doh: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("останавливаюсь…")
	sc, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = udp.ShutdownContext(sc)
	_ = tcp.ShutdownContext(sc)
	_ = doh.Shutdown(sc)
	<-workerDone // дописать остаток статистики
}
