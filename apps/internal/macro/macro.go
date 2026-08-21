// Package macro is the Go half of §27's cross-cutting macro/placeholder
// substitution system — one resolver, reused everywhere a URL or payload
// template needs runtime values. It ports the token contract already
// shipping in apps/web/src/lib/macros.ts (offer links, postback templates,
// pixel payloads) rather than inventing a second one; do not fork a new
// token list here.
//
// Phase 24 is the first Go caller (outgoing postback delivery, §46) and
// only has click/conversion-level data to resolve from — {payout},
// {offer_id} and {source} are part of the shared token vocabulary (the
// frontend's token picker still lists them) but nothing populates a Flow's
// destination offer or a click's traffic source yet, so this resolver
// simply doesn't recognize those three tokens. They pass through literally,
// same as a typo would — see Resolve's doc for why that's the right
// behavior rather than blanking them.
package macro

import (
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
	Country    string
	Device     string
	Subs       [10]string
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
	fields := map[string]string{
		"{click_id}":    v.ClickID,
		"{status}":      v.Status,
		"{revenue}":     v.Revenue,
		"{currency}":    v.Currency,
		"{campaign_id}": v.CampaignID,
		"{country}":     v.Country,
		"{device}":      v.Device,
	}
	for i, s := range v.Subs {
		fields["{sub"+strconv.Itoa(i+1)+"}"] = s
	}

	return token.ReplaceAllStringFunc(template, func(t string) string {
		if val, ok := fields[t]; ok {
			return val
		}
		return t
	})
}
