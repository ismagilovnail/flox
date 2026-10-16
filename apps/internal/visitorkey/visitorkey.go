// Package visitorkey turns a visitor's IP address and user agent into a
// keyed pseudonym (audit 2026-10-15, GDPR-1).
//
// Unique clicks count distinct visitors per campaign, and a visitor is
// told apart by IP + user agent. Raw IP and user agent are personal data
// and are wiped from ClickHouse after RETENTION_IP_DAYS
// (chstore.SetPersonalDataTTL); the pseudonym stays, so unique clicks of
// an old period read the same before and after the wipe. Keyed
// (HMAC-SHA256 with VISITOR_KEY): without the key, the IPv4 space times
// the common user agents is small enough to reverse an unkeyed hash.
//
// The key must be the same on every tracker replica and never change: a
// new key makes every returning visitor a new one.
package visitorkey

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

// ParseKey decodes VISITOR_KEY: base64, at least 32 bytes.
func ParseKey(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, errors.New("visitorkey: VISITOR_KEY is not set")
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("visitorkey: VISITOR_KEY is not valid base64: %w", err)
	}
	if len(key) < 32 {
		return nil, fmt.Errorf("visitorkey: VISITOR_KEY must decode to at least 32 bytes, got %d", len(key))
	}
	return key, nil
}

// Of is the visitor's pseudonym: 32 hex characters, "" when there is
// neither an IP nor a user agent (nothing to tell the visitor by — the
// report counts such a click by its click_id), or without a key.
func Of(key []byte, ip, userAgent string) string {
	if len(key) == 0 || (ip == "" && userAgent == "") {
		return ""
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(ip))
	mac.Write([]byte{0})
	mac.Write([]byte(userAgent))
	return hex.EncodeToString(mac.Sum(nil)[:16])
}
