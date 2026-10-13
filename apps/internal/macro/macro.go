// Package macro is the Go half of §27's cross-cutting macro/placeholder
// substitution system — one resolver, reused everywhere a URL or payload
// template needs runtime values. It ports the token contract already
// shipping in apps/web/src/lib/macros.ts (offer links, postback templates,
// pixel payloads) rather than inventing a second one; do not fork a new
// token list here.
//
// Two callers: outgoing postback delivery (§46) and apps/tracker, which
// resolves a flow's offer/redirect URL and an external landing's URL at
// click time, so a network gets the click id it must echo back in its
// postback (tech audit T7). {payout} and {source} are part of the shared token
// vocabulary (the frontend's token picker lists them) but neither caller
// has them, so this resolver does not recognize them. They pass through
// literally, same as a typo would — see Resolve's doc for why that's the
// right behavior rather than blanking them.
package macro

import (
	"net/url"
	"regexp"
	"strconv"
)

// Values is what a template gets resolved against. Every field here IS
// resolved — set one to "" for "known, legitimately empty" (e.g. Revenue on
// a CPA_HOLD registration ping), which substitutes as a blank, not as the
// literal token. A token this package doesn't know about at all ({payout},
// a typo, a future macro) is a different case: see the package doc.
type Values struct {
	ClickID    string
	Status     string
	Revenue    string
	Currency   string
	CampaignID string
	OfferID    string
	Country    string
	Device     string
	Subs       [10]string
	// NextURL is the signed link a postlanding page sends the visitor on
	// with (apps/tracker/postlanding.go). Only a postlanding URL is resolved
	// with it set; anywhere else {next_url} is known-but-empty.
	NextURL string
}

var token = regexp.MustCompile(`\{[a-z0-9_]+\}`)

// Resolve replaces every {token} in template that this package recognizes
// with its value from v, including substituting "" for a recognized-but-
// empty field. A token NOT in the recognized set — {payout}, a typo, or any
// future macro this resolver doesn't yet implement — is left exactly as
// written. This must never panic or error on a partial Values (mirrors
// resolveMacros' contract on the TypeScript side): a best-effort delivery
// has to render something even when not every field is known.
func Resolve(template string, v Values) string {
	return resolve(template, v, func(s string) string { return s })
}

// ResolveURL is Resolve for a URL template: every substituted value is
// query-escaped. Values such as sub1..sub10 come from the click's own query
// string, so anyone can set them; unescaped, sub1=x%26click_id%3Devil would
// inject a second click_id (or a '#' cut the URL short) in the URL FLOX
// sends the visitor or the partner to.
func ResolveURL(template string, v Values) string {
	return resolve(template, v, url.QueryEscape)
}

func resolve(template string, v Values, escape func(string) string) string {
	fields := map[string]string{
		"{click_id}":    v.ClickID,
		"{status}":      v.Status,
		"{revenue}":     v.Revenue,
		"{currency}":    v.Currency,
		"{campaign_id}": v.CampaignID,
		"{offer_id}":    v.OfferID,
		"{country}":     v.Country,
		"{device}":      v.Device,
		"{next_url}":    v.NextURL,
	}
	for i, s := range v.Subs {
		fields["{sub"+strconv.Itoa(i+1)+"}"] = s
	}

	return token.ReplaceAllStringFunc(template, func(t string) string {
		if val, ok := fields[t]; ok {
			return escape(val)
		}
		return t
	})
}
