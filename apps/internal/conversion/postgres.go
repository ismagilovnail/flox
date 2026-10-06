package conversion

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/eventqueue"
	"github.com/ismagilovnail/flox/apps/internal/fx"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// PostgresStore is the durable ledger: the postbacks table, exactly as
// migration 00013 shapes it. Correct on its own, with or without a Redis
// cache in front of it (redis.go).
type PostgresStore struct {
	db *pgxpool.Pool
	// deliveries queues a success's outgoing postback in the same
	// transaction; nil for read-only users of the store, which then refuse
	// to record an entry that carries one.
	deliveries DeliveryWriter
}

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore { return &PostgresStore{db: db} }

// WithDeliveries returns the store set up to record conversions with their
// outgoing postbacks (internal/postback.PostgresStore).
func (s *PostgresStore) WithDeliveries(w DeliveryWriter) *PostgresStore {
	return &PostgresStore{db: s.db, deliveries: w}
}

var _ Store = (*PostgresStore)(nil)

// LastStatus reads the most recent ResultSuccess row for this click.
//
// Scoped to result = 'success' deliberately: duplicate/ignored/error rows
// share the table (see migration 00013's comment) but never represent an
// applied status change, so they must never influence the progression
// check.
func (s *PostgresStore) LastStatus(ctx context.Context, organizationID, clickID string) (event.Type, bool, error) {
	return lastStatus(ctx, s.db, organizationID, clickID)
}

// querier is the part of *pgxpool.Pool and pgx.Tx this file needs.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func lastStatus(ctx context.Context, q querier, organizationID, clickID string) (event.Type, bool, error) {
	var status string
	err := q.QueryRow(ctx, `
		SELECT status FROM postbacks
		WHERE organization_id = $1 AND click_id = $2 AND result = 'success'
		ORDER BY created_at DESC
		LIMIT 1`,
		organizationID, clickID,
	).Scan(&status)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("conversion: querying last status: %w", err)
	}
	return event.Type(status), true, nil
}

func (s *PostgresStore) FindSuccessID(ctx context.Context, organizationID, networkID, clickID, status, eventRef string) (string, bool, error) {
	var id string
	err := s.db.QueryRow(ctx, `
		SELECT id FROM postbacks
		WHERE organization_id = $1 AND network_id = $2 AND click_id = $3
		  AND status = $4 AND event_ref = $5 AND direction = 'incoming' AND result = 'success'
		ORDER BY created_at DESC
		LIMIT 1`,
		organizationID, networkID, clickID, status, eventRef,
	).Scan(&id)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("conversion: finding source postback: %w", err)
	}
	return id, true, nil
}

// direction is hardcoded 'incoming': this package only handles postbacks a
// network sends TO FLOX. Outgoing (FLOX notifying a network) is Phase 24's
// postback engine (internal/postback), which reads a success row here via
// source_postback_id but writes its own delivery lifecycle to a separate
// postback_deliveries table — see that migration's comment for why sharing
// this one wasn't the right call after all.
const insertColumns = `
	id, organization_id, network_id, click_id, status, direction, result,
	network_accepts_duplicates, event_ref, network_txn_id, raw_status,
	revenue, currency, usd_value, attribution_outcome,
	attributed_click_id, attribution_method, time_to_conversion_ms, message`

