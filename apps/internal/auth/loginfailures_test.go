package auth_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	stdtotp "github.com/pquerna/otp/totp"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/auth"
)

const testPassword = "correct-horse-battery-staple"

// forgetLoginFailures removes email's login_failures row after the test.
func forgetLoginFailures(t *testing.T, pool *pgxpool.Pool, email string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM login_failures WHERE email_hash = encode(sha256(convert_to(lower($1), 'UTF8')), 'hex')`, email)
	})
}

func isStatus(err error, status int) bool {
	var apiErr *apierror.Error
	return errors.As(err, &apiErr) && apiErr.Status == status
}

func failLogins(t *testing.T, svc *auth.Service, email string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		_, err := svc.Login(context.Background(), auth.LoginInput{Email: email, Password: "wrong-password"})
		if !isStatus(err, http.StatusUnauthorized) {
			t.Fatalf("wrong password #%d: err = %v, want 401", i+1, err)
		}
	}
}

func TestLoginLocksAfterTenFailuresEvenForTheRightPassword(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	email := uniqueEmail(t, pool)
	forgetLoginFailures(t, pool, email)
	signupOrg(t, ctx, pool, svc, "Lockout Org", email)

	failLogins(t, svc, email, 9)
	// Nine failures leave one try: the right password still works and
	// clears the count.
	if _, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: testPassword}); err != nil {
		t.Fatalf("login after 9 failures: %v", err)
	}

	failLogins(t, svc, email, 10)
	_, err := svc.Login(ctx, auth.LoginInput{Email: upper(email), Password: testPassword})
	if !isStatus(err, http.StatusTooManyRequests) {
		t.Fatalf("right password after 10 failures (email in another case): err = %v, want 429", err)
	}

	// The window over, the account signs in again.
	if _, err := pool.Exec(ctx, `UPDATE login_failures SET window_start = now() - interval '16 minutes'
		WHERE email_hash = encode(sha256(convert_to(lower($1), 'UTF8')), 'hex')`, email); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: testPassword}); err != nil {
		t.Fatalf("login after the window: %v", err)
	}
}

func TestLoginLocksAnEmailWithNoAccountTheSameWay(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	email := uniqueEmail(t, pool) // never signed up
	forgetLoginFailures(t, pool, email)

	failLogins(t, svc, email, 10)
	_, err := svc.Login(context.Background(), auth.LoginInput{Email: email, Password: testPassword})
	if !isStatus(err, http.StatusTooManyRequests) {
		t.Fatalf("11th attempt for an email with no account: err = %v, want 429 like a real one", err)
	}
}

func TestWrongTOTPCodesSpendTheSameBudget(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	email := uniqueEmail(t, pool)
	forgetLoginFailures(t, pool, email)
	owner := signupOrg(t, ctx, pool, svc, "Lockout TOTP Org", email)

	enrollment, err := svc.EnrollTOTP(ctx, owner.User.ID, email)
	if err != nil {
		t.Fatalf("EnrollTOTP: %v", err)
	}
	code, err := stdtotp.GenerateCode(enrollment.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ConfirmTOTP(ctx, owner.User.ID, code); err != nil {
		t.Fatalf("ConfirmTOTP: %v", err)
	}

	// The right password each time, a wrong code each time: the password
	// must not reset the count, or code guesses would be unlimited.
	for i := 0; i < 10; i++ {
		outcome, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: testPassword})
		if err != nil {
			t.Fatalf("login #%d: %v", i+1, err)
		}
		if _, err := svc.VerifyPendingLogin(ctx, outcome.PendingToken, "000000"); err == nil {
			t.Fatalf("wrong code #%d accepted", i+1)
		}
	}
	if _, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: testPassword}); !isStatus(err, http.StatusTooManyRequests) {
		t.Fatalf("login after 10 wrong codes: err = %v, want 429", err)
	}
}

func TestResolveSessionWritesLastActiveAtMostOncePerMinute(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	owner := signupOrg(t, ctx, pool, svc, "Last Active Org", uniqueEmail(t, pool))

	setLastActive := func(ago time.Duration) {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE memberships SET last_active_at = now() - make_interval(secs => $3)
			WHERE organization_id = $1 AND user_id = $2`, owner.Organization.ID, owner.User.ID, ago.Seconds()); err != nil {
			t.Fatal(err)
		}
	}
	age := func() time.Duration {
		t.Helper()
		var secs float64
		if err := pool.QueryRow(ctx, `SELECT extract(epoch FROM now() - last_active_at) FROM memberships
			WHERE organization_id = $1 AND user_id = $2`, owner.Organization.ID, owner.User.ID).Scan(&secs); err != nil {
			t.Fatal(err)
		}
		return time.Duration(secs * float64(time.Second))
	}
	resolve := func() {
		t.Helper()
		orgID, userID, err := svc.ResolveSession(ctx, owner.Token)
		if err != nil || orgID != owner.Organization.ID || userID != owner.User.ID {
			t.Fatalf("ResolveSession = (%q, %q, %v), want the owner's session", orgID, userID, err)
		}
	}

	setLastActive(30 * time.Second)
	resolve()
	if a := age(); a < 25*time.Second {
		t.Fatalf("last_active_at rewritten 30s after the last write (age now %s)", a)
	}

	setLastActive(2 * time.Minute)
	resolve()
	if a := age(); a > 5*time.Second {
		t.Fatalf("last_active_at not rewritten 2 min after the last write (age now %s)", a)
	}

	// Suspended: the session stops resolving even when no write is due.
	setLastActive(0)
	if _, err := pool.Exec(ctx, `UPDATE memberships SET status = 'suspended'
		WHERE organization_id = $1 AND user_id = $2`, owner.Organization.ID, owner.User.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ResolveSession(ctx, owner.Token); err == nil {
		t.Fatal("a suspended membership's session still resolves")
	}
}
