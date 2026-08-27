package main

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func hashFor(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func TestValidPostbackSecret(t *testing.T) {
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
			if got := validPostbackSecret(tc.provided, tc.storedHas); got != tc.want {
				t.Fatalf("validPostbackSecret(%q, %q) = %v, want %v", tc.provided, tc.storedHas, got, tc.want)
			}
		})
	}
}
