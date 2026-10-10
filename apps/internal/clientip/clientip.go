// Package clientip resolves a request's real client IP, honouring
// X-Forwarded-For only when the request's immediate peer is a configured
// trusted proxy.
//
// X-Forwarded-For's leftmost entry is written by the client. Trusting it
// would give unlimited password attempts past the per-IP auth rate limit
// (§54), satisfy the org IP allowlist (§30 Security) with a claimed
// address, and let a visitor pick its own A/B arm and geo in the tracker
// (the visit hash, §38, and the geo filters read the IP).
//
// chi's middleware.RealIP has that flaw (GHSA-3fxj-6jh8-hvhx,
// GHSA-rjr7-jggh-pgcp, GHSA-9g5q-2w5x-hmxf); the
// ClientIPFromXFFTrustedProxies its doc points at is not in chi v5
// (checked at v5.3.2).
//
// # The algorithm
//
// X-Forwarded-For grows left to right: each proxy APPENDS the address it
// received the request from. So entries near the right were written by
// infrastructure closest to this server, and entries near the left are
// whatever the original client chose to send. Trusting the leftmost
// entry trusts the client; the correct read is the RIGHTMOST entry that
// did not come from a proxy we trust.
//
//  1. Take the peer address from RemoteAddr.
//  2. If the peer is NOT a trusted proxy, return it and ignore the
//     header entirely — a direct connection's RemoteAddr is the one
//     thing a client cannot forge.
//  3. Otherwise walk X-Forwarded-For right to left and return the first
//     address that is not itself a trusted proxy.
//  4. If every entry is trusted (an all-internal chain), return the
//     leftmost one — the closest to the client — falling back to the
//     peer if the header holds nothing parseable.
//
// Step 2 is what makes the rest safe: steps 3 and 4 are only ever
// reached for a connection that physically originated inside the trusted
// set, so spoofing the header requires already being inside it.
package clientip

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
)

// DefaultTrustedProxies is used when TRUSTED_PROXIES is unset: loopback
// plus the RFC 1918 / RFC 4193 private ranges, which is where a load
// balancer, ingress controller or sidecar sits in essentially every real
// deployment.
//
// It is safe as a default precisely because of step 2 above: a request
// arriving from the public internet has a public peer address, which is
// not in this set, so its X-Forwarded-For is ignored no matter what it
// claims. Deliberately NOT included: link-local (169.254.0.0/16,
// fe80::/10) — nothing legitimately proxies from there, and every range
// added here is spoofing surface.
var DefaultTrustedProxies = []string{
	"127.0.0.0/8",
	"::1/128",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"fc00::/7",
}

// TrustNothing is the literal TRUSTED_PROXIES value that disables
// X-Forwarded-For entirely, so RemoteAddr is always the answer. For a
// deployment with no proxy in front of it at all.
const TrustNothing = "none"

type Resolver struct {
	trusted []netip.Prefix
	logger  *slog.Logger
	warn    sync.Once
}

// New builds a Resolver from a list of CIDR blocks or bare IP addresses.
// An empty list means DefaultTrustedProxies; the single entry "none"
// means trust nothing.
//
// A malformed entry is a hard error rather than a skipped line: silently
// dropping one would quietly widen or narrow a security boundary, and a
// typo in TRUSTED_PROXIES should stop the process at boot the same way a
// missing encryption key already does.
func New(entries []string, logger *slog.Logger) (*Resolver, error) {
	if len(entries) == 0 {
		entries = DefaultTrustedProxies
	}
	if len(entries) == 1 && strings.EqualFold(strings.TrimSpace(entries[0]), TrustNothing) {
		entries = nil
	}

	prefixes := make([]netip.Prefix, 0, len(entries))
	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			p, err := netip.ParsePrefix(entry)
			if err != nil {
				return nil, fmt.Errorf("clientip: %q is not a valid CIDR block: %w", entry, err)
			}
			prefixes = append(prefixes, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			return nil, fmt.Errorf("clientip: %q is neither a CIDR block nor an IP address: %w", entry, err)
		}
		addr = addr.Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return &Resolver{trusted: prefixes, logger: logger}, nil
}

// ParseList splits a TRUSTED_PROXIES-style comma-separated value.
func ParseList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Addr resolves the request's client address. The zero netip.Addr means
// nothing usable could be determined, which callers should treat as "no
// IP" rather than as a valid key.
func (rs *Resolver) Addr(r *http.Request) netip.Addr {
	peer, ok := parseAddr(hostOf(r.RemoteAddr))
	if !ok {
		return netip.Addr{}
	}
	if !rs.isTrusted(peer) {
		// A direct client. Its X-Forwarded-For, if any, is unverifiable —
		// warn once, since a proxied deployment that forgot to configure
		// TRUSTED_PROXIES lands here on every request and would otherwise
		// silently collapse every visitor onto the proxy's own address.
		if r.Header.Get("X-Forwarded-For") != "" && rs.logger != nil {
			rs.warn.Do(func() {
				rs.logger.Warn("ignoring X-Forwarded-For from an untrusted peer — if this server sits behind a proxy or CDN, set TRUSTED_PROXIES to its address range, otherwise every client will share one rate-limit bucket",
					"peer", peer.String())
			})
		}
		return peer
	}

	entries := forwardedFor(r)
	for i := len(entries) - 1; i >= 0; i-- {
		addr, ok := parseAddr(entries[i])
		if !ok {
			continue
		}
		if !rs.isTrusted(addr) {
			return addr
		}
	}
	// Every hop was trusted: the leftmost entry is the closest thing to
	// an original client this chain describes.
	for _, e := range entries {
		if addr, ok := parseAddr(e); ok {
			return addr
		}
	}
	return peer
}

// String is Addr rendered for use as a map/Redis key. It returns "" when
// no address could be resolved — callers keying a rate limit on this
// should treat "" as its own bucket rather than as a per-client one.
func (rs *Resolver) String(r *http.Request) string {
	addr := rs.Addr(r)
	if !addr.IsValid() {
		return ""
	}
	return addr.String()
}

// NetIP is Addr as a net.IP, for callers still on the older stdlib type
// (apps/internal/classifier's Geo/ASN providers take net.IP).
func (rs *Resolver) NetIP(r *http.Request) net.IP {
	addr := rs.Addr(r)
	if !addr.IsValid() {
		return nil
	}
	return net.IP(addr.AsSlice())
}

func (rs *Resolver) isTrusted(addr netip.Addr) bool {
	for _, p := range rs.trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func forwardedFor(r *http.Request) []string {
	var out []string
	// Multiple X-Forwarded-For headers are equivalent to one
	// comma-joined header (RFC 9110 §5.3), and net/http keeps them
	// separate — join them in order rather than reading only the first.
	for _, header := range r.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(header, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// parseAddr accepts a bare address, a bracketed IPv6 literal, or either
// with a port attached — all three appear in real X-Forwarded-For
// headers. IPv4-mapped IPv6 is unmapped so ::ffff:1.2.3.4 and 1.2.3.4
// resolve to one identity: leaving them distinct would hand out two
// rate-limit buckets to the same client.
func parseAddr(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Addr{}, false
	}
	if addr, err := netip.ParseAddr(s); err == nil {
		return addr.Unmap(), true
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap(), true
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		if addr, err := netip.ParseAddr(s[1 : len(s)-1]); err == nil {
			return addr.Unmap(), true
		}
	}
	return netip.Addr{}, false
}

func hostOf(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}
