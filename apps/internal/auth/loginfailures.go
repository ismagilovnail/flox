package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

// Failed sign-ins per account (tech audit 2026-10-06, API2). The per-IP
// limiter on /auth/* lives in Redis; this one lives in Postgres
// (login_failures, migration 00063), so guessing one account's password
// stays bounded while Redis is down and from any number of IPs.
//
// The key is the email, not the user: an email with no account fills up
// and locks exactly like one with an account, so a lockout tells nothing
// about which emails exist. While locked, even the right password is
// refused — otherwise the lock would not stop a guess. The price is that
// anyone can keep an account's sign-in locked by failing on purpose;
// existing sessions are not touched.
const (
	loginFailureLimit  = 10
	LoginFailureWindow = 15 * time.Minute

	// lastActiveResolution: ResolveSession writes last_active_at at most
	// this often (tech audit 2026-10-06, DB4).
	lastActiveResolution = time.Minute
)

// emailKey is how login_failures keys an email: sha256 of the lowercased
// address, computed in SQL so the password and TOTP paths cannot drift.
const emailKey = `encode(sha256(convert_to(lower(%s), 'UTF8')), 'hex')`

func errLoginLocked() error {
	return apierror.TooManyRequests("too many failed sign-in attempts for this account, try again in 15 minutes")
}

// LoginLocked reports whether email has used up its failed sign-ins for
// the current window.
func (r *Repository) LoginLocked(ctx context.Context, email string) (bool, error) {
	var locked bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM login_failures
			WHERE email_hash = `+fmt.Sprintf(emailKey, "$1")+`
			  AND window_start > now() - make_interval(secs => $2)
			  AND failures >= $3)`,
		email, LoginFailureWindow.Seconds(), loginFailureLimit,
	).Scan(&locked)
	if err != nil {
		return false, fmt.Errorf("auth: checking login failures: %w", err)
	}
	return locked, nil
}

// recordLoginFailureSQL counts one failure for the email in $1, starting a
// new window when the last one is over.
var recordLoginFailureSQL = `
	INSERT INTO login_failures (email_hash, failures, window_start)
	SELECT ` + fmt.Sprintf(emailKey, "%s") + `, 1, now() %s
	ON CONFLICT (email_hash) DO UPDATE SET
		failures = CASE WHEN login_failures.window_start <= now() - make_interval(secs => $2)
		                THEN 1 ELSE login_failures.failures + 1 END,
		window_start = CASE WHEN login_failures.window_start <= now() - make_interval(secs => $2)
		                    THEN now() ELSE login_failures.window_start END`

// RecordLoginFailure counts a wrong password for email.
func (r *Repository) RecordLoginFailure(ctx context.Context, email string) error {
	if _, err := r.db.Exec(ctx, fmt.Sprintf(recordLoginFailureSQL, "$1::text", ""),
		email, LoginFailureWindow.Seconds()); err != nil {
		return fmt.Errorf("auth: recording login failure: %w", err)
	}
	return nil
}

// RecordLoginFailureForUser counts a wrong second-factor code against the
// user's email — the same budget as wrong passwords.
func (r *Repository) RecordLoginFailureForUser(ctx context.Context, userID string) error {
	if _, err := r.db.Exec(ctx, fmt.Sprintf(recordLoginFailureSQL, "email", "FROM users WHERE id = $1"),
		userID, LoginFailureWindow.Seconds()); err != nil {
		return fmt.Errorf("auth: recording login failure: %w", err)
	}
	return nil
}

// ClearLoginFailures forgets an email's failures after a sign-in succeeds.
func (r *Repository) ClearLoginFailures(ctx context.Context, email string) error {
	if _, err := r.db.Exec(ctx,
		`DELETE FROM login_failures WHERE email_hash = `+fmt.Sprintf(emailKey, "$1"), email); err != nil {
		return fmt.Errorf("auth: clearing login failures: %w", err)
	}
	return nil
}
