// Package urlsafety is §54's SSRF defense for any outbound HTTP call this
// server makes to an operator-supplied URL — today, exclusively
// apps/internal/postback.Deliverer's postback delivery requests
// (apps/internal/adaccount/facebookads and .../tiktokads never take an
// operator-supplied host, only a hardcoded API hostname, so they aren't
// an SSRF vector and don't use this package).
//
// Two layers, both needed:
//
//   - ValidateURL: a cheap, save-time check (scheme + a literal-IP
//     rejection) — apps/internal/network.Service calls this when a
//     network's postback_url is created/updated, so an obviously bad URL
//     is rejected immediately with a clear validation error instead of
//     surfacing only much later as a silent delivery failure.
//   - SafeDialContext: the actual, authoritative defense, used as the
//     http.Transport.DialContext for the client apps/internal/postback.
//     Deliverer sends requests with. ValidateURL alone cannot be the real
//     protection — a hostname that resolves to a public IP at save time
//     can be repointed at a private/metadata IP by the time delivery
//     happens (DNS rebinding), and a save-time check has no way to catch
//     that. SafeDialContext resolves the host and validates the IP at the
//     moment of connection, then dials that exact validated IP (not the
//     hostname again), closing the gap between "checked" and "connected."
package urlsafety

import (
	"context"
	"fmt"
	"net"
	"net/url"
)

// ValidateURL is the save-time check: scheme must be http/https, and the
// host must not be a literal IP in a forbidden range (a literal, since a
// hostname's resolved IP can change later — DNS rebinding is
// SafeDialContext's job, not this one's).
func ValidateURL(rawURL string) error {
	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("URL has no host")
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil && isForbidden(ip) {
		return fmt.Errorf("URL host %q resolves to a forbidden address range", host)
	}
	return nil
}

// SafeDialContext is a drop-in http.Transport.DialContext. Set it on the
// http.Client any code makes operator-supplied-URL requests with:
//
//	client := &http.Client{Transport: &http.Transport{DialContext: urlsafety.SafeDialContext}}
func SafeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("urlsafety: parsing dial address %q: %w", addr, err)
	}

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("urlsafety: resolving %q: %w", host, err)
	}

	var allowed net.IP
	for _, ip := range ips {
		if isForbidden(ip) {
			return nil, fmt.Errorf("urlsafety: %q resolved to %s, a forbidden address range (private/loopback/link-local/metadata)", host, ip)
		}
		if allowed == nil {
			allowed = ip
		}
	}
	if allowed == nil {
		return nil, fmt.Errorf("urlsafety: %q did not resolve to any address", host)
	}

	// Dial the specific IP just validated, not the hostname again — a
	// second lookup here could return a different (rebound) address than
	// the one just checked, silently reopening the exact gap this
	// function exists to close.
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, net.JoinHostPort(allowed.String(), port))
}

// isForbidden covers loopback (127.0.0.0/8, ::1), RFC 1918 + RFC 4193
// private ranges (net.IP.IsPrivate, Go's own stdlib classification),
// link-local unicast/multicast (169.254.0.0/16 — which includes the
// 169.254.169.254 cloud-metadata address every major cloud provider
// uses — and fe80::/10), and the unspecified address (0.0.0.0/::).
func isForbidden(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}
