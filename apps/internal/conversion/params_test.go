package conversion

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
	"testing"
	"time"
)

func hashFor(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func TestValidSecret(t *testing.T) {
	storedHash := hashFor("correct-secret")

	cases := []struct {
		name      string
		provided  string
		storedHas string
		want      bool
	}{
		{"correct secret", "correct-secret", storedHash, true},
		{"wrong secret", "wrong-secret", storedHash, false},
		{"empty provided", "", storedHash, false},
		{"empty stored hash (network predates this feature)", "correct-secret", "", false},
		{"both empty", "", "", false},
		{"provided secret happens to equal the stored HASH string itself", storedHash, storedHash, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidSecret(tc.provided, tc.storedHas); got != tc.want {
				t.Fatalf("ValidSecret(%q, %q) = %v, want %v", tc.provided, tc.storedHas, got, tc.want)
			}
		})
	}
}

// Audit UNI-4: a partial refund's amount and id come from the URL; a bad
// amount is a 400 like a bad revenue.
func TestParseParamsRefund(t *testing.T) {
	n := Network{ID: "net-1", OrganizationID: "org-1"}
	p, err := ParseParams(url.Values{"click_id": {"c"}, "status": {"refund"}, "refund_amount": {"250.5"}, "refund_id": {" R1 "}}, n, time.Now())
	if err != nil || p.RefundAmount == nil || *p.RefundAmount != 250.5 || p.RefundID != "R1" {
		t.Fatalf("ParseParams = %+v, %v", p, err)
	}
	if p, _ := ParseParams(url.Values{"click_id": {"c"}, "status": {"refund"}}, n, time.Now()); p.RefundAmount != nil {
		t.Fatal("no refund_amount: want nil")
	}
	for _, bad := range []string{"NaN", "-5", "abc"} {
		if _, err := ParseParams(url.Values{"click_id": {"c"}, "status": {"refund"}, "refund_amount": {bad}}, n, time.Now()); err == nil || !strings.Contains(err.Error(), "refund_amount") {
			t.Errorf("refund_amount=%s: err = %v, want one naming refund_amount", bad, err)
		}
	}
}
