// Package testpg holds Postgres helpers shared by integration tests.
package testpg

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// PrivateTable creates a copy of table (columns, defaults, CHECK
// constraints, indexes — not foreign keys) for one test and drops it after.
//
// The queue tables (event_queue, postback_attempt_queue,
// postback_deliveries) are polled by the dev-stack worker, and full-suite
// runs fill them from other packages' tests; a test that claims from the
// real table loses rows to both and flakes. Its own copy cannot be touched.
func PrivateTable(t testing.TB, pool *pgxpool.Pool, table string) string {
	t.Helper()
	name := table + "_t_" + strings.ToLower(idgen.New())
	if _, err := pool.Exec(context.Background(), `CREATE TABLE `+name+` (LIKE `+table+` INCLUDING ALL)`); err != nil {
		t.Fatalf("creating a private copy of %s: %v", table, err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DROP TABLE `+name) })
	return name
}

// DeleteOrganization removes an organization a test created (its rows
// cascade) together with the users who belonged to it and to no other
// organization (or acted in it and left) — tests that sign up or invite people through the real
// service otherwise leave those users behind in the dev database. Three
// statements in one transaction: the users are found first, then the
// organization's cascade removes everything that points at them (audit
// log, memberships), then they go.
func DeleteOrganization(ctx context.Context, pool *pgxpool.Pool, orgID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT u.id FROM users u
		WHERE (EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = u.id AND m.organization_id = $1)
		       -- a member the test itself removed still has entries in the audit log
		       OR EXISTS (SELECT 1 FROM audit_logs a WHERE a.actor_user_id = u.id AND a.organization_id = $1))
		  AND NOT EXISTS (SELECT 1 FROM memberships o WHERE o.user_id = u.id AND o.organization_id <> $1)`, orgID)
	if err != nil {
		return err
	}
	var users []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		users = append(users, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, orgID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = ANY($1)`, users); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
