// Package mappingreplay records the postbacks that arrived before their
// status was mapped (audit 2026-10-15, DATA-4).
//
// A postback whose network status no event mapping knows is answered 200
// "error": the request itself was fine, and a network retries only what
// failed. It used to be lost for good, however soon the operator mapped the
// status. Now internal/conversion keeps its ledger row as awaiting_mapping,
// with what the network sent and when the conversion happened, and this
// package records it — through the same conversion.Service a live postback
// goes through — once a mapping for its status exists, for
// conversion.MappingReplayWindow after it arrived.
//
// The rows of a run are locked (FOR UPDATE SKIP LOCKED) until each is
// recorded and cleared, so two workers never take the same one. A worker
// that dies after recording a row but before clearing it leaves the row
// awaiting; the next run records it again and the dedup key answers
// "duplicate" — nothing is counted twice (for a network with
// acceptDuplicates, the one place a retry can count twice anyway).
package mappingreplay

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// Interval is how often apps/worker looks for postbacks to record: a
// mapping saved by the operator takes effect within it.
const Interval = 30 * time.Second

// Recorder is the conversion engine; *conversion.Service is the real one.
type Recorder interface {
	Record(ctx context.Context, p conversion.Postback) (conversion.Result, error)
}

type Replayer struct {
	db       *pgxpool.Pool
	recorder Recorder
	logger   *slog.Logger
	// org limits a run to one organization — for tests, which share the
	// database with the dev worker and with each other.
	org string
}

func New(db *pgxpool.Pool, recorder Recorder, logger *slog.Logger) *Replayer {
	return &Replayer{db: db, recorder: recorder, logger: logger}
}

// ForOrganization returns a replayer that only touches orgID's postbacks.
func (r *Replayer) ForOrganization(orgID string) *Replayer {
	c := *r
	c.org = orgID
	return &c
}

