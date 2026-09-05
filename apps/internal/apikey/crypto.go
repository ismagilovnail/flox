package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"strings"
)

// newKey mirrors the frontend mock's own exact format
// (stores/settings.ts's createApiKey, kept identical so this real
// backend produces keys indistinguishable in shape from what the UI
// already expected): "flx_live_" + a random secret, with prefix being
// that same string truncated to the first 4 secret characters plus a
// literal "..." — the panel renders `key.prefix` verbatim with no
// ellipsis of its own, so the truncation marker has to be baked in
// here.
func newKey() (fullKey, prefix, hash string, err error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("generating api key: %w", err)
	}
	secret := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf))
	fullKey = "flx_live_" + secret
	prefix = "flx_live_" + secret[:4] + "..."
	return fullKey, prefix, hashKey(fullKey), nil
}

func hashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
