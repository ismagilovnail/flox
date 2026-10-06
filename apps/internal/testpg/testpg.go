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