func (r *Replayer) RunLoop(ctx context.Context, interval time.Duration, batch int) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if n, err := r.RunOnce(ctx, batch); err != nil && ctx.Err() == nil {
			r.logger.Warn("mapping replay run failed", "error", err, "recorded", n)
		} else if n > 0 {
			r.logger.Info("recorded postbacks that waited for their status mapping", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// parked is an awaiting_mapping row with what its network needs to record it.
type parked struct {
	id string
	p  conversion.Postback
}

// RunOnce records up to limit waiting postbacks whose status is mapped now,
// oldest event first — a HOLD before the ACCEPT that followed it — and
// returns how many it recorded. It stops at the first failure: the rows
// left stay awaiting for the next run.
func (r *Replayer) RunOnce(ctx context.Context, limit int) (int, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("mappingreplay: beginning: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := r.claim(ctx, tx, limit)
	if err != nil {
		return 0, err
	}
	done := 0
	for _, row := range rows {
		res, err := r.recorder.Record(ctx, row.p)
		if err != nil {
			// Infrastructure, not the postback: keep what was cleared so
			// far and leave the rest for the next run.
			if cerr := tx.Commit(ctx); cerr != nil {
				return 0, fmt.Errorf("mappingreplay: recording %s: %w (and committing: %v)", row.id, err, cerr)
			}
			return done, fmt.Errorf("mappingreplay: recording %s: %w", row.id, err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE postbacks SET awaiting_mapping = false, replayed_at = now() WHERE id = $1 AND organization_id = $2`,
			row.id, row.p.OrganizationID); err != nil {
			return done, fmt.Errorf("mappingreplay: clearing %s: %w", row.id, err)
		}
		r.logger.Info("recorded a postback that waited for its status mapping",
			"postback_id", row.id, "organization_id", row.p.OrganizationID, "network_id", row.p.NetworkID,
			"raw_status", row.p.RawStatus, "result", res.Kind, "new_postback_id", res.ID)
		done++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("mappingreplay: committing: %w", err)
	}
	return done, nil
}

// claim locks the waiting rows of a run. The network is read with the row,
// so the replay uses its settings as they are now (dedup override,
// currency, outgoing URL), like a postback arriving now would.
func (r *Replayer) claim(ctx context.Context, tx pgx.Tx, limit int) ([]parked, error) {
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.organization_id, p.network_id, p.click_id, p.external_click_id, p.raw_status,
		       p.network_txn_id, p.revenue::float8, p.currency, p.event_at,
		       n.accept_duplicates, n.currency, n.postback_url
		FROM postbacks p
		JOIN networks n ON n.id = p.network_id AND n.organization_id = p.organization_id
		WHERE p.awaiting_mapping
		  AND p.created_at > now() - make_interval(secs => $1)
		  AND ($3 = '' OR p.organization_id = $3)
		  AND EXISTS (SELECT 1 FROM event_mappings m
		              WHERE m.organization_id = p.organization_id AND m.network_id = p.network_id
		                AND lower(m.network_status) = lower(p.raw_status))
		ORDER BY p.event_at, p.created_at
		LIMIT $2
		FOR UPDATE OF p SKIP LOCKED`,
		conversion.MappingReplayWindow.Seconds(), limit, r.org)
	if err != nil {
		return nil, fmt.Errorf("mappingreplay: claiming: %w", err)
	}
	defer rows.Close()
	var out []parked
	for rows.Next() {
		var (
			row      parked
			revenue  *float64
			currency string
			at       time.Time
		)
		if err := rows.Scan(&row.id, &row.p.OrganizationID, &row.p.NetworkID, &row.p.ClickID, &row.p.ExternalClickID,
			&row.p.RawStatus, &row.p.NetworkTxnID, &revenue, &currency, &at,
			&row.p.AcceptDuplicates, &row.p.NetworkCurrency, &row.p.PostbackURL); err != nil {
			return nil, fmt.Errorf("mappingreplay: reading: %w", err)
		}
		row.p.Revenue, row.p.Currency, row.p.OccurredAt = revenue, currency, at.UTC()
		out = append(out, row)
	}
	return out, rows.Err()
}

// Waiting is one network status postbacks are waiting to have mapped.
type Waiting struct {
	NetworkID string    `json:"networkId"`
	RawStatus string    `json:"rawStatus"`
	Count     int       `json:"count"`
	LastAt    time.Time `json:"lastAt"`
}

// WaitingStatuses lists, per network, the statuses of orgID's postbacks
// waiting for a mapping within the window, most frequent first. Scoped to
// orgID (CLAUDE.md #5).
func WaitingStatuses(ctx context.Context, db *pgxpool.Pool, orgID string) ([]Waiting, error) {
	rows, err := db.Query(ctx, `
		SELECT network_id, min(raw_status), count(*), max(created_at)
		FROM postbacks
		WHERE organization_id = $1 AND awaiting_mapping
		  AND created_at > now() - make_interval(secs => $2)
		GROUP BY network_id, lower(raw_status)
		ORDER BY count(*) DESC, min(raw_status)`,
		orgID, conversion.MappingReplayWindow.Seconds())
	if err != nil {
		return nil, fmt.Errorf("mappingreplay: listing waiting statuses: %w", err)
	}
	defer rows.Close()
	out := []Waiting{}
	for rows.Next() {
		var w Waiting
		if err := rows.Scan(&w.NetworkID, &w.RawStatus, &w.Count, &w.LastAt); err != nil {
			return nil, fmt.Errorf("mappingreplay: reading waiting statuses: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

type Handler struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

func NewHandler(db *pgxpool.Pool, logger *slog.Logger) *Handler {
	return &Handler{db: db, logger: logger}
}

// Register mounts GET /waiting on the /event-mappings group. The caller
// applies tenant.Middleware and the read gate.
func (h *Handler) Register(r chi.Router) {
	r.Get("/waiting", h.waiting)
}

func (h *Handler) waiting(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	statuses, err := WaitingStatuses(r.Context(), h.db, orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"statuses": statuses})
}
