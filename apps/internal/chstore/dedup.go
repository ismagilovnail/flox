package chstore

import (
	"context"

	"github.com/ClickHouse/clickhouse-go/v2"
)

type batchIDKey struct{}

// WithBatchID marks ctx's inserts as one retryable batch: InsertBatch and
// InsertPostbackAttempts then send each table's block with
// insert_deduplication_token = batchID:table, and ClickHouse skips a block
// whose token it already stored — in the dependent materialized views too
// (deduplicate_blocks_in_dependent_materialized_views). A retry of a batch
// that partly or fully landed therefore adds nothing (audit 2026-10-06, D2).
//
// The caller must send exactly the same rows under the same batchID
// (internal/pgqueue guarantees that). Every insert into these tables should
// carry one: with the deduplication window on (schema/012), an insert
// WITHOUT a token is deduplicated by a hash of its content instead, so a
// second block that is byte-identical to an earlier one is silently dropped
// from the table — while the views, which do not dedup untokened blocks,
// still count it.
func WithBatchID(ctx context.Context, batchID string) context.Context {
	return context.WithValue(ctx, batchIDKey{}, batchID)
}

// dedupContext adds the table's deduplication token when ctx carries a
// batch id.
func dedupContext(ctx context.Context, table string) context.Context {
	batchID, _ := ctx.Value(batchIDKey{}).(string)
	if batchID == "" {
		return ctx
	}
	return clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
		"insert_deduplication_token":                         batchID + ":" + table,
		"deduplicate_blocks_in_dependent_materialized_views": 1,
	}))
}
