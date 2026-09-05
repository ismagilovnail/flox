package integrations

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
// IntegrationEncryptionKey) — called once at startup so a misconfigured
// key fails loudly before any request ever tries to encrypt/decrypt a
// credential with it. Deliberately a separate key from TOTP_ENCRYPTION_
// KEY (apps/internal/totp) even though the AES-256-GCM mechanics are
// identical — a real secret rotation or compromise of one should never
// force rotating the other.
func ParseKey(b64 string) ([]byte, error) {
	if b64 == "" {
		return nil, errors.New("integrations: INTEGRATION_ENCRYPTION_KEY is not set")
	}
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("integrations: INTEGRATION_ENCRYPTION_KEY is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("integrations: INTEGRATION_ENCRYPTION_KEY must decode to 32 bytes, got %d", len(key))
	}
	return key, nil
}

// encrypt/decrypt are the same AES-256-GCM shape as totp.EncryptSecret/
// DecryptSecret — a provider's credential fields must be recoverable in
// full to make a real API call with them, unlike a password or API key,
// which only ever need one-way comparison.
func encrypt(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("integrations: creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("integrations: creating gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("integrations: generating nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decrypt(key []byte, encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("integrations: decoding ciphertext: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("integrations: creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("integrations: creating gcm: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("integrations: ciphertext too short")
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("integrations: decrypting: %w", err)
	}
	return string(plaintext), nil
}
