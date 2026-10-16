package visitorkey_test

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/visitorkey"
)

func TestOf(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	a := visitorkey.Of(key, "203.0.113.5", "Mozilla/5.0")
	if len(a) != 32 || a != visitorkey.Of(key, "203.0.113.5", "Mozilla/5.0") {
		t.Fatalf("Of = %q, want 32 hex characters, the same every time", a)
	}
	for name, other := range map[string]string{
		"another IP":         visitorkey.Of(key, "203.0.113.6", "Mozilla/5.0"),
		"another user agent": visitorkey.Of(key, "203.0.113.5", "Mozilla/5.1"),
		// The separator keeps "1.2.3.4"+"5x" apart from "1.2.3.45"+"x".
		"shifted boundary": visitorkey.Of(key, "203.0.113.", "5Mozilla/5.0"),
		"another key":      visitorkey.Of(bytes.Repeat([]byte{8}, 32), "203.0.113.5", "Mozilla/5.0"),
	} {
		if other == a {
			t.Errorf("%s gives the same pseudonym", name)
		}
	}
	if got := visitorkey.Of(key, "", ""); got != "" {
		t.Errorf("no IP and no user agent = %q, want empty", got)
	}
	if got := visitorkey.Of(nil, "203.0.113.5", "Mozilla/5.0"); got != "" {
		t.Errorf("no key = %q, want empty", got)
	}
}

func TestParseKey(t *testing.T) {
	good := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	if _, err := visitorkey.ParseKey(good); err != nil {
		t.Fatalf("ParseKey(32 bytes): %v", err)
	}
	for _, bad := range []string{"", "not base64!", base64.StdEncoding.EncodeToString([]byte("short"))} {
		if _, err := visitorkey.ParseKey(bad); err == nil {
			t.Errorf("ParseKey(%q) succeeded", bad)
		}
	}
}