// Record persists e. For e.Kind == ResultSuccess, it relies on the partial
// unique index (organization_id, click_id, status, event_ref) WHERE NOT
// network_accepts_duplicates AND result = 'success' as the sole, atomic
// arbiter of "have we already processed this" — no separate check-then-
// insert, so two postbacks racing for the same key can never both win. A
// conflict there does not mean nothing gets written: it means a SECOND row
// gets written with result = 'duplicate', which cannot itself conflict
// (the index only guards 'success' rows), so the attempt is still visible
// for replay (§45) even when it wasn't applied.
//
// A success is one transaction: click lock, progression re-check, the row,
// and its outbox (CPA event + outgoing postback) — see recordSuccess.
func (s *PostgresStore) Record(ctx context.Context, e Entry) (Recorded, error) {
	if e.Kind != ResultSuccess {
		id, err := insertEntry(ctx, s.db, e, e.Kind, e.Message)
		if err != nil {
			return Recorded{}, fmt.Errorf("conversion: recording %s: %w", e.Kind, err)
		}
		return Recorded{ID: id, Kind: e.Kind, Message: e.Message}, nil
	}
	if e.Delivery != nil && s.deliveries == nil {
		// A misconfigured store must fail loudly, not record a conversion
		// whose outgoing postback silently never exists.
		return Recorded{}, errors.New("conversion: entry carries an outgoing delivery but the store has no DeliveryWriter")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Recorded{}, fmt.Errorf("conversion: beginning record tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rec, err := s.recordSuccess(ctx, tx, e)
	if err != nil {
		return Recorded{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Recorded{}, fmt.Errorf("conversion: committing record tx: %w", err)
	}
	return rec, nil
}

// recordSuccess runs inside Record's transaction. Every postback for one
// click is serialized by a transaction-scoped advisory lock, so the
// progression check and the insert are one step: before this, a CPA_HOLD
// replay racing a CPA_ACCEPT could both pass the check and the HOLD land
// last (audit 2026-10-06, D3). The lock key is per (organization, click),
// so unrelated conversions never wait on each other.
func (s *PostgresStore) recordSuccess(ctx context.Context, tx pgx.Tx, e Entry) (Recorded, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		e.OrganizationID+":"+e.ClickID); err != nil {
		return Recorded{}, fmt.Errorf("conversion: locking click: %w", err)
	}

	last, hasLast, err := lastStatus(ctx, tx, e.OrganizationID, e.ClickID)
	if err != nil {
		return Recorded{}, err
	}
	if refusesProgression(last, hasLast, e.Status) {
		msg := progressionRefusal(e.ClickID, last)
		id, err := insertEntry(ctx, tx, e, ResultIgnored, msg)
		if err != nil {
			return Recorded{}, fmt.Errorf("conversion: recording ignored: %w", err)
		}
		return Recorded{ID: id, Kind: ResultIgnored, Message: msg}, nil
	}

	if reversesAccept(e.Status) {
		rev, ok, err := acceptToReverse(ctx, tx, e.OrganizationID, e.ClickID)
		if err != nil {
			return Recorded{}, err
		}
		if ok {
			e.Reversal = &rev
			e.Message = strings.TrimSpace(e.Message + "; reverses the CPA_ACCEPT payout" + describeAmount(rev.Revenue, rev.Currency))
		}
	}
	var revRevenue, revUSD *float64
	revCurrency := ""
	if e.Reversal != nil {
		revRevenue, revUSD, revCurrency = e.Reversal.Revenue, e.Reversal.USDValue, e.Reversal.Currency
	}

	// created_at is the moment of the write, not the column default now()
	// (the transaction's START): LastStatus orders by it, and a transaction
	// that began first but got the lock second must still sort last.
	id := idgen.New()
	var returnedID string
	err = tx.QueryRow(ctx, `
		INSERT INTO postbacks (`+insertColumns+`, created_at, reversal_revenue, reversal_currency, reversal_usd_value)
		VALUES ($1,$2,$3,$4,$5,'incoming','success',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17, clock_timestamp(), $18, $19, $20)
		ON CONFLICT (organization_id, click_id, status, event_ref)
			WHERE NOT network_accepts_duplicates AND result = 'success'
		DO NOTHING
		RETURNING id`,
		id, e.OrganizationID, e.NetworkID, e.ClickID, string(e.Status),
		e.AcceptDuplicates, e.EventRef, e.NetworkTxnID, e.RawStatus,
		e.Revenue, e.Currency, e.USDValue, e.AttributionOutcome,
		e.AttributedClickID, e.AttributionMethod, e.TimeToConversionMS, e.Message,
		revRevenue, revCurrency, revUSD,
	).Scan(&returnedID)

	if errors.Is(err, pgx.ErrNoRows) {
		const msg = "dedup key (click_id, status, event_ref) already recorded"
		dupID, err := insertEntry(ctx, tx, e, ResultDuplicate, msg)
		if err != nil {
			return Recorded{}, fmt.Errorf("conversion: logging duplicate: %w", err)
		}
		return Recorded{ID: dupID, Kind: ResultDuplicate, Message: msg}, nil
	}
	if err != nil {
		return Recorded{}, fmt.Errorf("conversion: recording success: %w", err)
	}

	// The outbox: the CPA event and the outgoing postback commit with the
	// row or not at all (audit 2026-10-06, D1).
	if e.Event != nil {
		ev := *e.Event
		if e.Reversal != nil {
			// The decline books the accept's payout back, on the decline's
			// own date (closed periods keep what they showed).
			ev.Currency = e.Reversal.Currency
			if e.Reversal.Revenue != nil {
				ev.Revenue = -*e.Reversal.Revenue
			}
			if e.Reversal.USDValue != nil {
				ev.USDValue, ev.HasUSDValue = -*e.Reversal.USDValue, true
			}
		}
		if err := eventqueue.InsertTx(ctx, tx, []event.Event{ev}); err != nil {
			return Recorded{}, fmt.Errorf("conversion: queueing cpa event: %w", err)
		}
	}
	if e.Delivery != nil {
		req := *e.Delivery
		req.SourcePostbackID = returnedID
		if err := s.deliveries.InsertTx(ctx, tx, req); err != nil {
			return Recorded{}, fmt.Errorf("conversion: queueing outgoing postback: %w", err)
		}
	}
	return Recorded{ID: returnedID, Kind: ResultSuccess, Message: e.Message}, nil
}

// insertEntry writes one non-success ledger row (error/ignored/duplicate)
// with the given result and message.
func insertEntry(ctx context.Context, q querier, e Entry, kind ResultKind, message string) (string, error) {
	id := idgen.New()
	_, err := q.Exec(ctx, `INSERT INTO postbacks (`+insertColumns+`)
		VALUES ($1,$2,$3,$4,$5,'incoming',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		id, e.OrganizationID, e.NetworkID, e.ClickID, string(e.Status), string(kind),
		e.AcceptDuplicates, e.EventRef, e.NetworkTxnID, e.RawStatus,
		e.Revenue, e.Currency, e.USDValue, e.AttributionOutcome,
		e.AttributedClickID, e.AttributionMethod, e.TimeToConversionMS, message,
	)
	return id, err
}

// PostgresNetworkLookup resolves {networkId} from the postback URL
// (CHANGELOG's api.floxlink.io/postback/{networkId}) to the organization
// that owns it and that network's dedup override.
//
// Looked up by id alone, not scoped to an organization_id the caller
// doesn't have yet — this IS how the organization_id is discovered
// (CLAUDE.md #5), the same role a session/API key plays for the
// control-plane API. A network id is an unguessable ULID, not a slug, so
// this is not an enumeration surface.
type PostgresNetworkLookup struct {
	db *pgxpool.Pool
}

func NewPostgresNetworkLookup(db *pgxpool.Pool) *PostgresNetworkLookup {
	return &PostgresNetworkLookup{db: db}
}

var _ NetworkLookup = (*PostgresNetworkLookup)(nil)

func (l *PostgresNetworkLookup) ByID(ctx context.Context, networkID string) (Network, error) {
	var n Network
	err := l.db.QueryRow(ctx, `
		SELECT id, organization_id, accept_duplicates, status, postback_url, postback_secret_hash
		FROM networks
		WHERE id = $1`,
		networkID,
	).Scan(&n.ID, &n.OrganizationID, &n.AcceptDuplicates, &n.Status, &n.PostbackURL, &n.PostbackSecretHash)

	if errors.Is(err, pgx.ErrNoRows) {
		return Network{}, ErrNetworkNotFound
	}
	if err != nil {
		return Network{}, fmt.Errorf("conversion: looking up network: %w", err)
	}
	return n, nil
}

// PostgresMapper reads the per-network Event Mapping table (§29; Phase 13's
// frontend documented this as "what the real Conversion Engine runs at
// ingest time").
type PostgresMapper struct {
	db *pgxpool.Pool
}

func NewPostgresMapper(db *pgxpool.Pool) *PostgresMapper { return &PostgresMapper{db: db} }

var _ Mapper = (*PostgresMapper)(nil)

// MapStatus matches network_status case-insensitively — networks are
// inconsistent about casing across retries/redeploys of their own postback
// code, and requiring exact-case configuration in the Event Mapping UI buys
// no correctness, only support tickets.
func (m *PostgresMapper) MapStatus(ctx context.Context, organizationID, networkID, rawStatus string) (event.Type, error) {
	var floxStatus string
	err := m.db.QueryRow(ctx, `
		SELECT flox_status FROM event_mappings
		WHERE organization_id = $1 AND network_id = $2 AND lower(network_status) = lower($3)`,
		organizationID, networkID, strings.TrimSpace(rawStatus),
	).Scan(&floxStatus)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUnmapped
	}
	if err != nil {
		return "", fmt.Errorf("conversion: mapping status: %w", err)
	}
	return event.Type(floxStatus), nil
}

// PostgresFX reads fx_rates (§50-FX), kept filled by internal/fx. The rate
// used is the latest published on or before the event's date, at most
// fx.MaxStaleDays old: reference rates skip weekends and holidays, and an
// event before today's fixing takes yesterday's — the rate in force then,
// never a later one. Before the 2026-10-06 audit (AN1) this demanded an exact
// date match on a table nothing filled, so every non-USD amount had no USD
// value.
type PostgresFX struct {
	db *pgxpool.Pool
}

func NewPostgresFX(db *pgxpool.Pool) *PostgresFX { return &PostgresFX{db: db} }

var _ FXConverter = (*PostgresFX)(nil)

func (f *PostgresFX) ToUSD(ctx context.Context, currency string, amount float64, at time.Time) (float64, bool, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return 0, false, nil
	}
	if currency == "USD" {
		return amount, true, nil
	}

	var rate float64
	err := f.db.QueryRow(ctx, `
		SELECT rate_to_usd FROM fx_rates
		WHERE currency = $1 AND rate_date <= $2::date AND rate_date > $2::date - $3::int
		ORDER BY rate_date DESC
		LIMIT 1`,
		currency, at.UTC().Format("2006-01-02"), fx.MaxStaleDays,
	).Scan(&rate)

	if errors.Is(err, pgx.ErrNoRows) {
		// No rate on file for this currency/date. Not an error: the caller
		// stores the conversion with its original currency/amount and a
		// nil USD value rather than inventing a rate (CLAUDE.md #7).
		metrics.FXRateMissingTotal.Inc()
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("conversion: querying fx rate: %w", err)
	}
	return amount * rate, true, nil
}

// acceptToReverse finds the payout a decline takes back: the click's latest
// decision among CPA_ACCEPT / CPA_DECLINE / CPA_TRASH must be an accept —
// one already declined has nothing left to reverse. Runs under the click's
// lock (recordSuccess).
func acceptToReverse(ctx context.Context, q querier, organizationID, clickID string) (Reversal, bool, error) {
	var (
		status   string
		rev      Reversal
		revenue  *float64
		usdValue *float64
	)
	err := q.QueryRow(ctx, `
		SELECT status, revenue::float8, currency, usd_value::float8 FROM postbacks
		WHERE organization_id = $1 AND click_id = $2 AND result = 'success'
		  AND status IN ('CPA_ACCEPT', 'CPA_DECLINE', 'CPA_TRASH')
		ORDER BY created_at DESC
		LIMIT 1`,
		organizationID, clickID,
	).Scan(&status, &revenue, &rev.Currency, &usdValue)
	if errors.Is(err, pgx.ErrNoRows) {
		return Reversal{}, false, nil
	}
	if err != nil {
		return Reversal{}, false, fmt.Errorf("conversion: finding the accept to reverse: %w", err)
	}
	if event.Type(status) != event.CpaAccept {
		return Reversal{}, false, nil
	}
	rev.Revenue, rev.USDValue = revenue, usdValue
	return rev, true, nil
}

func describeAmount(amount *float64, currency string) string {
	if amount == nil {
		return " (none recorded)"
	}
	return fmt.Sprintf(" %s %s", strconv.FormatFloat(*amount, 'f', -1, 64), currency)
}
