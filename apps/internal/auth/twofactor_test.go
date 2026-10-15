package auth_test

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/auth"
)

const password = "correct-horse-battery-staple"

var sixDigits = regexp.MustCompile(`\b(\d{6})\b`)

// fakeSMS captures what would go to the SMS gateway.
type fakeSMS struct{ to, text string }

func (f *fakeSMS) SendSMS(_ context.Context, to, text string) error {
	f.to, f.text = to, text
	return nil
}

func codeIn(t *testing.T, text string) string {
	t.Helper()
	m := sixDigits.FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("no code in %q", text)
	}
	return m[1]
}

func apiStatus(err error) int {
	var apiErr *apierror.Error
	if errors.As(err, &apiErr) {
		return apiErr.Status
	}
	return 0
}

// pastCooldown ages the user's codes, as if the resend cooldown had passed.
func pastCooldown(t *testing.T, pool *pgxpool.Pool, userID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE two_factor_codes SET created_at = created_at - interval '2 minutes' WHERE user_id = $1`, userID); err != nil {
		t.Fatal(err)
	}
}

// Email as the second factor: set up by a code to the account's email,
// then every sign-in sends one; a wrong code counts, three spend the
// sign-in; a backup code works too; a resend replaces the code.
func TestEmailTwoFactor(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	email := uniqueEmail(t, pool)
	owner := signupOrg(t, ctx, pool, svc, "Email 2FA Org", email)
	userID := owner.User.ID

	enrollment, err := svc.EnrollCode(ctx, userID, auth.EnrollCodeInput{Method: auth.MethodEmail})
	if err != nil {
		t.Fatalf("EnrollCode: %v", err)
	}
	if enrollment.Method != auth.MethodEmail || enrollment.SentTo != auth.MaskEmail(email) || len(enrollment.BackupCodes) != 10 ||
		sharedTestMailer.lastTo != email {
		t.Fatalf("enrollment = %+v, mail to %q", enrollment, sharedTestMailer.lastTo)
	}
	if _, err := svc.ConfirmCode(ctx, userID, "000000"); err == nil {
		t.Fatal("a wrong confirmation code was accepted")
	}
	if method, err := svc.ConfirmCode(ctx, userID, codeIn(t, sharedTestMailer.lastBody)); err != nil || method != auth.MethodEmail {
		t.Fatalf("ConfirmCode = %q, %v", method, err)
	}
	if st, err := svc.TwoFactorStatus(ctx, userID); err != nil || !st.Enabled || st.Method != auth.MethodEmail {
		t.Fatalf("status = %+v, %v", st, err)
	}

	login := func() auth.LoginOutcome {
		t.Helper()
		out, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: password})
		if err != nil {
			t.Fatalf("Login: %v", err)
		}
		if out.Session != nil || !out.RequiresTOTP || out.TwoFactorMethod != auth.MethodEmail || out.CodeSentTo != auth.MaskEmail(email) {
			t.Fatalf("login outcome = %+v", out)
		}
		return out
	}

	// Two wrong codes, then the right one.
	out := login()
	code := codeIn(t, sharedTestMailer.lastBody)
	for range 2 {
		if _, err := svc.VerifyPendingLogin(ctx, out.PendingToken, "111111"); apiStatus(err) != http.StatusUnauthorized {
			t.Fatalf("wrong code = %v, want 401", err)
		}
	}
	if res, err := svc.VerifyPendingLogin(ctx, out.PendingToken, code); err != nil || res.User.ID != userID {
		t.Fatalf("right code = %+v, %v", res, err)
	}

	// A resend within the cooldown is refused and the sent code still
	// works; after the cooldown the new code works and the old one not.
	pastCooldown(t, pool, userID)
	out = login()
	if _, err := svc.ResendLoginCode(ctx, out.PendingToken); apiStatus(err) != http.StatusTooManyRequests {
		t.Fatalf("resend within the cooldown = %v, want 429", err)
	}
	if _, err := svc.VerifyPendingLogin(ctx, out.PendingToken, codeIn(t, sharedTestMailer.lastBody)); err != nil {
		t.Fatalf("the sent code after a refused resend: %v", err)
	}
	pastCooldown(t, pool, userID)
	out = login()
	old := codeIn(t, sharedTestMailer.lastBody)
	pastCooldown(t, pool, userID)
	if _, err := svc.ResendLoginCode(ctx, out.PendingToken); err != nil {
		t.Fatalf("resend: %v", err)
	}
	fresh := codeIn(t, sharedTestMailer.lastBody)
	if fresh != old {
		if _, err := svc.VerifyPendingLogin(ctx, out.PendingToken, old); err == nil {
			t.Fatal("the replaced code still worked")
		}
	}
	if _, err := svc.VerifyPendingLogin(ctx, out.PendingToken, fresh); err != nil {
		t.Fatalf("fresh code: %v", err)
	}

	// Three wrong codes spend the sign-in.
	pastCooldown(t, pool, userID)
	out = login()
	code = codeIn(t, sharedTestMailer.lastBody)
	for range 3 {
		_, _ = svc.VerifyPendingLogin(ctx, out.PendingToken, "222222")
	}
	if _, err := svc.VerifyPendingLogin(ctx, out.PendingToken, code); err == nil {
		t.Fatal("the right code worked after three wrong ones")
	}

	// A backup code works in place of the sent code, once.
	pastCooldown(t, pool, userID)
	out = login()
	if _, err := svc.VerifyPendingLogin(ctx, out.PendingToken, enrollment.BackupCodes[0]); err != nil {
		t.Fatalf("backup code: %v", err)
	}

	// An expired code is refused.
	pastCooldown(t, pool, userID)
	out = login()
	code = codeIn(t, sharedTestMailer.lastBody)
	if _, err := pool.Exec(ctx, `UPDATE two_factor_codes SET expires_at = now() - interval '1 second' WHERE user_id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VerifyPendingLogin(ctx, out.PendingToken, code); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired code = %v", err)
	}
}

