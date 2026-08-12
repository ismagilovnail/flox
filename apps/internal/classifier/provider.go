// Package classifier turns a raw request (IP, User-Agent, headers) into
// the normalized attributes internal/routing's filters evaluate against
// (§40, Phase 20): country, region, city, device, platform, os, os_version,
// browser, browser_version, language, bot, proxy, asn, connection_type.
//
// External signal (geo/ASN/bot lookups) sits behind interfaces (§74/§75,
// CLAUDE.md non-negotiable #11 — "no vendor lock-in"); User-Agent parsing
// into the small fixed vocabulary the frontend and routing.FilterField
// already use (mobile/desktop/tablet, ios/android/windows/macos/linux,
// chrome/safari/firefox/edge/samsung_internet/other) is done locally with
// Go's stdlib regexp (RE2 only, CLAUDE.md non-negotiable #8) — no external
// UA-parsing dependency, since the target vocabulary is already this
// small and fixed.
package classifier

import (
	"context"
	"net"
)

type GeoResult struct {
	Country string // ISO 3166-1 alpha-2, e.g. "US" — never "UK" (see routing's ISO-mismatch conformance case)
	Region  string
	City    string
}

type GeoProvider interface {
	Lookup(ctx context.Context, ip net.IP) (GeoResult, error)
}

type ASNResult struct {
	ASN string
}

type ASNProvider interface {
	Lookup(ctx context.Context, ip net.IP) (ASNResult, error)
}

type BotInput struct {
	IP        net.IP
	UserAgent string
}

type BotResult struct {
	IsBot   bool
	IsProxy bool
}

type BotDetector interface {
	Detect(ctx context.Context, in BotInput) (BotResult, error)
}
