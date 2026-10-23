package retention

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
)

// Plans keeps how long ClickHouse holds each organization's raw clicks
// and tracking events in step with its plan (audit DATA-6;
// docs/retention.md "Raw clicks by plan").
//
// organization_retention (migration 00099) holds, per organization, the
// months its rows carry and its plan's target. Each Run:
//
//  1. records each organization's target, and when it changed;
//  2. moves months to the target — at once when the target is longer,
//     after ShortenAfter when it is shorter, so a lapsed payment or a short
//     downgrade deletes nothing — and from then on new rows carry it;
//  3. rewrites the rows of organizations whose months changed at least
//     settle ago, so every worker's cache already writes the new value
//     (chstore.SetRawRetention); the tables' TTL deletes what is older.
//
// Of is the per-insert lookup (chstore.EventStore.WithRetention), from a
// cache Load refreshes. Like the other retention jobs it runs across all
// organizations; Only scopes it to one (tests).
type Plans struct {
	DB           *pgxpool.Pool
	CH           driver.Conn
	ShortenAfter time.Duration
	Logger       *slog.Logger
	Only         string

	mu     sync.RWMutex
	months map[string]uint8
}

const (
	// PlansInterval: how often Run checks plans.
	PlansInterval = time.Hour
	// refreshEvery: how often a worker reloads the months new rows carry.
	refreshEvery = time.Minute
	// settle: how long after months change the rows are rewritten — past
	// every worker's next refresh, so no row is inserted with the old value
	// after the rewrite.
	settle = 5 * refreshEvery
)

// Of is how many months the organization's new raw rows are kept; 0 —
// not loaded yet, or a new organization — is the column default, 24.
func (p *Plans) Of(organizationID string) uint8 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.months[organizationID]
}

// Load reloads the cache Of reads.
func (p *Plans) Load(ctx context.Context) error {
	rows, err := p.DB.Query(ctx, `SELECT organization_id, months FROM organization_retention`)
	if err != nil {
		return fmt.Errorf("retention: loading raw retention: %w", err)
	}
	defer rows.Close()
	months := map[string]uint8{}
	for rows.Next() {
		var org string
		var m int
		if err := rows.Scan(&org, &m); err != nil {
			return fmt.Errorf("retention: loading raw retention: %w", err)
		}
		months[org] = uint8(m)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("retention: loading raw retention: %w", err)
	}
	p.mu.Lock()
	p.months = months
	p.mu.Unlock()
	return nil
}

// RefreshLoop reloads the cache every minute until ctx ends.
func (p *Plans) RefreshLoop(ctx context.Context) {
	t := time.NewTicker(refreshEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := p.Load(ctx); err != nil && ctx.Err() == nil {
				p.Logger.Warn("raw retention cache not refreshed", "error", err)
			}
		}
	}
}

// RunLoop runs Run hourly until ctx ends.
func (p *Plans) RunLoop(ctx context.Context) {
	t := time.NewTicker(PlansInterval)
	defer t.Stop()
	for {
		if err := p.Run(ctx); err != nil && ctx.Err() == nil {
			p.Logger.Error("raw retention not applied", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Run is one pass of the three steps in the type's comment.
func (p *Plans) Run(ctx context.Context) error {
	only := ""
	args := []any{}
	if p.Only != "" {
		only, args = " AND o.id = $1", append(args, p.Only)
	}
	// 1. Targets. A new organization starts at 24 months, what its rows
	// already mean (0 = 24), applied.
	if _, err := p.DB.Exec(ctx, `
		INSERT INTO organization_retention (organization_id, months, target, target_since, applied_at)
		SELECT o.id, 24, coalesce(p.raw_retention_months, f.raw_retention_months), now(), now()
		FROM organizations o
		LEFT JOIN organization_plans op ON op.organization_id = o.id AND op.ended_at IS NULL
		LEFT JOIN plans p ON p.code = op.plan_code
		CROSS JOIN (SELECT raw_retention_months FROM plans WHERE code = 'free') f
		WHERE true`+only+`
		ON CONFLICT (organization_id) DO UPDATE SET target = EXCLUDED.target, target_since = now()
		WHERE organization_retention.target <> EXCLUDED.target`, args...); err != nil {
		return fmt.Errorf("retention: recording plan targets: %w", err)
	}

	// 2. Months follow the target: longer at once, shorter after the wait.
	only = ""
	args = []any{p.ShortenAfter.Seconds()}
	if p.Only != "" {
		only, args = " AND organization_id = $2", append(args, p.Only)
	}
	moved, err := p.DB.Exec(ctx, `
		UPDATE organization_retention SET months = target, applied_at = NULL, updated_at = now()
		WHERE months <> target
		  AND (target > months OR target_since <= now() - make_interval(secs => $1))`+only, args...)
	if err != nil {
		return fmt.Errorf("retention: moving to plan targets: %w", err)
	}
	if moved.RowsAffected() > 0 {
		if err := p.Load(ctx); err != nil {
			return err
		}
	}

	// 3. Rewrite the rows of organizations whose months settled.
	only = ""
	args = []any{settle.Seconds()}
	if p.Only != "" {
		only, args = " AND organization_id = $2", append(args, p.Only)
	}
	rows, err := p.DB.Query(ctx, `
		SELECT organization_id, months FROM organization_retention
		WHERE applied_at IS NULL AND updated_at <= now() - make_interval(secs => $1)`+only, args...)
	if err != nil {
		return fmt.Errorf("retention: listing rewrites: %w", err)
	}
	byMonths := map[uint8][]string{}
	for rows.Next() {
		var org string
		var m int
		if err := rows.Scan(&org, &m); err != nil {
			rows.Close()
			return fmt.Errorf("retention: listing rewrites: %w", err)
		}
		byMonths[uint8(m)] = append(byMonths[uint8(m)], org)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("retention: listing rewrites: %w", err)
	}
	for m, orgs := range byMonths {
		if err := chstore.SetRawRetention(ctx, p.CH, m, orgs); err != nil {
			return err
		}
		if _, err := p.DB.Exec(ctx, `UPDATE organization_retention SET applied_at = now() WHERE organization_id = ANY($1) AND months = $2`,
			orgs, int(m)); err != nil {
			return fmt.Errorf("retention: recording rewrites: %w", err)
		}
		p.Logger.Info("raw retention applied", "months", m, "organizations", len(orgs))
	}
	return nil
}
