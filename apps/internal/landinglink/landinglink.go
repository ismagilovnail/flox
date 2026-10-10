// Package landinglink signs and verifies the click URL an internal
// landing's `trackUrl` points at (docs/landings.md, "Serving visitors").
//
// The tracker serves a landing in place of redirecting, so the visitor's
// next step — the click through to the offer — arrives as a separate
// request (GET /lc/{token}) with no server-side state to look up: click
// rows reach ClickHouse asynchronously and may not exist yet when the
// visitor clicks a second later, and Redis is a cache only (CLAUDE.md #4).
// So everything that request needs travels in the token itself.
//
// It must be signed. Unsigned, /lc/ would be an open redirect on every
// customer's own tracking domain (the destination is in the token) and
// anyone could mint LAND_CLICK events into any organization's analytics.
// HMAC-SHA256 under a deployment key (LANDING_LINK_KEY), constant-time
// compare, and a maximum age so a token copied out of a page doesn't stay
// redeemable forever.
package landinglink

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// MaxAge bounds how long after the landing was served its click link is
// honoured. Generous on purpose — a visitor may leave a landing open in a
// tab for days — but finite.
const MaxAge = 30 * 24 * time.Hour

var ErrInvalid = errors.New("landinglink: invalid or expired token")

// Claims is what one served landing's click link carries: enough to
// record a LAND_CLICK on the same attribution chain as the SOURCE_CLICK
// that served it, and where to send the visitor next. Short JSON keys keep
// the URL compact.
type Claims struct {
	OrganizationID string `json:"o"`
	CampaignID     string `json:"c"`
	ClickID        string `json:"k"`
	StreamSetID    string `json:"s,omitempty"`
	FlowID         string `json:"f,omitempty"`
	LandingID      string `json:"l"`
	Destination    string `json:"d"`
	OfferID        string `json:"of,omitempty"`
	// TrafficSourceID is the click's campaign's traffic source, carried so
	// onward events (LAND_CLICK, PWA_*) stay on the same `source` bucket.
	TrafficSourceID string `json:"ts,omitempty"`
	// PwaID is the flow's PWA stage (internal type) — set when the visitor
	// is to be shown that PWA's install page after this step.
	PwaID string `json:"p,omitempty"`
	// Test marks a link minted by the PWA constructor's "open test page"
	// (apps/api), not by a real click: the tracker serves it normally but
	// records no events, so testing never pollutes analytics.
	Test          bool  `json:"tst,omitempty"`
	ConfigVersion int64 `json:"v,omitempty"`
	// Host is the host the link was issued for, lowercase and without a
	// port: the tracking domain the visitor came in on, or the tracker's
	// public host for a test link. The tracker honours the link only on
	// that host — it serves every organization's domains, and a link
	// valid anywhere would let one organization show its PWA page or
	// redirect from another's domain (tech audit M4).
	Host     string `json:"h"`
	IssuedAt int64  `json:"t"` // unix seconds

	// Pass-through attribution parameters from the original click, so a
	// LAND_CLICK row filters by utm/sub exactly like its SOURCE_CLICK.
	// Geo/device are re-derived from the click request instead (same
	// visitor, same IP/UA) — no need to carry them.
	UTMSource       string   `json:"us,omitempty"`
	UTMMedium       string   `json:"um,omitempty"`
	UTMCampaign     string   `json:"uc,omitempty"`
	UTMContent      string   `json:"ut,omitempty"`
	UTMTerm         string   `json:"uk,omitempty"`
	Subs            []string `json:"sb,omitempty"` // sub1..subN, trailing empties trimmed
	ExternalClickID string   `json:"x,omitempty"`
	FBClickID       string   `json:"fb,omitempty"`
	TTClickID       string   `json:"tt,omitempty"`
	Referrer        string   `json:"r,omitempty"`
}

type Signer struct {
	key []byte
}

// ParseKey decodes a base64 key (the same "32 raw bytes, base64-encoded"
// shape every other key in config.Config uses).
func ParseKey(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, errors.New("landinglink: LANDING_LINK_KEY is not set")
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("landinglink: LANDING_LINK_KEY is not valid base64: %w", err)
	}
	if len(key) < 32 {
		return nil, fmt.Errorf("landinglink: LANDING_LINK_KEY must decode to at least 32 bytes, got %d", len(key))
	}
	return key, nil
}

func NewSigner(key []byte) *Signer { return &Signer{key: key} }

// Sign returns "<payload>.<signature>", both base64url without padding —
// safe as a single URL path segment.
func (s *Signer) Sign(c Claims) string {
	raw, _ := json.Marshal(c) // only strings/ints/slices: cannot fail
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + base64.RawURLEncoding.EncodeToString(s.mac(payload))
}

// Verify checks the signature, then the age, then decodes. Every failure
// is the same ErrInvalid: a caller has nothing useful to tell a visitor
// holding a bad link beyond "not found".
func (s *Signer) Verify(token string, now time.Time) (Claims, error) {
	return s.verify(token, now, true)
}

// VerifyAnyAge is Verify without MaxAge — only for the routes an
// installed PWA keeps calling for as long as it stays installed (its
// manifest and its start_url). The signature is still checked: age is
// the only thing relaxed, so nothing unsigned ever gets through.
func (s *Signer) VerifyAnyAge(token string, now time.Time) (Claims, error) {
	return s.verify(token, now, false)
}

func (s *Signer) verify(token string, now time.Time, checkAge bool) (Claims, error) {
	payload, sig, ok := strings.Cut(token, ".")
	if !ok || payload == "" || sig == "" {
		return Claims{}, ErrInvalid
	}
	gotMAC, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(gotMAC, s.mac(payload)) {
		return Claims{}, ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Claims{}, ErrInvalid
	}
	var c Claims
	if err := json.Unmarshal(raw, &c); err != nil {
		return Claims{}, ErrInvalid
	}
	issued := time.Unix(c.IssuedAt, 0)
	if c.IssuedAt == 0 || issued.Sub(now) > time.Minute {
		return Claims{}, ErrInvalid
	}
	if checkAge && now.Sub(issued) > MaxAge {
		return Claims{}, ErrInvalid
	}
	if c.OrganizationID == "" || c.ClickID == "" || c.Destination == "" || c.Host == "" {
		return Claims{}, ErrInvalid
	}
	return c, nil
}

// NormalizeHost is how a Claims.Host is written and compared: lowercase,
// without a port.
func NormalizeHost(hostport string) string {
	h := hostport
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		h = host
	}
	return strings.ToLower(h)
}

// OnHost reports whether c may be used on a request to hostport.
func (c Claims) OnHost(hostport string) bool {
	return c.Host != "" && c.Host == NormalizeHost(hostport)
}

func (s *Signer) mac(payload string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(payload))
	return m.Sum(nil)
}

// TrimSubs drops trailing empty subs so an all-empty sub1..sub10 (the
// common case) costs nothing in the token.
func TrimSubs(subs [10]string) []string {
	n := len(subs)
	for n > 0 && subs[n-1] == "" {
		n--
	}
	if n == 0 {
		return nil
	}
	return append([]string(nil), subs[:n]...)
}

// ExpandSubs is TrimSubs' inverse.
func ExpandSubs(subs []string) [10]string {
	var out [10]string
	copy(out[:], subs)
	return out
}
