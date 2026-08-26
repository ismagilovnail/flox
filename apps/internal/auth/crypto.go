package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost matches bcrypt's own package default (10) explicitly rather
// than leaving it implicit, so a future bcrypt default change upstream
// can't silently weaken (or slow down) every password hash this package
// writes.
const bcryptCost = bcrypt.DefaultCost

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(hash), nil
}

// checkPassword reports whether password matches hash. A blank hash (the
// sentinel migration 00020 gives a shell user before they've accepted
// their invite, see users.password_hash's own doc comment) never matches
// anything — bcrypt.CompareHashAndPassword errors out on a malformed hash
// rather than matching, so this needs no separate blank-hash check to be
// safe, but the early return avoids paying bcrypt's deliberately-slow
// comparison cost for a login attempt against an account that can't
// possibly succeed.
func checkPassword(hash, password string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// newBearerToken returns a fresh cryptographically random token (used as
// both a session cookie value and an invite link's token — same shape,
// same trust requirements) and its SHA-256 hash, hex-encoded. Only the
// hash is ever persisted (same one-way-hash-at-rest precedent as
// api_keys.key_hash, migration 00010) — the raw token exists only in the
// Set-Cookie header / the invite URL, never in a database row.
func newBearerToken() (token, tokenHash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generating token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
