package dnscore

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"os"
	"sync"
	"time"
)

type tlsConfig = tls.Config

func tlsListen(addr string, cfg *tls.Config) (net.Listener, error) { return tls.Listen("tcp", addr, cfg) }

// handshakeSNI выполняет TLS-рукопожатие и возвращает SNI клиента.
func handshakeSNI(c net.Conn) (string, error) {
	tc, ok := c.(*tls.Conn)
	if !ok {
		return "", net.ErrClosed
	}
	if err := tc.Handshake(); err != nil {
		return "", err
	}
	return tc.ConnectionState().ServerName, nil
}

type certReloader struct {
	certFile, keyFile string
	mu                sync.Mutex
	cert              *tls.Certificate
	mod, last         time.Time
}

func (c *certReloader) load() error {
	st, err := os.Stat(c.certFile)
	if err != nil {
		return err
	}
	cert, err := tls.LoadX509KeyPair(c.certFile, c.keyFile)
	if err != nil {
		return err
	}
	c.cert, c.mod = &cert, st.ModTime()
	return nil
}

// GetCertificate подхватывает обновлённый (перевыпущенный) сертификат без перезапуска.
func (c *certReloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.last) > time.Minute {
		c.last = time.Now()
		if st, err := os.Stat(c.certFile); err == nil && st.ModTime().After(c.mod) {
			if err := c.load(); err != nil {
				logf("перезагрузка сертификата: %v", err)
			}
		}
	}
	return c.cert, nil
}

// LoadTLS: сертификат из файлов, а если их нет — самоподписанный (только для разработки).
func LoadTLS(certFile, keyFile, base string) (*tls.Config, error) {
	if certFile != "" {
		r := &certReloader{certFile: certFile, keyFile: keyFile}
		if err := r.load(); err == nil {
			return &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: r.GetCertificate}, nil
		} else {
			logf("сертификат %s недоступен (%v) — использую самоподписанный", certFile, err)
		}
	}
	cert, err := selfSigned(base)
	if err != nil {
		return nil, err
	}
	logf("DoT работает с самоподписанным сертификатом (только для разработки)")
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}, nil
}

func selfSigned(base string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	now := time.Now()
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject:      pkix.Name{CommonName: "dns." + base},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"dns." + base, "*.dns." + base},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}
