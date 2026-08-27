package network

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// newPostbackSecret returns a fresh cryptographically random secret — the
// value embedded in the incoming postback URL FLOX hands a network
// operator (?secret=...) — and its SHA-256 hash, hex-encoded. Only the
// hash is ever persisted (networks.postback_secret_hash, migration
// 00021); the raw secret exists only in the HTTP response that creates
// or regenerates it, same one-way-hash-at-rest precedent as
// apps/internal/auth's own session/invite tokens.
func newPostbackSecret() (secret, secretHash string, err error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("network: generating postback secret: %w", err)
	}
	secret = base64.RawURLEncoding.EncodeToString(buf)
	return secret, hashSecret(secret), nil
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
