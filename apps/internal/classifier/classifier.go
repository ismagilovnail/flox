package classifier

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/routing"
)

// Input is the raw signal a request carries — everything classification
// starts from. utm_*/sub1-10/referrer/external_click_id are deliberately
// absent: §40 only asks this package to produce the 12 normalized fields
// listed there (plus os_version/browser_version, see doc comment below);
// those other routing.Attributes fields pass through raw from query
// params, with no classification step, and are the caller's (apps/
// tracker's) job to populate directly.
type Input struct {
	IP             net.IP
	UserAgent      string
	AcceptLanguage string
}

// Classifier wires the pluggable external providers (§74/§75 — no vendor
// lock-in) together with local User-Agent parsing to produce
// routing.Attributes. Zero-value providers are replaced with honest
// defaults by New — see defaults.go.
type Classifier struct {
	Geo GeoProvider
	ASN ASNProvider
	Bot BotDetector
}

func New(geo GeoProvider, asn ASNProvider, bot BotDetector) *Classifier {
	if geo == nil {
		geo = NoopGeoProvider{}
	}
	if asn == nil {
		asn = NoopASNProvider{}
	}
	if bot == nil {
		bot = HeuristicBotDetector{}
	}
	return &Classifier{Geo: geo, ASN: asn, Bot: bot}
}

// Classify produces routing.Attributes keyed with the exact
// routing.FilterField constants stream-set filters already evaluate
// against — the classifier and the filter builder can never disagree
// about a field's name, because there is only one definition of it
// (routing.FilterField), imported here rather than redeclared.
//
// §40 lists 12 fields (country, region, city, device, platform, os,
// browser, language, bot, proxy, asn, connection). This also populates
// os_version/browser_version: they're already real, filterable
// routing.FilterFields the frontend's filter builder has exposed since
// Phase 8, and a User-Agent parse that produces "os" without "os_version"
// alongside it would leave two long-standing filter fields permanently
// dead for no reason — §40's list reads as representative, not
// exhaustive-to-the-exclusion of fields the rest of the system already
// depends on.
func (c *Classifier) Classify(ctx context.Context, in Input) (routing.Attributes, error) {
	geo, err := c.Geo.Lookup(ctx, in.IP)
	if err != nil {
		return nil, fmt.Errorf("geo lookup: %w", err)
	}
	asn, err := c.ASN.Lookup(ctx, in.IP)
	if err != nil {
		return nil, fmt.Errorf("asn lookup: %w", err)
	}
	bot, err := c.Bot.Detect(ctx, BotInput{IP: in.IP, UserAgent: in.UserAgent})
	if err != nil {
		return nil, fmt.Errorf("bot detection: %w", err)
	}

	ua := ParseUserAgent(in.UserAgent)

	return routing.Attributes{
		routing.FieldCountry:        geo.Country,
		routing.FieldRegion:         geo.Region,
		routing.FieldCity:           geo.City,
		routing.FieldDevice:         ua.Device,
		routing.FieldPlatform:       ua.Platform,
		routing.FieldOS:             ua.OS,
		routing.FieldOSVersion:      ua.OSVersion,
		routing.FieldBrowser:        ua.Browser,
		routing.FieldBrowserVersion: ua.BrowserVersion,
		routing.FieldLanguage:       primaryLanguage(in.AcceptLanguage),
		routing.FieldBot:            boolFlag(bot.IsBot),
		routing.FieldProxy:          boolFlag(bot.IsProxy),
		routing.FieldASN:            asn.ASN,
		// No reliable server-side signal distinguishes wifi/cellular/
		// ethernet without either a paid network-intelligence vendor or a
		// client-side JS beacon (neither exists yet) — "unknown" is
		// honest, not a guess. Matches the frontend's own FIELD_VOCAB
		// option of the same name.
		routing.FieldConnectionType: "unknown",
	}, nil
}

func boolFlag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// primaryLanguage takes the first tag from an Accept-Language header
// ("en-US,en;q=0.9,de;q=0.8" -> "en-US"), ignoring quality values.
func primaryLanguage(acceptLanguage string) string {
	first := strings.SplitN(acceptLanguage, ",", 2)[0]
	first = strings.SplitN(first, ";", 2)[0]
	return strings.TrimSpace(first)
}
