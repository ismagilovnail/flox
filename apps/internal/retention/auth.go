package retention

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// ExpiredAuth removes sign-in state nothing can use any more (tech audit
// 2026-10-06, DB5). Every reader already ignores it — ResolveSession and
// the token lookups require expires_at > now() — so this only stops the
// tables from growing.
//
//   - sessions and pending_totp_logins past expires_at are deleted;
//   - an expired invite or email-verification token is cleared, not its
//     row: the membership is still the pending invite on the Team page
//     (resend issues a new token) and the user is still the account;
//   - login_failures whose window is over are deleted.
type ExpiredAuth struct {
	DB *pgxpool.Pool
	// LoginFailureWindow is internal/auth's: a row older than it counts
	// for nothing.
	LoginFailureWindow time.Duration
	Batch              int // 0: DefaultBatch
	Logger             *slog.Logger
}

// expiredAuthSteps: what goes, by table label. $1 is the batch size;
// login_failures also takes $2, the window in seconds.
var expiredAuthSteps = []struct {
	table, sql string
	window     bool
}{
	{"sessions", `
		DELETE FROM sessions WHERE id IN (
			SELECT id FROM sessions WHERE expires_at <= now() LIMIT $1)`, false},
	{"pending_totp_logins", `
		DELETE FROM pending_totp_logins WHERE id IN (
			SELECT id FROM pending_totp_logins WHERE expires_at <= now() LIMIT $1)`, false},
	{"invite_tokens", `
		UPDATE memberships SET invite_token_hash = NULL, invite_token_expires_at = NULL
		WHERE id IN (
			SELECT id FROM memberships
			WHERE invite_token_hash IS NOT NULL AND invite_token_expires_at <= now() LIMIT $1)`, false},
	{"email_verification_tokens", `
		UPDATE users SET email_verification_token_hash = NULL, email_verification_token_expires_at = NULL
		WHERE id IN (
			SELECT id FROM users
			WHERE email_verification_token_hash IS NOT NULL AND email_verification_token_expires_at <= now() LIMIT $1)`, false},
	{"login_failures", `
		DELETE FROM login_failures WHERE email_hash IN (
			SELECT email_hash FROM login_failures
			WHERE window_start <= now() - make_interval(secs => $2) LIMIT $1)`, true},
}

// RunOnce clears everything expired, batch by batch, and returns how many
// rows it touched per table label.
func (e *ExpiredAuth) RunOnce(ctx context.Context) (map[string]int64, error) {
	batch := e.Batch
	if batch <= 0 {
		batch = DefaultBatch
	}
	done := map[string]int64{}
	for _, step := range expiredAuthSteps {
		for {
			args := []any{batch}
			if step.window {
				args = append(args, e.LoginFailureWindow.Seconds())
			}
			tag, err := e.DB.Exec(ctx, step.sql, args...)
			if err != nil {
				return done, fmt.Errorf("retention: clearing expired %s: %w", step.table, err)
			}
			n := tag.RowsAffected()
			done[step.table] += n
			metrics.RetentionDeletedTotal.WithLabelValues(step.table).Add(float64(n))
			if n < int64(batch) {
				break
			}
		}
	}
	return done, nil
}

// RunLoop runs RunOnce every interval until ctx is done.
func (e *ExpiredAuth) RunLoop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		done, err := e.RunOnce(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			e.Logger.Warn("expired auth cleanup failed", "error", err)
		default:
			for table, n := range done {
				if n > 0 {
					e.Logger.Info("expired auth state cleared", "table", table, "rows", n)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
