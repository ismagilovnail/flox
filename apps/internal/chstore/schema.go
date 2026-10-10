// Package chstore is the ClickHouse-facing half of the analytics pipeline
// (§47/§48): applying the real five-table schema (schema/*.sql) —
// click_events, tracking_events, conversion_events, cost_events,
// postback_events — batch-writing into it, and reading back the
// materialized aggregates it maintains. Replaces Phase 25's single
// disposable `events` table (see schema/000_drop_phase25_schema.sql and
// docs/analytics-pipeline.md for that history).
package chstore

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema/*.sql
var schemaFS embed.FS

// migrationsTable records which schema/*.sql files are applied, one row per
// file (the name without .sql). ReplacingMergeTree: two processes racing a
// fresh database (tests, which use Migrate unlocked) may both record a
// file; reads take DISTINCT.
const migrationsTable = "schema_migrations"

// Migrate applies the schema/*.sql files not yet recorded in
// schema_migrations, in lexical order (001_, 002_, ...), recording each
// once all its statements succeeded (tech audit 2026-10-06, CH5). A file
// that fails part-way is not recorded and runs again from its first
// statement next time, so a file with several statements must still be
// safe to re-run; one that succeeded never runs again, so a new file may
// be a non-idempotent ALTER or a one-off data fix.
//
// A database from before the table existed has none recorded: every file
// runs once more — all of those are idempotent — and is recorded.
//
// Processes that start together use MigrateLocked; Migrate itself does
// not serialize (ClickHouse has no locks), which is fine for tests.
func Migrate(ctx context.Context, conn driver.Conn) error {
	if err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS `+migrationsTable+` (
			version    String,
			applied_at DateTime64(3, 'UTC') DEFAULT now64(3)
		) ENGINE = ReplacingMergeTree ORDER BY version`); err != nil {
		return fmt.Errorf("chstore: creating %s: %w", migrationsTable, err)
	}
	applied, err := appliedMigrations(ctx, conn)
	if err != nil {
		return err
	}

	entries, err := fs.ReadDir(schemaFS, "schema")
	if err != nil {
		return fmt.Errorf("chstore: reading embedded schema dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		version := strings.TrimSuffix(name, ".sql")
		if applied[version] {
			continue
		}
		raw, err := schemaFS.ReadFile("schema/" + name)
		if err != nil {
			return fmt.Errorf("chstore: reading %s: %w", name, err)
		}
		for _, stmt := range splitStatements(string(raw)) {
			if err := conn.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("chstore: applying %s: %w", name, err)
			}
		}
		if err := conn.Exec(ctx, `INSERT INTO `+migrationsTable+` (version) VALUES (?)`, version); err != nil {
			return fmt.Errorf("chstore: recording %s: %w", name, err)
		}
	}
	return nil
}

func appliedMigrations(ctx context.Context, conn driver.Conn) (map[string]bool, error) {
	rows, err := conn.Query(ctx, `SELECT DISTINCT version FROM `+migrationsTable)
	if err != nil {
		return nil, fmt.Errorf("chstore: reading %s: %w", migrationsTable, err)
	}
	defer rows.Close()
	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("chstore: reading %s: %w", migrationsTable, err)
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// migrateLockKey is the Postgres advisory lock MigrateLocked holds: an
// arbitrary constant, unique in this codebase.
const migrateLockKey int64 = 0x666c6f78_63686d67 // "floxchmg"

// MigrateLocked is Migrate under a Postgres advisory lock. api, tracker and
// worker each migrate ClickHouse at start; without the lock, starting
// together they ran the same files at once (CH5). The first one applies,
// the others wait and then find everything recorded. A process that dies
// holding the lock drops its session, and Postgres releases it.
func MigrateLocked(ctx context.Context, pg *pgxpool.Pool, conn driver.Conn) error {
	lockConn, err := pg.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("chstore: acquiring connection for migration lock: %w", err)
	}
	defer lockConn.Release()
	if _, err := lockConn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrateLockKey); err != nil {
		return fmt.Errorf("chstore: taking migration lock: %w", err)
	}
	defer func() {
		// Unlock even if ctx is done, or the lock outlives the migration
		// for as long as the pooled connection lives.
		_, _ = lockConn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, migrateLockKey)
	}()
	return Migrate(ctx, conn)
}

// splitStatements strips "--" line comments, then splits what's left on
// top-level ";" terminators. Stripping comments first (rather than just
// splitting raw text on ";") matters because a comment explaining the SQL
// is free to contain a literal ";" in English prose — splitting on that
// would hand ClickHouse a comment-only "statement" and get back "Empty
// query." Good enough for this package's own hand-written DDL files; not a
// general SQL parser (a ";" inside a string literal would still confuse
// it, but none of schema/*.sql has one).
func splitStatements(sql string) []string {
	var stripped strings.Builder
	for _, line := range strings.Split(sql, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		stripped.WriteString(line)
		stripped.WriteByte('\n')
	}

	var out []string
	for _, part := range strings.Split(stripped.String(), ";") {
		if s := strings.TrimSpace(part); s != "" {
			out = append(out, s)
		}
	}
	return out
}
