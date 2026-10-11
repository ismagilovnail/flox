package pixel

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// ParseKey decodes PIXEL_ENCRYPTION_KEY: base64 of 32 bytes, AES-256-GCM.
// Its own key, like ADACCOUNT_ENCRYPTION_KEY and INTEGRATION_ENCRYPTION_KEY:
// rotating or leaking one never forces rotating the others. Called at
// startup by apps/api and apps/worker, so a bad key fails before any token
// is written or read with it.
func ParseKey(b64 string) ([]byte, error) {
	if b64 == "" {
		return nil, errors.New("pixel: PIXEL_ENCRYPTION_KEY is not set")
	}
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("pixel: PIXEL_ENCRYPTION_KEY is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("pixel: PIXEL_ENCRYPTION_KEY must decode to 32 bytes, got %d", len(key))
	}
	return key, nil
}

// The token has to be read back in full to call the platform, so it is
// encrypted, not hashed.
func encrypt(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("pixel: creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("pixel: creating gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("pixel: generating nonce: %w", err)
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plaintext), nil)), nil
}

func decrypt(key []byte, encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("pixel: decoding ciphertext: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("pixel: creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("pixel: creating gcm: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("pixel: ciphertext too short")
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("pixel: decrypting: %w", err)
	}
	return string(plain), nil
}
