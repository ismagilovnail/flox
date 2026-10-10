package retention_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/retention"
)

func TestExpiredAuthClearsOnlyWhatExpired(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("needs DATABASE_URL")
	}
	ctx := context.Background()
	pg, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pg.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	count := func(sql string, args ...any) int {
		t.Helper()
		var n int
		if err := pg.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}

	orgID, userID, invitedID := idgen.New(), idgen.New(), idgen.New()
	exec(`INSERT INTO organizations (id, name) VALUES ($1, 'expired auth test')`, orgID)
	t.Cleanup(func() { _, _ = pg.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID) })
	for _, u := range []string{userID, invitedID} {
		exec(`INSERT INTO users (id, email, name, password_hash, email_verification_token_hash, email_verification_token_expires_at)
		      VALUES ($1, $2, 'u', 'x', $3, now() - interval '1 hour')`, u, "retention-"+u+"@example.com", "vt-"+u)
	}
	t.Cleanup(func() {
		_, _ = pg.Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1)`, []string{userID, invitedID})
	})
	// invitedID's verification token is still good.
	exec(`UPDATE users SET email_verification_token_expires_at = now() + interval '1 hour' WHERE id = $1`, invitedID)

	exec(`INSERT INTO sessions (id, user_id, organization_id, token_hash, expires_at) VALUES
	      ($1, $3, $4, $5, now() - interval '1 minute'), ($2, $3, $4, $6, now() + interval '1 day')`,
		idgen.New(), idgen.New(), userID, orgID, "h-"+idgen.New(), "h-"+idgen.New())
	exec(`INSERT INTO pending_totp_logins (id, user_id, organization_id, token_hash, expires_at) VALUES
	      ($1, $3, $4, $5, now() - interval '1 minute'), ($2, $3, $4, $6, now() + interval '5 minutes')`,
		idgen.New(), idgen.New(), userID, orgID, "h-"+idgen.New(), "h-"+idgen.New())
	exec(`INSERT INTO memberships (id, organization_id, user_id, role_id, status, invite_token_hash, invite_token_expires_at)
	      SELECT $1, $2, $3, id, 'invited', $4, now() - interval '1 day' FROM roles WHERE key = 'Viewer'`,
		idgen.New(), orgID, invitedID, "it-"+invitedID)
	oldKey, newKey := "lf-old-"+idgen.New(), "lf-new-"+idgen.New()
	exec(`INSERT INTO login_failures (email_hash, failures, window_start) VALUES
	      ($1, 3, now() - interval '20 minutes'), ($2, 3, now() - interval '5 minutes')`, oldKey, newKey)
	t.Cleanup(func() {
		_, _ = pg.Exec(context.Background(), `DELETE FROM login_failures WHERE email_hash = ANY($1)`, []string{oldKey, newKey})
	})

	job := &retention.ExpiredAuth{DB: pg, LoginFailureWindow: 15 * time.Minute, Batch: 1,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if _, err := job.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if n := count(`SELECT count(*) FROM sessions WHERE user_id = $1`, userID); n != 1 {
		t.Errorf("sessions left = %d, want 1 (the live one)", n)
	}
	if n := count(`SELECT count(*) FROM sessions WHERE user_id = $1 AND expires_at > now()`, userID); n != 1 {
		t.Errorf("live session deleted")
	}
	if n := count(`SELECT count(*) FROM pending_totp_logins WHERE user_id = $1 AND expires_at > now()`, userID); n != 1 {
		t.Errorf("live pending login deleted")
	}
	if n := count(`SELECT count(*) FROM pending_totp_logins WHERE user_id = $1`, userID); n != 1 {
		t.Errorf("pending logins left = %d, want 1", n)
	}
	if n := count(`SELECT count(*) FROM memberships WHERE user_id = $1 AND status = 'invited' AND invite_token_hash IS NULL`, invitedID); n != 1 {
		t.Errorf("expired invite: want the row kept with its token cleared")
	}
	if n := count(`SELECT count(*) FROM users WHERE id = $1 AND email_verification_token_hash IS NULL`, userID); n != 1 {
		t.Errorf("expired verification token not cleared")
	}
	if n := count(`SELECT count(*) FROM users WHERE id = $1 AND email_verification_token_hash IS NOT NULL`, invitedID); n != 1 {
		t.Errorf("live verification token cleared")
	}
	if n := count(`SELECT count(*) FROM login_failures WHERE email_hash = ANY($1)`, []string{oldKey, newKey}); n != 1 {
		t.Errorf("login_failures left = %d, want 1 (the current window)", n)
	}
	if n := count(`SELECT count(*) FROM login_failures WHERE email_hash = $1`, newKey); n != 1 {
		t.Errorf("current login_failures window deleted")
	}
}
