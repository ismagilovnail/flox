// Package totp implements real per-user TOTP two-factor authentication —
// §30's "Security" settings sub-panel, undesigned anywhere in
// FLOX-master-prompt-v3.md (confirmed by grep before this package was
// written): the mock UI's org-wide "require 2FA" toggle is the only
// existing design, so enrollment/verification/backup-codes here are new
// product design built on the standard, Google-Authenticator-compatible
// TOTP algorithm (RFC 6238) via github.com/pquerna/otp, not a spec
// transcription.
package totp

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"io"

	stdtotp "github.com/pquerna/otp/totp"
)

// ParseKey decodes a base64-encoded 32-byte AES-256 key (config.
// TOTPEncryptionKey) — called once at startup so a misconfigured key
// fails loudly before any request ever tries to encrypt/decrypt a
// secret with it.
func ParseKey(b64 string) ([]byte, error) {
	if b64 == "" {
		return nil, errors.New("totp: TOTP_ENCRYPTION_KEY is not set")
	}
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("totp: TOTP_ENCRYPTION_KEY is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("totp: TOTP_ENCRYPTION_KEY must decode to 32 bytes, got %d", len(key))
	}
	return key, nil
}

// EncryptSecret/DecryptSecret are the one symmetrically-encrypted (not
// hashed) secret in this codebase — a TOTP secret must be recoverable to
// check a live 6-digit code against it, unlike a password or session
// token, which only ever need one-way comparison (see auth/crypto.go's
// own doc comments for that precedent).
func EncryptSecret(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("totp: creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("totp: creating gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("totp: generating nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func DecryptSecret(key []byte, encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("totp: decoding ciphertext: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("totp: creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("totp: creating gcm: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("totp: ciphertext too short")
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("totp: decrypting: %w", err)
	}
	return string(plaintext), nil
}

// Enrollment is what Service.EnrollTOTP hands back — shown to the user
// exactly once. QRCodePNG is rendered server-side (via the same library
// that generated the secret) and returned as raw PNG bytes for the
// handler to base64-encode into a data: URI — the secret never needs to
// leave this process to reach a third-party QR-rendering service.
type Enrollment struct {
	Secret      string // base32, for "or enter this code manually"
	URL         string // otpauth://... provisioning URI
	QRCodePNG   []byte
	BackupCodes []string // plaintext, shown once
}

// GenerateEnrollment creates a brand-new secret + QR code — does not
// touch storage; the caller (Service.EnrollTOTP) decides how to persist
// the encrypted secret and hashed backup codes.
func GenerateEnrollment(issuer, accountName string) (Enrollment, error) {
	key, err := stdtotp.Generate(stdtotp.GenerateOpts{Issuer: issuer, AccountName: accountName})
	if err != nil {
		return Enrollment{}, fmt.Errorf("totp: generating key: %w", err)
	}
	img, err := key.Image(256, 256)
	if err != nil {
		return Enrollment{}, fmt.Errorf("totp: rendering QR code: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return Enrollment{}, fmt.Errorf("totp: encoding QR code PNG: %w", err)
	}
	codes, err := GenerateBackupCodes()
	if err != nil {
		return Enrollment{}, err
	}
	return Enrollment{
		Secret:      key.Secret(),
		URL:         key.URL(),
		QRCodePNG:   buf.Bytes(),
		BackupCodes: codes,
	}, nil
}

// ValidateCode checks a live 6-digit code against secret using the
// standard 30-second window (RFC 6238 defaults, ±1 step skew — the
// library's own default tolerance for clock drift).
func ValidateCode(secret, code string) bool {
	return stdtotp.Validate(code, secret)
}

const (
	backupCodeCount    = 10
	backupCodeLength   = 10
	backupCodeAlphabet = "23456789abcdefghjkmnpqrstuvwxyz" // skips 0/1/i/l/o, same convention as referral codes
)

// GenerateBackupCodes returns 10 one-time recovery codes, plaintext —
// the caller must hash each with HashBackupCode before persisting and
// never store the plaintext anywhere.
func GenerateBackupCodes() ([]string, error) {
	codes := make([]string, backupCodeCount)
	for i := range codes {
		raw := make([]byte, backupCodeLength)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("totp: generating backup code: %w", err)
		}
		chars := make([]byte, backupCodeLength)
		for j, v := range raw {
			chars[j] = backupCodeAlphabet[int(v)%len(backupCodeAlphabet)]
		}
		codes[i] = fmt.Sprintf("%s-%s", chars[:5], chars[5:])
	}
	return codes, nil
}

func HashBackupCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}
