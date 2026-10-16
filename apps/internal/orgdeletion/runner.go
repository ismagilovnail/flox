package orgdeletion

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Interval is how often apps/worker looks for deletions that are due and
// tombstones to purge.
const Interval = 5 * time.Minute

// PurgeDelay is how long after the Postgres deletion the other stores are
// purged: the tracker serves a deleted organization's links until its
// routing cache drops them (seconds), and events already queued (NATS,
// the worker's buffer) land in ClickHouse after that. Purging earlier
// would leave those behind.
const PurgeDelay = time.Hour

// Storage is the object store; *storage.Client is the real one.
type Storage interface {
	DeletePrefix(ctx context.Context, prefix string) error
}

// Runner is apps/worker's half. CH, Storage and Redis may be nil when not
// configured; the purge then skips that store and says so in the
// tombstone's last_error.
type Runner struct {
	DB      *pgxpool.Pool
	CH      driver.Conn
	Storage Storage
	Redis   *redis.Client
	Logger  *slog.Logger
	// Organization limits a run to one organization — for tests, which
	// share the databases with the dev stack. Empty: every organization.
	Organization string
}

func (r *Runner) RunLoop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if err := r.RunOnce(ctx); err != nil && ctx.Err() == nil {
			r.Logger.Error("organization deletion run failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RunOnce deletes the organizations that are due, then purges the
// tombstones old enough.
func (r *Runner) RunOnce(ctx context.Context) error {
	for {
		deleted, err := r.deleteNext(ctx)
		if err != nil {
			return err
		}
		if !deleted {
			break
		}
	}
	return r.purgeDue(ctx, PurgeDelay)
}

// deleteNext deletes one due organization, if there is one: the
// tombstone, the organizations row (every tenant table cascades) and the
// users who belonged to it and to no other organization, in one
// transaction. The row is locked FOR UPDATE SKIP LOCKED, so two workers
// never take the same one, and a cancel racing it either lands first (the
// row no longer matches) or waits for the delete.
func (r *Runner) deleteNext(ctx context.Context) (bool, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("orgdeletion: beginning: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var orgID string
	var requestedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT id, deletion_requested_at FROM organizations
		WHERE deletion_scheduled_at <= now() AND ($1 = '' OR id = $1)
		ORDER BY deletion_scheduled_at
		LIMIT 1
		FOR UPDATE SKIP LOCKED`, r.Organization).Scan(&orgID, &requestedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("orgdeletion: finding a due deletion: %w", err)
	}

	users, err := usersToDelete(ctx, tx, orgID)
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO organization_deletions (organization_id, requested_at, users_deleted)
		VALUES ($1, $2, $3)`, orgID, requestedAt, len(users)); err != nil {
		return false, fmt.Errorf("orgdeletion: writing the tombstone: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, orgID); err != nil {
		return false, fmt.Errorf("orgdeletion: deleting organization %s: %w", orgID, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = ANY($1)`, users); err != nil {
		return false, fmt.Errorf("orgdeletion: deleting its users: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("orgdeletion: committing: %w", err)
	}
	r.Logger.Info("organization deleted", "organization_id", orgID, "users_deleted", len(users))
	return true, nil
}

// usersToDelete are the organization's members (active or invited) who
// belong to no other organization. A user still referenced from another
// organization's rows by a foreign key without a delete action (a member
// who left one that keeps their audit entries, API keys or cost entries)
// stays: deleting them would fail the whole transaction, and the account
// without memberships holds no organization's data.
func usersToDelete(ctx context.Context, tx pgx.Tx, orgID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT u.id FROM users u
		WHERE EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = u.id AND m.organization_id = $1)
		  AND NOT EXISTS (SELECT 1 FROM memberships o WHERE o.user_id = u.id AND o.organization_id <> $1)
		  AND NOT EXISTS (SELECT 1 FROM audit_logs a WHERE a.actor_user_id = u.id AND a.organization_id <> $1)
		  AND NOT EXISTS (SELECT 1 FROM api_keys k WHERE k.created_by_user_id = u.id AND k.organization_id <> $1)
		  AND NOT EXISTS (SELECT 1 FROM cost_entries c WHERE c.created_by_user_id = u.id AND c.organization_id <> $1)`, orgID)
	if err != nil {
		return nil, fmt.Errorf("orgdeletion: finding its users: %w", err)
	}
	defer rows.Close()
	users := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("orgdeletion: reading its users: %w", err)
		}
		users = append(users, id)
	}
	return users, rows.Err()
}

// purgeDue purges every tombstone deleted at least delay ago and not yet
// purged. A store that fails leaves the tombstone unpurged with the error,
// and the next run tries again; each step is idempotent.
func (r *Runner) purgeDue(ctx context.Context, delay time.Duration) error {
	rows, err := r.DB.Query(ctx, `
		SELECT organization_id FROM organization_deletions
		WHERE purged_at IS NULL AND deleted_at <= now() - make_interval(secs => $1)
		  AND ($2 = '' OR organization_id = $2)
		ORDER BY deleted_at`, delay.Seconds(), r.Organization)
	if err != nil {
		return fmt.Errorf("orgdeletion: finding tombstones: %w", err)
	}
	var orgs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("orgdeletion: reading tombstones: %w", err)
		}
		orgs = append(orgs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("orgdeletion: reading tombstones: %w", err)
	}
	for _, orgID := range orgs {
		notes, err := r.purge(ctx, orgID)
		if err != nil {
			r.Logger.Error("organization purge failed, will retry", "organization_id", orgID, "error", err)
			if _, uerr := r.DB.Exec(ctx, `UPDATE organization_deletions SET last_error = $2 WHERE organization_id = $1`, orgID, err.Error()); uerr != nil {
				return fmt.Errorf("orgdeletion: recording purge error: %w", uerr)
			}
			continue
		}
		if _, err := r.DB.Exec(ctx, `UPDATE organization_deletions SET purged_at = now(), last_error = $2 WHERE organization_id = $1`,
			orgID, strings.Join(notes, "; ")); err != nil {
			return fmt.Errorf("orgdeletion: marking purged: %w", err)
		}
		r.Logger.Info("organization purged", "organization_id", orgID, "notes", notes)
	}
	return nil
}

// purge deletes orgID's data outside Postgres. notes say which store was
// skipped because it is not configured.
func (r *Runner) purge(ctx context.Context, orgID string) (notes []string, err error) {
	if r.CH == nil {
		notes = append(notes, "ClickHouse not configured")
	} else if err := purgeClickHouse(ctx, r.CH, orgID); err != nil {
		return nil, err
	}
	if r.Storage == nil {
		notes = append(notes, "object storage not configured")
	} else {
		// Landing bundles (internal/landing) and gallery files
		// (internal/contentgallery). An organization id is a ULID, so
		// "<id>/" is no other organization's prefix.
		for _, prefix := range []string{"landings/" + orgID + "/", orgID + "/"} {
			if err := r.Storage.DeletePrefix(ctx, prefix); err != nil {
				return nil, fmt.Errorf("orgdeletion: deleting objects under %s: %w", prefix, err)
			}
		}
	}
	if r.Redis == nil {
		notes = append(notes, "Redis not configured")
	} else if err := purgeRedis(ctx, r.Redis, orgID); err != nil {
		return nil, err
	}
	return notes, nil
}

// purgeClickHouse deletes orgID's rows from every MergeTree table with an
// organization_id — read from the schema, so a table added later is
// covered without a change here. Lightweight DELETE, waited for.
func purgeClickHouse(ctx context.Context, conn driver.Conn, orgID string) error {
	rows, err := conn.Query(ctx, `
		SELECT t.name FROM system.tables t
		WHERE t.database = currentDatabase() AND t.engine LIKE '%MergeTree'
		  AND t.name IN (SELECT table FROM system.columns WHERE database = currentDatabase() AND name = 'organization_id')
		ORDER BY t.name`)
	if err != nil {
		return fmt.Errorf("orgdeletion: listing ClickHouse tables: %w", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return fmt.Errorf("orgdeletion: listing ClickHouse tables: %w", err)
		}
		tables = append(tables, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("orgdeletion: listing ClickHouse tables: %w", err)
	}
	wait := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"mutations_sync": 2, "lightweight_deletes_sync": 2}))
	for _, table := range tables {
		if err := conn.Exec(wait, "DELETE FROM `"+table+"` WHERE organization_id = ?", orgID); err != nil {
			return fmt.Errorf("orgdeletion: deleting from ClickHouse %s: %w", table, err)
		}
	}
	return nil
}

// purgeRedis deletes the organization's keys: the conversion engine's
// last-status cache (internal/conversion). Rate-limit counters expire in
// minutes on their own.
func purgeRedis(ctx context.Context, rdb *redis.Client, orgID string) error {
	iter := rdb.Scan(ctx, 0, "flox:conversion:last-status:"+orgID+":*", 1000).Iterator()
	var batch []string
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := rdb.Del(ctx, batch...).Err(); err != nil {
			return fmt.Errorf("orgdeletion: deleting Redis keys: %w", err)
		}
		batch = batch[:0]
		return nil
	}
	for iter.Next(ctx) {
		batch = append(batch, iter.Val())
		if len(batch) == 1000 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("orgdeletion: scanning Redis keys: %w", err)
	}
	return flush()
}
