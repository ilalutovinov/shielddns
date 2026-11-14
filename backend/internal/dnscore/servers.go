package dnscore

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"

	"shielddns/internal/model"
)

var idRe = regexp.MustCompile(`^[a-z0-9]{6,12}$`)

// ClientIDFromHost: "a7f9k2.dns.example.com[:port]" → "a7f9k2".
func ClientIDFromHost(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	labels := strings.Split(strings.ToLower(strings.TrimSuffix(host, ".")), ".")
	if len(labels) >= 3 && labels[1] == "dns" && idRe.MatchString(labels[0]) {
		return labels[0]
	}
	return ""
}

type IPMapper interface {
	ClientForIP(ctx context.Context, ip string) string
}

// Server объединяет транспорты (UDP/TCP :53, DoT, DoH) вокруг одного Resolver.
type Server struct {
	Res       *Resolver
	IPs       IPMapper
	DefaultID string // DEFAULT_CLIENT_ID для обычного DNS
	Anon      bool   // разрешить встроенный профиль по умолчанию
}

// fallbackID — клиент для запросов без идентификатора: привязка по IP → профиль по умолчанию → анонимный.
func (s *Server) fallbackID(ctx context.Context, ip string) string {
	if s.IPs != nil && ip != "" {
		if id := s.IPs.ClientForIP(ctx, ip); id != "" {
			return id
		}
	}
	if s.DefaultID != "" {
		return s.DefaultID
	}
	if s.Anon {
		return model.AnonClientID
	}
	return ""
}

func ipOf(a net.Addr) string {
	if a == nil {
		return ""
	}
	h, _, err := net.SplitHostPort(a.String())
	if err != nil {
		return a.String()
	}
	return h
}

// ServeDNS — обычный DNS (UDP/TCP).
func (s *Server) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	resp := s.Res.Resolve(ctx, s.fallbackID(ctx, ipOf(w.RemoteAddr())), req)
	if _, udp := w.RemoteAddr().(*net.UDPAddr); udp {
		size := dns.MinMsgSize
		if o := req.IsEdns0(); o != nil && int(o.UDPSize()) > size {
			size = int(o.UDPSize())
		}
		resp.Truncate(size)
	}
	_ = w.WriteMsg(resp)
}

// ServeDoT — DNS-over-TLS на своём цикле, чтобы получить SNI после рукопожатия.
func (s *Server) ServeDoT(ctx context.Context, addr string, cfg *tlsConfig) error {
	ln, err := tlsListen(addr, cfg)
	if err != nil {
		return err
	}
	go func() { <-ctx.Done(); ln.Close() }()
	for {
		c, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return nil
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}
		go s.handleDoT(c)
	}
}

func (s *Server) handleDoT(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	sni, err := handshakeSNI(conn)
	if err != nil {
		return
	}
	cid := ClientIDFromHost(sni)
	dc := &dns.Conn{Conn: conn}
	for {
		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		req, err := dc.ReadMsg()
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		id := cid
		if id == "" {
			id = s.fallbackID(ctx, ipOf(conn.RemoteAddr()))
		}
		resp := s.Res.Resolve(ctx, id, req)
		cancel()
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if dc.WriteMsg(resp) != nil {
			return
		}
	}
}

// DoH — RFC 8484: GET ?dns=<base64url> и POST application/dns-message.
// client_id берётся из Host (<id>.dns.<домен>), фолбэк — путь /dns-query/<id>.
func (s *Server) DoH() http.Handler {
	h := func(w http.ResponseWriter, r *http.Request) {
		var raw []byte
		var err error
		switch r.Method {
		case http.MethodGet:
			raw, err = base64.RawURLEncoding.DecodeString(strings.TrimRight(r.URL.Query().Get("dns"), "="))
		case http.MethodPost:
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/dns-message") {
				http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
				return
			}
			raw, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 65535))
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		req := new(dns.Msg)
		if err != nil || len(raw) == 0 || req.Unpack(raw) != nil {
			http.Error(w, "bad dns message", http.StatusBadRequest)
			return
		}
		cid := ClientIDFromHost(r.Host)
		if cid == "" {
			if p := r.PathValue("id"); idRe.MatchString(p) {
				cid = p
			}
		}
		if cid == "" {
			cid = s.fallbackID(r.Context(), httpClientIP(r))
		}
		resp := s.Res.Resolve(r.Context(), cid, req)
		out, err := resp.Pack()
		if err != nil {
			http.Error(w, "pack error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/dns-message")
		w.Header().Set("Cache-Control", "max-age="+strconv.Itoa(int(TTLOf(resp).Seconds())))
		_, _ = w.Write(out)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/dns-query", h)
	mux.HandleFunc("/dns-query/{id}", h)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	return mux
}

func httpClientIP(r *http.Request) string {
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		return strings.TrimSpace(strings.Split(x, ",")[0])
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

func logf(f string, a ...any) { log.Printf("dns: "+f, a...) }
