package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"shielddns/internal/api"
	"shielddns/internal/config"
	"shielddns/internal/db"
)

func main() {
	cfg := config.Load()
	if cfg.JWTSecret == "dev-secret-change-me" || cfg.JWTSecret == "change_me_to_long_random_string" {
		log.Println("ВНИМАНИЕ: задайте длинный случайный JWT_SECRET (openssl rand -hex 32)")
	}
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

	srv := &http.Server{
		Addr: cfg.APIAddr, Handler: api.New(pool, rdb, cfg).Handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second,
	}
	go func() {
		log.Printf("API %s", cfg.APIAddr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("api: %v", err)
		}
	}()
	<-ctx.Done()
	sc, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(sc)
}
