package landinglink

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

var testKey = []byte("0123456789abcdef0123456789abcdef")

func sample(now time.Time) Claims {
	return Claims{
		OrganizationID: "org", CampaignID: "camp", ClickID: "click", FlowID: "flow", LandingID: "lnd",
		Destination: "https://offer.example/?a=1&b=2", IssuedAt: now.Unix(),
		UTMSource: "fb", Subs: TrimSubs([10]string{"s1", "", "s3"}),
	}
}

func TestSignVerifyRoundTrip(t *testing.T) {
	now := time.Now()
	s := NewSigner(testKey)
	tok := s.Sign(sample(now))
	if strings.ContainsAny(tok, "/+=?#&") {
		t.Fatalf("token isn't a clean URL path segment: %q", tok)
	}
	got, err := s.Verify(tok, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Destination != "https://offer.example/?a=1&b=2" || got.ClickID != "click" || ExpandSubs(got.Subs)[2] != "s3" {
		t.Fatalf("claims didn't round-trip: %+v", got)
	}
}

func TestVerifyRejects(t *testing.T) {
	now := time.Now()
	s := NewSigner(testKey)
	tok := s.Sign(sample(now))
	payload, sig, _ := strings.Cut(tok, ".")

	forged := sample(now)
	forged.Destination = "https://evil.example"
	forgedPayload, _, _ := strings.Cut(s.Sign(forged), ".")

	other := NewSigner([]byte("ffffffffffffffffffffffffffffffff")).Sign(sample(now))

	old := sample(now.Add(-MaxAge - time.Hour))
	future := sample(now.Add(time.Hour))

	cases := map[string]string{
		"empty":               "",
		"no dot":              payload,
		"swapped payload":     forgedPayload + "." + sig,
		"other key":           other,
		"garbage signature":   payload + "." + base64.RawURLEncoding.EncodeToString([]byte("nope")),
		"expired":             s.Sign(old),
		"issued in future":    s.Sign(future),
		"missing destination": s.Sign(Claims{OrganizationID: "o", ClickID: "k", IssuedAt: now.Unix()}),
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := s.Verify(tok, now); err == nil {
				t.Fatalf("Verify accepted %q", tok)
			}
		})
	}
}

func TestParseKey(t *testing.T) {
	if _, err := ParseKey(""); err == nil {
		t.Fatal("accepted an empty key")
	}
	if _, err := ParseKey("short"); err == nil {
		t.Fatal("accepted a non-base64 key")
	}
	if _, err := ParseKey(base64.StdEncoding.EncodeToString([]byte("too short"))); err == nil {
		t.Fatal("accepted a 9-byte key")
	}
	if _, err := ParseKey(base64.StdEncoding.EncodeToString(testKey)); err != nil {
		t.Fatalf("rejected a valid key: %v", err)
	}
}

func TestTrimSubs(t *testing.T) {
	if TrimSubs([10]string{}) != nil {
		t.Fatal("all-empty subs should trim to nil")
	}
	if got := TrimSubs([10]string{"a", "", "c"}); len(got) != 3 {
		t.Fatalf("TrimSubs = %v, want 3 entries", got)
	}
}
