package conversion

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/url"
	"time"
)

// ParseParams builds the Postback an incoming postback's parameters
// describe — the query string of a GET, or with a POST's urlencoded body
// merged in (http.Request.Form) — for network n. apps/tracker's postback
// endpoint and the API's dry run (Service.Check) both parse here, so the
// dry run reads a URL exactly as the live endpoint would.
//
// The tenant scope, the dedup override, the payout currency and the
// outgoing template come from n, never from the parameters (CLAUDE.md #5).
// Without event_time (or ts) the conversion happened at now. A bad revenue
// or event_time is an error (ErrInvalidRevenue, ErrInvalidEventTime): the
// same request will never become valid, so the endpoint answers 400.
func ParseParams(form url.Values, n Network, now time.Time) (Postback, error) {
	p := Postback{
		OrganizationID:   n.OrganizationID,
		NetworkID:        n.ID,
		AcceptDuplicates: n.AcceptDuplicates,
		ClickID:          form.Get("click_id"),
		ExternalClickID:  form.Get("external_click_id"),
		RawStatus:        form.Get("status"),
		Currency:         form.Get("currency"),
		NetworkCurrency:  n.Currency,
		NetworkTxnID:     firstNonEmpty(form.Get("txn_id"), form.Get("transaction_id")),
		PostbackURL:      n.PostbackURL,
		OccurredAt:       now.UTC(),
	}

	// When the conversion happened at the network (tech audit T3); without
	// it, the moment the postback arrived.
	if raw := firstNonEmpty(form.Get("event_time"), form.Get("ts")); raw != "" {
		at, err := ParseEventTime(raw, p.OccurredAt)
		if err != nil {
			return Postback{}, err
		}
		p.OccurredAt = at
	}

	if raw := form.Get("revenue"); raw != "" {
		v, err := ParseRevenue(raw)
		if err != nil {
			return Postback{}, err
		}
		p.Revenue = &v
	}
	return p, nil
}

// ValidSecret reports whether provided is the secret whose hash a network
// stores (§54). It hashes the input and compares in constant time
// (subtle.ConstantTimeCompare) — an early-exit == comparison would leak
// how many leading bytes matched through response timing, letting an
// attacker recover the secret byte by byte. An empty stored hash matches
// nothing, so a network without a secret refuses every postback.
func ValidSecret(provided, storedHash string) bool {
	if provided == "" || storedHash == "" {
		return false
	}
	sum := sha256.Sum256([]byte(provided))
	providedHash := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(providedHash), []byte(storedHash)) == 1
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
