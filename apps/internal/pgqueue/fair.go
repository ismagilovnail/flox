package pgqueue

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
)

// Fair claiming (tech audit M2): one organization flooding a queue must not
// put every other organization's rows behind its backlog. A claim first
// takes a share from each organization with rows waiting — up to
// Quota(limit, n) of each one's oldest — and fills what is left of the
// batch oldest-first from anyone, so spare capacity is never left idle.
//
// The organizations are found by a skip scan over an index led by
// organization_id (migration 00066): one probe per organization, not a
// scan of the backlog. One claim looks at up to limit of them; a Rotation
// starts each claim after the last organization the previous one reached,
// so with more organizations than that, every one gets its turn.
//
// Order within an organization stays oldest-first. Across organizations
// there is no order — ClickHouse's aggregates do not depend on it (tech
// audit Q3).

// Querier is the slice of pgx the skip scan needs.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Rotation remembers where the last claim's organizations ended. Safe for
// concurrent use; the zero value starts from the first organization.
type Rotation struct {
	mu    sync.Mutex
	after string
}

func (r *Rotation) start() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.after
}

func (r *Rotation) advance(after string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.after = after
	r.mu.Unlock()
}

// orgCeiling sorts after every ULID (Crockford base32 is 0-9A-Z).
const orgCeiling = "~"

// PendingOrgs returns up to max organizations with rows in table matching
// pending (a SQL predicate over the table's own columns, a constant —
// never input), starting after rot's position and wrapping around, and
// moves rot past them.
func PendingOrgs(ctx context.Context, q Querier, table, pending string, rot *Rotation, max int) ([]string, error) {
	start := rot.start()
	orgs, err := skipScan(ctx, q, table, pending, start, orgCeiling, max)
	if err != nil {
		return nil, err
	}
	if len(orgs) < max && start != "" {
		// Wrap around: the organizations up to where this claim started.
		more, err := skipScan(ctx, q, table, pending, "", start, max-len(orgs))
		if err != nil {
			return nil, err
		}
		orgs = append(orgs, more...)
	}
	if len(orgs) == max {
		rot.advance(orgs[len(orgs)-1])
	} else {
		rot.advance("") // everyone fit: the next claim starts from the top
	}
	return orgs, nil
}

// skipScan lists the distinct organizations in (after, upTo] with pending
// rows, in order, at most max: each step is one index probe for the next
// organization_id (a recursive CTE — Postgres has no native skip scan).
func skipScan(ctx context.Context, q Querier, table, pending, after, upTo string, max int) ([]string, error) {
	next := `(SELECT organization_id::text FROM ` + table + `
		WHERE ` + pending + ` AND organization_id > %s AND organization_id <= $2::text
		ORDER BY organization_id LIMIT 1)`
	rows, err := q.Query(ctx, `
		WITH RECURSIVE orgs(org, n) AS (
			SELECT `+fmt.Sprintf(next, "$1::text")+`, 1
			UNION ALL
			SELECT `+fmt.Sprintf(next, "orgs.org")+`, orgs.n + 1
			FROM orgs WHERE orgs.org IS NOT NULL AND orgs.n < $3
		)
		SELECT org FROM orgs WHERE org IS NOT NULL`,
		after, upTo, max)
	if err != nil {
		return nil, fmt.Errorf("pgqueue: finding organizations with pending rows in %s: %w", table, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var org string
		if err := rows.Scan(&org); err != nil {
			return nil, fmt.Errorf("pgqueue: finding organizations with pending rows in %s: %w", table, err)
		}
		out = append(out, org)
	}
	return out, rows.Err()
}

// Quota is each organization's share of a limit-row claim over n
// organizations: at least one row each.
func Quota(limit, n int) int {
	if n <= 0 {
		return limit
	}
	return max(1, (limit+n-1)/n)
}

// fairPending: fresh rows waiting in an event_queue-shaped table — what
// migration 00066's partial index covers. Fresh rows are due on arrival.
const fairPending = `status = 'queued' AND batch_id IS NULL`

// claimFair claims each pending organization's share of a fresh batch
// under batchID; claimFresh fills the rest.
func claimFair(ctx context.Context, tx pgx.Tx, table string, limit int, batchID string, rot *Rotation) ([]Row, error) {
	orgs, err := PendingOrgs(ctx, tx, table, fairPending, rot, limit)
	if err != nil || len(orgs) == 0 {
		return nil, err
	}
	rows, err := queryRows(ctx, tx, `
		WITH picked AS (
			SELECT c.id FROM unnest($1::text[]) AS o(org)
			CROSS JOIN LATERAL (
				SELECT id FROM `+table+`
				WHERE organization_id = o.org AND `+fairPending+` AND next_attempt_at <= now()
				ORDER BY next_attempt_at, id
				LIMIT $2
				FOR UPDATE SKIP LOCKED
			) c
			LIMIT $3
		)
		UPDATE `+table+` q
		SET status = 'processing', claimed_at = now(), batch_id = $4
		FROM picked
		WHERE q.id = picked.id
		RETURNING q.id, q.payload`,
		orgs, Quota(limit, len(orgs)), limit, batchID)
	if err != nil {
		return nil, fmt.Errorf("pgqueue: claiming fair shares on %s: %w", table, err)
	}
	return rows, nil
}
