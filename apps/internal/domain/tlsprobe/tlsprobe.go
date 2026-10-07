// Package tlsprobe checks what certificate a domain actually serves: it
// opens a TLS connection to the domain on port 443, the way a visitor's
// browser would, and verifies the chain against the system roots and the
// hostname. Marketer audit 2026-10-06, #4.
//
// It knows nothing about who issues the certificate. Behind a proxy with
// on-demand certificates (Caddy, docs/deployment.md) the first connection
// is what triggers issuance, so a probe of a freshly pointed domain may see
// "pending" and the next one "issued".
package tlsprobe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"time"
)

// Outcome is what the probe concluded. It maps one-to-one onto
// domain.SSL, which this package does not import.
type Outcome string

const (
	// Issued: a valid certificate for this hostname, chained to a trusted
	// root and not expired.
	Issued Outcome = "issued"
	// Pending: the domain answers on 443 but completed no handshake with a
	// certificate — typically the proxy has not issued one yet, or refused
	// to because the domain is not registered with it.
	Pending Outcome = "pending"
	// None: nothing answers on 443 — the domain does not resolve, or does
	// not point at a server listening there.
	None Outcome = "none"
	// Error: a certificate is served but a browser would reject it
	// (expired, self-signed, issued for another name).
	Error Outcome = "error"
)

type Result struct {
	Outcome Outcome
	// ExpiresAt is the served leaf certificate's NotAfter, set whenever a
	// certificate was received — also for Error, so an expired one shows
	// when it expired.
	ExpiresAt *time.Time
	// Err says why the outcome is not Issued; empty for Issued.
	Err string
}

// Prober is safe for concurrent use. The zero value probes port 443 with
// the system roots and a 10-second timeout.
type Prober struct {
	// Roots overrides the system roots (tests).
	Roots *x509.CertPool
	// Addr maps a hostname to the address to dial; nil means host:443.
	// Tests point it at a local server.
	Addr    func(host string) string
	Timeout time.Duration
}

const defaultTimeout = 10 * time.Second

func (p *Prober) Probe(ctx context.Context, host string) Result {
	timeout := p.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addr := net.JoinHostPort(host, "443")
	if p.Addr != nil {
		addr = p.Addr(host)
	}

	var d net.Dialer
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return Result{Outcome: None, Err: err.Error()}
	}
	defer raw.Close()

	// Verification is done by hand below, after the handshake, so a
	// certificate that fails it is still available to report its expiry.
	conn := tls.Client(raw, &tls.Config{ServerName: host, InsecureSkipVerify: true}) //nolint:gosec // verified explicitly below
	if err := conn.HandshakeContext(ctx); err != nil {
		return Result{Outcome: Pending, Err: err.Error()}
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return Result{Outcome: Pending, Err: "no certificate presented"}
	}
	leaf := certs[0]
	expires := leaf.NotAfter
	intermediates := x509.NewCertPool()
	for _, c := range certs[1:] {
		intermediates.AddCert(c)
	}
	_, err = leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: p.Roots, Intermediates: intermediates})
	if err != nil {
		return Result{Outcome: Error, ExpiresAt: &expires, Err: describe(err)}
	}
	return Result{Outcome: Issued, ExpiresAt: &expires}
}

// describe turns x509's errors into what an operator can act on.
func describe(err error) string {
	var invalid x509.CertificateInvalidError
	var unknown x509.UnknownAuthorityError
	var host x509.HostnameError
	switch {
	case errors.As(err, &invalid) && invalid.Reason == x509.Expired:
		return "certificate expired or not yet valid: " + err.Error()
	case errors.As(err, &unknown):
		return "certificate is not from a trusted authority: " + err.Error()
	case errors.As(err, &host):
		return "certificate is for another name: " + err.Error()
	default:
		return err.Error()
	}
}
