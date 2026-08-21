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
)

//go:embed schema/*.sql
var schemaFS embed.FS

// Migrate applies every schema/*.sql file in lexical order (001_, 002_, ...)
// idempotently — every statement is CREATE ... IF NOT EXISTS, so this is
// safe to call from every process that touches ClickHouse (worker and api
// both do, at startup) without a separate migration-runner tool or a
// version-tracking table. That's a deliberate simplification matched to a
// single disposable table, not a pattern to keep once Phase 26 lands a
// schema meant to last — see the package doc.
func Migrate(ctx context.Context, conn driver.Conn) error {
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
		raw, err := schemaFS.ReadFile("schema/" + name)
		if err != nil {
			return fmt.Errorf("chstore: reading %s: %w", name, err)
		}
		for _, stmt := range splitStatements(string(raw)) {
			if err := conn.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("chstore: applying %s: %w", name, err)
			}
		}
	}
	return nil
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
