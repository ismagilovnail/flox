package adaccount

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// ParseKey decodes a base64-encoded 32-byte AES-256 key (config.
// AdAccountEncryptionKey) — called once at startup so a misconfigured
// key fails loudly before any request ever tries to encrypt/decrypt a
// Google Ads credential with it. Deliberately a separate key from
// TOTP_ENCRYPTION_KEY and INTEGRATION_ENCRYPTION_KEY even though the
// AES-256-GCM mechanics are identical everywhere they're used — a real
// secret rotation or compromise of one should never force rotating the
// others. Facebook/TikTok Ads' own access_token predates this
// encryption infrastructure and is left in plain text (migration
// 00018's own doc comment) — this key only ever encrypts the new
// Google Ads fields this package's own crypto.go introduces.
func ParseKey(b64 string) ([]byte, error) {
	if b64 == "" {
		return nil, errors.New("adaccount: ADACCOUNT_ENCRYPTION_KEY is not set")
	}
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("adaccount: ADACCOUNT_ENCRYPTION_KEY is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("adaccount: ADACCOUNT_ENCRYPTION_KEY must decode to 32 bytes, got %d", len(key))
	}
	return key, nil
}

// encrypt/decrypt are the same AES-256-GCM shape as totp.EncryptSecret/
// DecryptSecret and integrations' own crypto.go — Google Ads' developer
// token/client secret/refresh token must be recoverable in full to make
// a real sync call with them, unlike a password or API key, which only
// ever need one-way comparison.
func encrypt(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("adaccount: creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("adaccount: creating gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("adaccount: generating nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decrypt(key []byte, encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("adaccount: decoding ciphertext: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("adaccount: creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("adaccount: creating gcm: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("adaccount: ciphertext too short")
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("adaccount: decrypting: %w", err)
	}
	return string(plaintext), nil
}

// encryptOptional/decryptOptional handle the "not every connection uses
// this field" case (facebook_ads/tiktok_ads connections have no
// developer/client/refresh secrets at all) — nil in, nil out, never an
// empty-string ciphertext.
func encryptOptional(key []byte, plaintext string) (*string, error) {
	if plaintext == "" {
		return nil, nil
	}
	enc, err := encrypt(key, plaintext)
	if err != nil {
		return nil, err
	}
	return &enc, nil
}

func decryptOptional(key []byte, encoded *string) (string, error) {
	if encoded == nil {
		return "", nil
	}
	return decrypt(key, *encoded)
}
