package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL, RedisURL, JWTSecret, BaseDomain, FrontendOrigin string
	DNSAddr, DoTAddr, DoHAddr, APIAddr                           string
	CertFile, KeyFile, DefaultClientID                           string
	Upstreams                                                    []string
	StatsFlush, BlocklistRefresh                                 time.Duration
	AllowAnonDefault                                             bool
}

func Load() Config {
	var ups []string
	for _, u := range strings.Split(get("UPSTREAMS", "1.1.1.1:53,8.8.8.8:53"), ",") {
		if u = strings.TrimSpace(u); u != "" {
			ups = append(ups, u)
		}
	}
	return Config{
		DatabaseURL:      get("DATABASE_URL", "postgres://shield:shield_dev_pass@localhost:5432/shielddns?sslmode=disable"),
		RedisURL:         get("REDIS_URL", "redis://localhost:6379/0"),
		JWTSecret:        get("JWT_SECRET", "dev-secret-change-me"),
		BaseDomain:       get("BASE_DOMAIN", "localhost"),
		FrontendOrigin:   get("FRONTEND_ORIGIN", "*"),
		DNSAddr:          get("DNS_UDP_ADDR", ":53"),
		DoTAddr:          get("DOT_ADDR", ":853"),
		DoHAddr:          get("DOH_ADDR", ":8053"),
		APIAddr:          get("API_ADDR", ":8080"),
		CertFile:         get("DOT_CERT", ""),
		KeyFile:          get("DOT_KEY", ""),
		DefaultClientID:  get("DEFAULT_CLIENT_ID", ""),
		Upstreams:        ups,
		StatsFlush:       time.Duration(getInt("STATS_FLUSH_SECONDS", 4)) * time.Second,
		BlocklistRefresh: time.Duration(getInt("BLOCKLIST_REFRESH_HOURS", 24)) * time.Hour,
		AllowAnonDefault: get("ALLOW_ANON_DEFAULT", "true") == "true",
	}
}

func get(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func getInt(k string, d int) int {
	if n, err := strconv.Atoi(os.Getenv(k)); err == nil && n > 0 {
		return n
	}
	return d
}
