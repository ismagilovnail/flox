package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/routing"
)

// stickyCookie is the parsed sf_{campaignId} cookie (§39-STICKY), whose
// value is "setId:flowId[:clickId].signature".
//
// It carries ClickID, which routing.StickyState deliberately does not:
// the click_id affects attribution only, never which flow gets selected,
// so internal/routing has no business seeing it (documented in that
// package). The tracker parses the full cookie here and hands the engine
// only the two fields a routing decision actually depends on.
type stickyCookie struct {
	StreamSetID string
	FlowID      string
	ClickID     string
}

func stickyCookieName(campaignID string) string { return "sf_" + campaignID }

// stickySigner signs and checks sticky cookies (tech audit T5). Unsigned,
// anyone who saw a real visit's values could set one and skip the stream
// set's geo/bot filters straight into its flow, and with keepClickId write
// any click_id into the chain. A cookie the tracker did not issue is
// ignored: the visit is routed as a new one and gets a fresh, signed
// cookie.
//
// The signature covers the campaign too, so a cookie cannot be moved to
// another campaign; the key (STICKY_COOKIE_KEY) is shared by every tracker
// replica and must survive restarts — the cookie is the source of truth
// (CLAUDE.md #4), and rotating the key re-buckets every returning visitor.
type stickySigner struct{ key []byte }

// stickySigLen is the signature's length in the cookie: 16 bytes of
// HMAC-SHA256, base64url.
const stickySigLen = 22

func newStickySigner(encoded string) (*stickySigner, error) {
	if encoded == "" {
		return nil, errors.New("STICKY_COOKIE_KEY is not set")
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("STICKY_COOKIE_KEY is not valid base64: %w", err)
	}
	if len(key) < 32 {
		return nil, fmt.Errorf("STICKY_COOKIE_KEY must decode to at least 32 bytes, got %d", len(key))
	}
	return &stickySigner{key: key}, nil
}

func (s *stickySigner) sign(campaignID, value string) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte("sf|" + campaignID + "|" + value))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)[:16])
}

// Reasons a sticky cookie is ignored (flox_sticky_cookies_rejected_total).
const (
	stickyMalformed    = "malformed"
	stickyUnsigned     = "unsigned"
	stickyBadSignature = "bad_signature"
)

// parse reads the campaign's sticky cookie; nil when there is none or it
// is not one this tracker issued.
func (s *stickySigner) parse(r *http.Request, campaignID string) *stickyCookie {
	c, err := r.Cookie(stickyCookieName(campaignID))
	if err != nil || c.Value == "" {
		return nil
	}
	sc, reason := s.verify(campaignID, c.Value)
	if reason != "" {
		metrics.StickyCookiesRejectedTotal.WithLabelValues(reason).Inc()
		return nil
	}
	return sc
}

func (s *stickySigner) verify(campaignID, raw string) (*stickyCookie, string) {
	value, sig, ok := strings.Cut(raw, ".")
	if !ok {
		return nil, stickyUnsigned // issued before signing, or forged
	}
	if len(sig) != stickySigLen || !hmac.Equal([]byte(sig), []byte(s.sign(campaignID, value))) {
		return nil, stickyBadSignature
	}
	parts := strings.Split(value, ":")
	if len(parts) < 2 || len(parts) > 3 || !idgen.IsValid(parts[0]) || !idgen.IsValid(parts[1]) {
		return nil, stickyMalformed
	}
	sc := &stickyCookie{StreamSetID: parts[0], FlowID: parts[1]}
	if len(parts) == 3 {
		if !idgen.IsValid(parts[2]) {
			return nil, stickyMalformed
		}
		sc.ClickID = parts[2]
	}
	return sc, ""
}

// routingState narrows the cookie to just what the routing engine needs.
func (s *stickyCookie) routingState() *routing.StickyState {
	if s == nil {
		return nil
	}
	return &routing.StickyState{StreamSetID: s.StreamSetID, FlowID: s.FlowID}
}

// stickyMaxAge is deliberately long (§39-STICKY: the cookie is the source
// of truth and must "survive Redis eviction, restarts, and cross-session
// returns"). A short-lived cookie would silently corrupt A/B tests by
// letting returning users be re-bucketed.
const stickyMaxAge = 365 * 24 * 60 * 60 // one year, in seconds

func (s *stickySigner) set(w http.ResponseWriter, campaignID string, result routing.RouteResult, clickID string, keepClickID bool) {
	value := result.StreamSetID + ":" + result.FlowID
	if keepClickID {
		value += ":" + clickID
	}
	http.SetCookie(w, &http.Cookie{
		Name:     stickyCookieName(campaignID),
		Value:    value + "." + s.sign(campaignID, value),
		Path:     "/",
		MaxAge:   stickyMaxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
