package classifier

import (
	"context"
	"net"
	"strings"
)

// NoopGeoProvider is the default until a real vendor (MaxMind, IPinfo, ...)
// is wired up in a later integration phase — honestly returns "unknown"
// (empty) rather than fabricating a country/region/city, per CLAUDE.md's
// "no fake APIs that look real."
type NoopGeoProvider struct{}

func (NoopGeoProvider) Lookup(ctx context.Context, ip net.IP) (GeoResult, error) {
	return GeoResult{}, nil
}

// NoopASNProvider is the equivalent default for ASN lookups.
type NoopASNProvider struct{}

func (NoopASNProvider) Lookup(ctx context.Context, ip net.IP) (ASNResult, error) {
	return ASNResult{}, nil
}

// HeuristicBotDetector flags well-known crawlers/automation tools by
// User-Agent substring — a generic, provider-neutral technique (§73:
// "classification stays provider-agnostic; no platform-specific behavior
// hard-coded into routing"), explicitly NOT ad-network moderator/reviewer
// detection, which is forbidden. IsProxy is always false here: reliable
// proxy detection needs an IP-reputation vendor this project doesn't have
// wired up yet, and guessing would be exactly the kind of fake signal
// CLAUDE.md forbids. A real BotDetector (a paid fraud/bot vendor) replaces
// this wholesale in a later integration phase; this default exists so
// classification works meaningfully before that vendor exists.
type HeuristicBotDetector struct{}

// knownBotSubstrings are generic crawler/automation signatures, not tied
// to any single advertising platform.
var knownBotSubstrings = []string{
	"bot", "spider", "crawl", "slurp", "curl/", "wget/", "python-requests",
	"python-urllib", "go-http-client", "headlesschrome", "phantomjs",
	"facebookexternalhit", "preview", "monitor", "pingdom", "uptimerobot",
}

func (HeuristicBotDetector) Detect(ctx context.Context, in BotInput) (BotResult, error) {
	ua := strings.ToLower(in.UserAgent)
	for _, s := range knownBotSubstrings {
		if strings.Contains(ua, s) {
			return BotResult{IsBot: true}, nil
		}
	}
	return BotResult{}, nil
}
