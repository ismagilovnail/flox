package tlsprobe_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/domain/tlsprobe"
)

type authority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pool *x509.CertPool
}

func newAuthority(t *testing.T) authority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test root"},
		NotBefore:             time.Now().Add(-96 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return authority{cert: cert, key: key, pool: pool}
}

func (a authority) leaf(t *testing.T, name string, notAfter time.Time) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: name},
		DNSNames:     []string{name},
		NotBefore:    time.Now().Add(-72 * time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, a.cert, &key.PublicKey, a.key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// serve accepts TLS connections with cert and returns the address.
func serve(t *testing.T, cert tls.Certificate) string {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_ = c.(*tls.Conn).Handshake()
				c.Close()
			}()
		}
	}()
	return ln.Addr().String()
}

func probe(roots *x509.CertPool, addr string) tlsprobe.Result {
	p := &tlsprobe.Prober{Roots: roots, Addr: func(string) string { return addr }, Timeout: 3 * time.Second}
	return p.Probe(context.Background(), "track.example.com")
}

func TestProbe(t *testing.T) {
	ca := newAuthority(t)
	expiry := time.Now().Add(60 * 24 * time.Hour).Truncate(time.Second)

	t.Run("valid certificate is issued with its expiry", func(t *testing.T) {
		got := probe(ca.pool, serve(t, ca.leaf(t, "track.example.com", expiry)))
		if got.Outcome != tlsprobe.Issued || got.ExpiresAt == nil || !got.ExpiresAt.Equal(expiry) || got.Err != "" {
			t.Fatalf("got %+v, want issued expiring %s", got, expiry)
		}
	})

	t.Run("expired certificate is an error that still reports its expiry", func(t *testing.T) {
		past := time.Now().Add(-time.Hour).Truncate(time.Second)
		got := probe(ca.pool, serve(t, ca.leaf(t, "track.example.com", past)))
		if got.Outcome != tlsprobe.Error || got.ExpiresAt == nil || !got.ExpiresAt.Equal(past) || !strings.Contains(got.Err, "expired") {
			t.Fatalf("got %+v, want error with the past expiry", got)
		}
	})

	t.Run("certificate for another name is an error", func(t *testing.T) {
		got := probe(ca.pool, serve(t, ca.leaf(t, "other.example.com", expiry)))
		if got.Outcome != tlsprobe.Error || !strings.Contains(got.Err, "another name") {
			t.Fatalf("got %+v, want a hostname error", got)
		}
	})

	t.Run("untrusted authority is an error", func(t *testing.T) {
		got := probe(newAuthority(t).pool, serve(t, ca.leaf(t, "track.example.com", expiry)))
		if got.Outcome != tlsprobe.Error || !strings.Contains(got.Err, "trusted authority") {
			t.Fatalf("got %+v, want an unknown-authority error", got)
		}
	})

	t.Run("listener without a certificate is pending", func(t *testing.T) {
		// What an on-demand proxy does for a name it will not (yet) issue
		// for: accept the connection, then fail the handshake.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				c.Close()
			}
		}()
		got := probe(ca.pool, ln.Addr().String())
		if got.Outcome != tlsprobe.Pending || got.ExpiresAt != nil || got.Err == "" {
			t.Fatalf("got %+v, want pending", got)
		}
	})

	t.Run("nothing listening is none", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := ln.Addr().String()
		ln.Close()
		got := probe(ca.pool, addr)
		if got.Outcome != tlsprobe.None || got.Err == "" {
			t.Fatalf("got %+v, want none", got)
		}
	})
}
