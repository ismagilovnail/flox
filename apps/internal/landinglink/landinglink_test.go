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
		Destination: "https://offer.example/?a=1&b=2", IssuedAt: now.Unix(), Host: "t.example",
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

func TestVerifyAnyAgeOnlyRelaxesAge(t *testing.T) {
	now := time.Now()
	s := NewSigner(testKey)
	old := s.Sign(sample(now.Add(-MaxAge - 24*time.Hour)))
	if _, err := s.Verify(old, now); err == nil {
		t.Fatal("Verify accepted an expired token")
	}
	if _, err := s.VerifyAnyAge(old, now); err != nil {
		t.Fatalf("VerifyAnyAge rejected an old but validly signed token: %v", err)
	}
	other := NewSigner([]byte("ffffffffffffffffffffffffffffffff")).Sign(sample(now))
	if _, err := s.VerifyAnyAge(other, now); err == nil {
		t.Fatal("VerifyAnyAge accepted a token signed with another key")
	}
}

// A link counts only on the host it was issued for (tech audit M4).
func TestClaimsOnHost(t *testing.T) {
	c := Claims{Host: NormalizeHost("T.Example:8443")}
	for host, want := range map[string]bool{
		"t.example": true, "T.EXAMPLE": true, "t.example:80": true,
		"other.example": false, "t.example.evil": false, "": false,
	} {
		if got := c.OnHost(host); got != want {
			t.Errorf("OnHost(%q) = %v, want %v", host, got, want)
		}
	}
	if (Claims{}).OnHost("t.example") {
		t.Error("a link without a host is honoured")
	}
	if got := NormalizeHost("[::1]:8081"); got != "::1" {
		t.Errorf("NormalizeHost IPv6 = %q", got)
	}
}

func TestVerifyRejectsALinkWithoutHost(t *testing.T) {
	now := time.Now()
	s := NewSigner(testKey)
	c := sample(now)
	c.Host = ""
	if _, err := s.Verify(s.Sign(c), now); err == nil {
		t.Fatal("a link minted without a host verified")
	}
}