// At most 5 codes an hour per user, whatever the cooldown.
func TestTwoFactorHourlyCap(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	owner := signupOrg(t, ctx, pool, svc, "Cap 2FA Org", uniqueEmail(t, pool))
	for i := range 5 {
		if _, err := svc.EnrollCode(ctx, owner.User.ID, auth.EnrollCodeInput{Method: auth.MethodEmail}); err != nil {
			t.Fatalf("code %d: %v", i+1, err)
		}
		pastCooldown(t, pool, owner.User.ID)
	}
	if _, err := svc.EnrollCode(ctx, owner.User.ID, auth.EnrollCodeInput{Method: auth.MethodEmail}); apiStatus(err) != http.StatusTooManyRequests {
		t.Fatalf("6th code in an hour = %v, want 429", err)
	}
}

// SMS: not offered without a gateway; with one, the number is checked and
// normalised, the code goes to it, and sign-in sends the next one there.
// Setting SMS up replaces the app.
func TestSMSTwoFactor(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	noSMS := newService(pool)
	email := uniqueEmail(t, pool)
	owner := signupOrg(t, ctx, pool, noSMS, "SMS 2FA Org", email)
	userID := owner.User.ID
	if _, err := noSMS.EnrollCode(ctx, userID, auth.EnrollCodeInput{Method: auth.MethodSMS, Phone: "+79001234567"}); apiStatus(err) != http.StatusServiceUnavailable {
		t.Fatalf("SMS without a gateway = %v, want 503", err)
	}

	svc := newService(pool)
	sms := &fakeSMS{}
	svc.SetSMSSender(sms)
	// The app first: setting up SMS must replace it.
	if _, err := svc.EnrollTOTP(ctx, userID, email); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET totp_enabled_at = now(), two_factor_method = 'totp' WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.EnrollCode(ctx, userID, auth.EnrollCodeInput{Method: auth.MethodSMS, Phone: "8900"}); apiStatus(err) != http.StatusUnprocessableEntity {
		t.Fatalf("bad phone = %v, want 422", err)
	}
	if _, err := svc.EnrollCode(ctx, userID, auth.EnrollCodeInput{Method: auth.MethodSMS, Phone: "+7 (900) 123-45-67"}); err != nil {
		t.Fatalf("EnrollCode sms: %v", err)
	}
	if sms.to != "+79001234567" {
		t.Fatalf("SMS to %q, want the normalised number", sms.to)
	}
	// Still the app until the code is confirmed.
	if st, _ := svc.TwoFactorStatus(ctx, userID); st.Method != auth.MethodTOTP {
		t.Fatalf("method before confirming = %q, want totp", st.Method)
	}
	if method, err := svc.ConfirmCode(ctx, userID, codeIn(t, sms.text)); err != nil || method != auth.MethodSMS {
		t.Fatalf("ConfirmCode = %q, %v", method, err)
	}
	st, _ := svc.TwoFactorStatus(ctx, userID)
	var secret *string
	_ = pool.QueryRow(ctx, `SELECT totp_secret FROM users WHERE id = $1`, userID).Scan(&secret)
	if st.Method != auth.MethodSMS || st.Phone != "+7******4567" || !st.SMSAvailable || secret != nil {
		t.Fatalf("status = %+v, totp secret %v; want sms, masked phone, the app's secret gone", st, secret)
	}

	// Signing in where the gateway is gone: refused, not a code to nowhere.
	if _, err := noSMS.Login(ctx, auth.LoginInput{Email: email, Password: password}); apiStatus(err) != http.StatusServiceUnavailable {
		t.Fatalf("SMS sign-in without a gateway = %v, want 503", err)
	}

	sms.to, sms.text = "", ""
	out, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: password})
	if err != nil || out.TwoFactorMethod != auth.MethodSMS || out.CodeSentTo != "+7******4567" || sms.to != "+79001234567" {
		t.Fatalf("login = %+v, %v; SMS to %q", out, err, sms.to)
	}
	if _, err := svc.VerifyPendingLogin(ctx, out.PendingToken, codeIn(t, sms.text)); err != nil {
		t.Fatalf("SMS code: %v", err)
	}

	// Disabling (with the password) clears the method and the phone.
	if err := svc.DisableTOTP(ctx, userID, owner.Organization.ID, password); err != nil {
		t.Fatal(err)
	}
	if st, _ := svc.TwoFactorStatus(ctx, userID); st.Enabled || st.Phone != "" {
		t.Fatalf("after disabling = %+v", st)
	}
}

