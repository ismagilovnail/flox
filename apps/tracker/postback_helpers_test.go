package main

import (
	"crypto/sha256"
	"encoding/hex"
)

// hashFor is a network's stored postback secret hash for secret
// (conversion.ValidSecret).
func hashFor(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
