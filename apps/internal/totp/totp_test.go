package totp_test

import (
	"strings"
	"testing"
	"time"

	stdtotp "github.com/pquerna/otp/totp"

	"github.com/ismagilovnail/flox/apps/internal/totp"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key, err := totp.ParseKey("IXkldrBTvgfDhIxzUBn3MrRAJq7Gh7z8mQ5UhBC4DRo=")
	if err != nil {
		t.Fatalf("ParseKey: %v", err)
	}
	return key
}

func TestParseKeyRejectsInvalidInput(t *testing.T) {
	if _, err := totp.ParseKey(""); err == nil {
		t.Error("empty key accepted")
	}
	if _, err := totp.ParseKey("not-base64!!!"); err == nil {
		t.Error("invalid base64 accepted")
	}
	if _, err := totp.ParseKey("YWJj"); err == nil { // "abc", decodes to 3 bytes
		t.Error("wrong-length key accepted")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := testKey(t)
	plaintext := "JBSWY3DPEHPK3PXP"

	ciphertext, err := totp.EncryptSecret(key, plaintext)
	if err != nil {
		t.Fatalf("EncryptSecret: %v", err)
	}
	if ciphertext == plaintext || strings.Contains(ciphertext, plaintext) {
		t.Fatal("ciphertext leaks the plaintext secret")
	}

	got, err := totp.DecryptSecret(key, ciphertext)
	if err != nil {
		t.Fatalf("DecryptSecret: %v", err)
	}
	if got != plaintext {
		t.Fatalf("DecryptSecret = %q, want %q", got, plaintext)
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	key := testKey(t)
	ciphertext, err := totp.EncryptSecret(key, "JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatalf("EncryptSecret: %v", err)
	}
	tampered := ciphertext[:len(ciphertext)-4] + "AAAA"
	if _, err := totp.DecryptSecret(key, tampered); err == nil {
		t.Fatal("tampered ciphertext decrypted successfully, want rejected")
	}
}

func TestGenerateEnrollmentAndValidateCode(t *testing.T) {
	enrollment, err := totp.GenerateEnrollment("FLOX", "owner@example.com")
	if err != nil {
		t.Fatalf("GenerateEnrollment: %v", err)
	}
	if enrollment.Secret == "" || enrollment.URL == "" || len(enrollment.QRCodePNG) == 0 {
		t.Fatalf("Enrollment = %+v, want all fields populated", enrollment)
	}
	if len(enrollment.BackupCodes) != 10 {
		t.Fatalf("len(BackupCodes) = %d, want 10", len(enrollment.BackupCodes))
	}
	seen := map[string]bool{}
	for _, c := range enrollment.BackupCodes {
		if seen[c] {
			t.Fatalf("duplicate backup code: %q", c)
		}
		seen[c] = true
	}

	realCode, err := stdtotp.GenerateCode(enrollment.Secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if !totp.ValidateCode(enrollment.Secret, realCode) {
		t.Fatal("a real, freshly generated code was rejected")
	}
	if totp.ValidateCode(enrollment.Secret, "000000") {
		t.Fatal("an arbitrary wrong code was accepted (extremely unlikely unless broken)")
	}
}

func TestHashBackupCodeIsDeterministicAndOneWay(t *testing.T) {
	h1 := totp.HashBackupCode("abcde-fghjk")
	h2 := totp.HashBackupCode("abcde-fghjk")
	if h1 != h2 {
		t.Fatal("HashBackupCode is not deterministic")
	}
	if h1 == "abcde-fghjk" {
		t.Fatal("HashBackupCode returned the plaintext")
	}
}