// When the organization requires 2FA, a member without it can set up
// email at sign-in, and confirming signs them in.
func TestForcedEnrollmentWithEmail(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	email := uniqueEmail(t, pool)
	owner := signupOrg(t, ctx, pool, svc, "Forced Email Org", email)
	required := true
	if _, err := svc.UpdateSecuritySettings(ctx, owner.Organization.ID, owner.User.ID, "203.0.113.5", auth.UpdateSecuritySettingsInput{TwoFactorRequired: &required}); err != nil {
		t.Fatal(err)
	}
	out, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: password})
	if err != nil || !out.RequiresTOTPEnrollment {
		t.Fatalf("login = %+v, %v; want enrollment required", out, err)
	}
	if _, err := svc.EnrollCode(ctx, owner.User.ID, auth.EnrollCodeInput{Method: auth.MethodEmail}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.CompleteCodeEnrollmentLogin(ctx, out.PendingToken, codeIn(t, sharedTestMailer.lastBody))
	if err != nil || res.User.ID != owner.User.ID || res.Token == "" {
		t.Fatalf("CompleteCodeEnrollmentLogin = %+v, %v", res, err)
	}
	if st, _ := svc.TwoFactorStatus(ctx, owner.User.ID); st.Method != auth.MethodEmail {
		t.Fatalf("method = %q, want email", st.Method)
	}
}

func TestMasks(t *testing.T) {
	if got := auth.MaskEmail("owner@flox.local"); got != "o***@flox.local" {
		t.Fatalf("MaskEmail = %q", got)
	}
	if got := auth.MaskPhone("+79001234567"); got != "+7******4567" {
		t.Fatalf("MaskPhone = %q", got)
	}
	if got := auth.NormalizePhone(" +7 (900) 123-45.67 "); got != "+79001234567" {
		t.Fatalf("NormalizePhone = %q", got)
	}
}
