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
	// pixels queues a success's pixel events in the same transaction; nil:
	// the store records conversions without sending them to pixels.
	pixels PixelWriter
}

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore { return &PostgresStore{db: db} }

// WithDeliveries returns the store set up to record conversions with their
// outgoing postbacks (internal/postback.PostgresStore).
func (s *PostgresStore) WithDeliveries(w DeliveryWriter) *PostgresStore {
	return &PostgresStore{db: s.db, deliveries: w, pixels: s.pixels}
}

// WithPixels returns the store set up to also queue conversions for their
// stream set's pixels (internal/pixelsend.Store).
func (s *PostgresStore) WithPixels(w PixelWriter) *PostgresStore {
	return &PostgresStore{db: s.db, deliveries: s.deliveries, pixels: w}
}

var (
	_ Store     = (*PostgresStore)(nil)
	_ Previewer = (*PostgresStore)(nil)
)

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
// network sends to FLOX. Outgoing ones (internal/postback) point at a
// success row here through source_postback_id and keep their own delivery
// lifecycle in postback_deliveries.
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
	if (e.Delivery != nil || e.SourceDelivery != nil) && s.deliveries == nil {
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

// Preview is Record's success path run in a transaction that is always
// rolled back (Previewer): the click lock, the progression re-check, the
// reversal of an accept, the dedup constraint and the outbox all run as
// they would, and none of it stays. A NOTIFY or a sequence step inside it
// is undone or harmless.
func (s *PostgresStore) Preview(ctx context.Context, e Entry) (Recorded, error) {
	if e.Kind != ResultSuccess {
		return Recorded{Kind: e.Kind, Message: e.Message}, nil
	}
	if (e.Delivery != nil || e.SourceDelivery != nil) && s.deliveries == nil {
		return Recorded{}, errors.New("conversion: entry carries an outgoing delivery but the store has no DeliveryWriter")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Recorded{}, fmt.Errorf("conversion: beginning preview tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rec, err := s.recordSuccess(ctx, tx, e)
	if err != nil {
		return Recorded{}, err
	}
	rec.ID = ""
	return rec, nil
}

// recordSuccess runs inside Record's transaction. Every postback for one
// click is serialized by a transaction-scoped advisory lock, so the
// progression check and the insert are one step: a CPA_HOLD replay racing
// a CPA_ACCEPT cannot both pass the check and land the HOLD last (tech
// audit D3). The lock key is per (organization, click), so unrelated
// conversions never wait on each other.
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

	// A re-approval after a reversal, and the reversal of that re-approval,
	// are keyed by their txn_id (audit 2026-10-15, DATA-3); under the lock,
	// so the decision reads the click's settled history.
	rd, err := redecisionFor(ctx, tx, e)
	if err != nil {
		return Recorded{}, err
	}
	if rd.eventRef != "" {
		e.EventRef = rd.eventRef
		if e.Event != nil {
			e.Event.EventRef = rd.eventRef
		}
		e.Message = strings.TrimSpace(e.Message + "; " + rd.note)
	}

	refundKind := ""
	if reversesAccept(e.Status) {
		plan, err := planReversal(ctx, tx, e)
		if err != nil {
			return Recorded{}, err
		}
		if plan.refusal != "" {
			id, err := insertEntry(ctx, tx, e, ResultError, plan.refusal)
			if err != nil {
				return Recorded{}, fmt.Errorf("conversion: recording a refused refund: %w", err)
			}
			return Recorded{ID: id, Kind: ResultError, Message: plan.refusal}, nil
		}
		if plan.eventRef != e.EventRef {
			e.EventRef = plan.eventRef
			if e.Event != nil {
				e.Event.EventRef = plan.eventRef
			}
			rd.eventRef = plan.eventRef
		}
		e.Reversal = plan.reversal
		refundKind = plan.kind
		if plan.note != "" {
			e.Message = strings.TrimSpace(strings.TrimPrefix(e.Message+"; "+plan.note, "; "))
		}
	}
	var revRevenue, revUSD *float64
	var revTarget *string
	revCurrency := ""
	if e.Reversal != nil {
		revRevenue, revUSD, revCurrency = e.Reversal.Revenue, e.Reversal.USDValue, e.Reversal.Currency
		if e.Reversal.TargetID != "" {
			revTarget = &e.Reversal.TargetID
		}
	}
	// event_at is the analytics event's own time, so the reconciliation
	// (internal/reconcile) buckets this row on the same day ClickHouse does.
	var eventAt *time.Time
	if e.Event != nil && !e.Event.EventAt.IsZero() {
		t := e.Event.EventAt
		eventAt = &t
	}

	// created_at is the moment of the write, not the column default now()
	// (the transaction's START): LastStatus orders by it, and a transaction
	// that began first but got the lock second must still sort last.
	id := idgen.New()
	var returnedID string
	err = tx.QueryRow(ctx, `
		INSERT INTO postbacks (`+insertColumns+`, created_at, reversal_revenue, reversal_currency, reversal_usd_value, event_at,
		                       reverses_postback_id, refund_kind)
		VALUES ($1,$2,$3,$4,$5,'incoming','success',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17, clock_timestamp(), $18, $19, $20,
		        COALESCE($21, clock_timestamp()), $22, $23)
		ON CONFLICT (organization_id, click_id, status, event_ref)
			WHERE NOT network_accepts_duplicates AND result = 'success'
		DO NOTHING
		RETURNING id`,
		id, e.OrganizationID, e.NetworkID, e.ClickID, string(e.Status),
		e.AcceptDuplicates, e.EventRef, e.NetworkTxnID, e.RawStatus,
		e.Revenue, e.Currency, e.USDValue, e.AttributionOutcome,
		e.AttributedClickID, e.AttributionMethod, e.TimeToConversionMS, e.Message,
		revRevenue, revCurrency, revUSD, eventAt,
		revTarget, refundKind,
	).Scan(&returnedID)

	if errors.Is(err, pgx.ErrNoRows) {
		msg := duplicateMessage(e, rd)
		dupID, err := insertEntry(ctx, tx, e, ResultDuplicate, msg)
		if err != nil {
			return Recorded{}, fmt.Errorf("conversion: logging duplicate: %w", err)
		}
		return Recorded{ID: dupID, Kind: ResultDuplicate, Message: msg, EventRef: rd.eventRef}, nil
	}
	if err != nil {
		return Recorded{}, fmt.Errorf("conversion: recording success: %w", err)
	}

	// The outbox: the CPA event, the outgoing postback and the pixel events
	// commit with the row or not at all (tech audit D1).
	if e.Event != nil {
		ev := *e.Event
		// The conversion's event is identified by its postbacks row, so
		// the reconciliation can match and repair it row by row.
		ev.ID = returnedID
		// The decline books the accept's payout back, on the decline's own
		// date (closed periods keep what they showed).
		applyReversal(&ev, e.Reversal)
		if err := eventqueue.InsertTx(ctx, tx, []event.Event{ev}); err != nil {
			return Recorded{}, fmt.Errorf("conversion: queueing cpa event: %w", err)
		}
	}
	for _, d := range []*DeliveryRequest{e.Delivery, e.SourceDelivery} {
		if d == nil {
			continue
		}
		req := *d
		req.SourcePostbackID = returnedID
		if err := s.deliveries.InsertTx(ctx, tx, req); err != nil {
			return Recorded{}, fmt.Errorf("conversion: queueing outgoing postback: %w", err)
		}
	}
	if e.Pixel != nil && s.pixels != nil {
		ev := *e.Pixel
		ev.SourceEventID = returnedID
		if err := s.pixels.InsertTx(ctx, tx, ev); err != nil {
			return Recorded{}, fmt.Errorf("conversion: queueing pixel events: %w", err)
		}
	}
	if e.RetryAttribution {
		if _, err := tx.Exec(ctx, `
			INSERT INTO attribution_retries (postback_id, organization_id, next_attempt_at)
			VALUES ($1, $2, clock_timestamp() + make_interval(secs => $3))`,
			returnedID, e.OrganizationID, AttributionRetryDelays[0].Seconds()); err != nil {
			return Recorded{}, fmt.Errorf("conversion: scheduling attribution retry: %w", err)
		}
	}
	return Recorded{ID: returnedID, Kind: ResultSuccess, Message: e.Message, EventRef: rd.eventRef}, nil
}

// duplicateMessage says why a postback is a duplicate. A REDEP without a
// transaction id has an empty event_ref, so every repeat deposit of the
// click after the first lands here: the message names the parameter that
// tells them apart (audit 2026-10-15, DATA-2; §45 keeps the key as is).
func duplicateMessage(e Entry, r redecision) string {
	const base = "dedup key (click_id, status, event_ref) already recorded"
	switch {
	case e.Status == event.CpaRedep && e.EventRef == "":
		return base + " — this repeat purchase came without txn_id, so every repeat purchase of the click counts once; send the network's transaction id as txn_id"
	case r.reapprovalRefused:
		return base + " — the click was declined after its accept; a re-approval counts only with a txn_id no earlier accept of the click carried"
	}
	return base
}

// redecision is how a CPA_ACCEPT / CPA_DECLINE / CPA_TRASH relates to the
// click's earlier decisions (audit 2026-10-15, DATA-3; §45 amended).
//
// The dedup key of a non-repeatable status has an empty event_ref, so once
// a click was accepted and declined a later accept was a duplicate — even
// a real re-approval after a dispute. A re-approval is now told apart by
// its txn_id: an accept after a reversal whose txn_id no earlier accept of
// the click carried — recorded or refused: a retry with a fresh id per
// attempt was a duplicate, and its replay must stay one — is keyed by that
// txn_id. The nightly replay of the
// original accept carries the original txn_id (or none) and stays a
// duplicate, so a chargeback is never undone by a replay. A reversal of a
// re-approval is keyed the same way: a decline/trash carrying the txn_id a
// re-approval was keyed by; a replay of the first decline (its txn_id, or
// none) stays a duplicate.
type redecision struct {
	eventRef string // "" = the usual key
	note     string
	// reapprovalRefused: an accept after a reversal that did not qualify
	// (no txn_id, or one an earlier accept carried) — the duplicate's
	// message says why.
	reapprovalRefused bool
}

func redecisionFor(ctx context.Context, q querier, e Entry) (redecision, error) {
	if e.EventRef != "" || (e.Status != event.CpaAccept && !reversesAccept(e.Status)) {
		return redecision{}, nil
	}
	txn := strings.TrimSpace(e.NetworkTxnID)
	if e.Status == event.CpaAccept {
		var last string
		err := q.QueryRow(ctx, `
			SELECT status FROM postbacks
			WHERE organization_id = $1 AND click_id = $2 AND result = 'success'
			  AND status IN ('CPA_ACCEPT', 'CPA_DECLINE', 'CPA_TRASH') AND refund_kind = ''
			ORDER BY created_at DESC
			LIMIT 1`,
			e.OrganizationID, e.ClickID,
		).Scan(&last)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !reversesAccept(event.Type(last))) {
			return redecision{}, nil
		}
		if err != nil {
			return redecision{}, fmt.Errorf("conversion: reading the click's last decision: %w", err)
		}
		if txn == "" {
			return redecision{reapprovalRefused: true}, nil
		}
		var seen bool
		if err := q.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM postbacks
			WHERE organization_id = $1 AND click_id = $2 AND direction = 'incoming'
			  AND status = 'CPA_ACCEPT' AND network_txn_id <> '' AND network_txn_id = $3)`,
			e.OrganizationID, e.ClickID, txn,
		).Scan(&seen); err != nil {
			return redecision{}, fmt.Errorf("conversion: looking up earlier accepts: %w", err)
		}
		if seen {
			return redecision{reapprovalRefused: true}, nil
		}
		return redecision{eventRef: txn, note: "re-approval after " + last + " (txn_id " + txn + ")"}, nil
	}
	if txn == "" {
		return redecision{}, nil
	}
	var reapproved bool
	if err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM postbacks
		WHERE organization_id = $1 AND click_id = $2 AND result = 'success'
		  AND status = 'CPA_ACCEPT' AND event_ref = $3)`,
		e.OrganizationID, e.ClickID, txn,
	).Scan(&reapproved); err != nil {
		return redecision{}, fmt.Errorf("conversion: looking up the re-approval: %w", err)
	}
	if !reapproved {
		return redecision{}, nil
	}
	return redecision{eventRef: txn, note: "reverses the re-approval (txn_id " + txn + ")"}, nil
}

// insertEntry writes one non-success ledger row (error/ignored/duplicate)
// with the given result and message.
func insertEntry(ctx context.Context, q querier, e Entry, kind ResultKind, message string) (string, error) {
	id := idgen.New()
	var eventAt *time.Time
	if !e.EventAt.IsZero() {
		eventAt = &e.EventAt
	}
	_, err := q.Exec(ctx, `INSERT INTO postbacks (`+insertColumns+`, external_click_id, awaiting_mapping, event_at)
		VALUES ($1,$2,$3,$4,$5,'incoming',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20, COALESCE($21, now()))`,
		id, e.OrganizationID, e.NetworkID, e.ClickID, string(e.Status), string(kind),
		e.AcceptDuplicates, e.EventRef, e.NetworkTxnID, e.RawStatus,
		e.Revenue, e.Currency, e.USDValue, e.AttributionOutcome,
		e.AttributedClickID, e.AttributionMethod, e.TimeToConversionMS, message,
		e.ExternalClickID, e.AwaitingMapping && kind == ResultError, eventAt,
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
		SELECT id, organization_id, accept_duplicates, status, postback_url, postback_secret_hash, currency,
		       browser_conversions, browser_domains
		FROM networks
		WHERE id = $1`,
		networkID,
	).Scan(&n.ID, &n.OrganizationID, &n.AcceptDuplicates, &n.Status, &n.PostbackURL, &n.PostbackSecretHash, &n.Currency,
		&n.BrowserConversions, &n.BrowserDomains)

	if errors.Is(err, pgx.ErrNoRows) {
		return Network{}, ErrNetworkNotFound
	}
	if err != nil {
		return Network{}, fmt.Errorf("conversion: looking up network: %w", err)
	}
	return n, nil
}

// PostgresMapper reads the per-network Event Mapping table (§29).
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
// never a later one (tech audit AN1).
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
	// The click's last decision: a refund of one order (refund_kind
	// 'repeat'/'partial') is not one (audit UNI-4).
	err := q.QueryRow(ctx, `
		SELECT id, status, revenue::float8, currency, usd_value::float8 FROM postbacks
		WHERE organization_id = $1 AND click_id = $2 AND result = 'success'
		  AND status IN ('CPA_ACCEPT', 'CPA_DECLINE', 'CPA_TRASH') AND refund_kind = ''
		ORDER BY created_at DESC
		LIMIT 1`,
		organizationID, clickID,
	).Scan(&rev.TargetID, &status, &revenue, &rev.Currency, &usdValue)
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

// reversalPlan is what a CPA_DECLINE / CPA_TRASH reverses (audit UNI-4,
// §45 amended; docs/conversion.md "Refunds by order").
type reversalPlan struct {
	reversal *Reversal // nil: nothing to reverse
	// kind is the row's refund_kind: "" a decision on the click (the
	// reversal of its accept, or a decline with nothing standing), "repeat"
	// or "partial" — a refund of one order, even when nothing was left.
	kind     string
	eventRef string // the dedup key's event_ref
	note     string // for the row's message
	refusal  string // refused, with this reason
}

// planReversal decides, under the click's lock:
//
//   - which purchase: the repeat purchase whose txn_id the refund carries,
//     else the click's accept while it stands (the reversal of a
//     re-approval keeps its key and reverses the latest accept, as before);
//   - how much: refund_amount, at most what is left of it; else all that is
//     left of it — the whole payout unless partial refunds came first;
//   - its key: a repeat purchase's refund is keyed by that txn_id, a partial
//     refund adds "#refund" and its refund_id, so a replay is a duplicate
//     and two partial refunds of one order are not.
func planReversal(ctx context.Context, q querier, e Entry) (reversalPlan, error) {
	plan := reversalPlan{eventRef: e.EventRef}
	txn := strings.TrimSpace(e.NetworkTxnID)
	var (
		target Reversal
		found  bool
		err    error
		what   = "CPA_ACCEPT"
	)
	if e.EventRef == "" && txn != "" {
		target, found, err = redepToReverse(ctx, q, e.OrganizationID, e.ClickID, txn)
		if err != nil {
			return plan, err
		}
		if found {
			target.Kind, plan.eventRef, what = "repeat", txn, "CPA_REDEP"
			plan.kind = "repeat"
		}
	}
	if !found {
		if target, found, err = acceptToReverse(ctx, q, e.OrganizationID, e.ClickID); err != nil {
			return plan, err
		}
		if e.RefundAmount != nil {
			target.Kind, plan.kind = "partial", "partial"
		}
	}
	if e.RefundAmount != nil {
		plan.eventRef += "#refund"
		if e.RefundID != "" {
			plan.eventRef += ":" + e.RefundID
		}
	}
	if !found {
		return plan, nil
	}
	if e.RefundAmount != nil && e.Currency != "" && target.Currency != "" && !strings.EqualFold(e.Currency, target.Currency) {
		plan.refusal = fmt.Sprintf("refund_amount must be in the purchase's currency, %s, not %s", target.Currency, e.Currency)
		return plan, nil
	}
	if target.Revenue == nil {
		// A purchase recorded without money: there is nothing to take back
		// but the decision itself (as before).
		plan.reversal = &target
		plan.note = "reverses the " + what + " payout" + describeAmount(nil, "")
		return plan, nil
	}
	reversed, err := reversedSoFar(ctx, q, target.TargetID)
	if err != nil {
		return plan, err
	}
	left := *target.Revenue - reversed
	if left <= 0 {
		plan.note = what + " " + describeTxn(txn, what) + "already refunded in full; nothing reversed"
		return plan, nil
	}
	amount := left
	if e.RefundAmount != nil && *e.RefundAmount < left {
		amount = *e.RefundAmount
	}
	rev := target
	rev.Revenue = &amount
	// The whole payout keeps the purchase's own USD value, so the two net
	// to exactly zero; a part of it takes the same share.
	if target.USDValue != nil && *target.Revenue > 0 && amount != *target.Revenue {
		usd := *target.USDValue * amount / *target.Revenue
		rev.USDValue = &usd
	}
	plan.reversal = &rev
	if amount == *target.Revenue {
		plan.note = "reverses the " + what + " payout" + describeAmount(target.Revenue, target.Currency)
	} else {
		plan.note = "reverses" + describeAmount(&amount, target.Currency) + " of the " + what + " payout" + describeAmount(target.Revenue, target.Currency)
	}
	if what == "CPA_REDEP" {
		plan.note += " (txn_id " + txn + ")"
	}
	return plan, nil
}

func describeTxn(txn, what string) string {
	if what == "CPA_REDEP" {
		return "(txn_id " + txn + ") "
	}
	return ""
}

// redepToReverse is the click's recorded repeat purchase with txn_id txn.
func redepToReverse(ctx context.Context, q querier, organizationID, clickID, txn string) (Reversal, bool, error) {
	var (
		rev      Reversal
		revenue  *float64
		usdValue *float64
	)
	err := q.QueryRow(ctx, `
		SELECT id, revenue::float8, currency, usd_value::float8 FROM postbacks
		WHERE organization_id = $1 AND click_id = $2 AND result = 'success'
		  AND status = 'CPA_REDEP' AND event_ref = $3
		ORDER BY created_at DESC
		LIMIT 1`,
		organizationID, clickID, txn,
	).Scan(&rev.TargetID, &revenue, &rev.Currency, &usdValue)
	if errors.Is(err, pgx.ErrNoRows) {
		return Reversal{}, false, nil
	}
	if err != nil {
		return Reversal{}, false, fmt.Errorf("conversion: finding the repeat purchase to reverse: %w", err)
	}
	rev.Revenue, rev.USDValue = revenue, usdValue
	return rev, true, nil
}

// reversedSoFar sums what earlier refunds took back from one purchase.
func reversedSoFar(ctx context.Context, q querier, targetID string) (float64, error) {
	var sum float64
	err := q.QueryRow(ctx, `
		SELECT coalesce(sum(reversal_revenue), 0)::float8 FROM postbacks
		WHERE reverses_postback_id = $1 AND result = 'success'`, targetID).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("conversion: summing earlier refunds: %w", err)
	}
	return sum, nil
}

func describeAmount(amount *float64, currency string) string {
	if amount == nil {
		return " (none recorded)"
	}
	return fmt.Sprintf(" %s %s", strconv.FormatFloat(*amount, 'f', -1, 64), currency)
}

// storedColumns are what scanStored reads from postbacks.
const storedColumns = `
	id, organization_id, network_id, click_id, status, event_ref, network_txn_id,
	revenue::float8, currency, usd_value::float8,
	reversal_revenue::float8, reversal_currency, reversal_usd_value::float8,
	attribution_outcome, attribution_method, attributed_click_id, time_to_conversion_ms, event_at`

func scanStored(row pgx.Row) (StoredConversion, error) {
	var (
		c                  StoredConversion
		st                 string
		revRevenue, revUSD *float64
		revCurrency        string
	)
	if err := row.Scan(&c.ID, &c.OrganizationID, &c.NetworkID, &c.ClickID, &st, &c.EventRef, &c.NetworkTxnID,
		&c.Revenue, &c.Currency, &c.USDValue, &revRevenue, &revCurrency, &revUSD,
		&c.AttributionOutcome, &c.AttributionMethod, &c.AttributedClickID, &c.TimeToConversionMS, &c.EventAt); err != nil {
		return StoredConversion{}, err
	}
	c.Status = event.Type(st)
	if revRevenue != nil || revUSD != nil {
		c.Reversal = &Reversal{Revenue: revRevenue, Currency: revCurrency, USDValue: revUSD}
	}
	return c, nil
}

// StoredSuccesses reads the recorded incoming successes of one organization
// and status with event_at in [from, to) — what internal/reconcile rebuilds
// ClickHouse events from.
func (s *PostgresStore) StoredSuccesses(ctx context.Context, organizationID string, status event.Type, from, to time.Time) ([]StoredConversion, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+storedColumns+`
		FROM postbacks
		WHERE organization_id = $1 AND status = $2 AND result = 'success' AND direction = 'incoming'
		  AND event_at >= $3 AND event_at < $4`,
		organizationID, string(status), from, to)
	if err != nil {
		return nil, fmt.Errorf("conversion: reading stored successes: %w", err)
	}
	defer rows.Close()
	var out []StoredConversion
	for rows.Next() {
		c, err := scanStored(rows)
		if err != nil {
			return nil, fmt.Errorf("conversion: scanning stored success: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ErrStoredNotFound is returned by StoredSuccess.
var ErrStoredNotFound = errors.New("conversion: stored success not found")

// StoredSuccess reads one recorded incoming success of an organization by
// its postbacks id (internal/reattribute).
func (s *PostgresStore) StoredSuccess(ctx context.Context, organizationID, id string) (StoredConversion, error) {
	c, err := scanStored(s.db.QueryRow(ctx, `
		SELECT `+storedColumns+`
		FROM postbacks
		WHERE organization_id = $1 AND id = $2 AND result = 'success' AND direction = 'incoming'`,
		organizationID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredConversion{}, ErrStoredNotFound
	}
	if err != nil {
		return StoredConversion{}, fmt.Errorf("conversion: reading stored success: %w", err)
	}
	return c, nil
}

// PostgresSourcePostbacks reads traffic sources' postbacks
// (SourcePostbacks), scoped to the organization.
type PostgresSourcePostbacks struct{ db *pgxpool.Pool }

func NewPostgresSourcePostbacks(db *pgxpool.Pool) *PostgresSourcePostbacks {
	return &PostgresSourcePostbacks{db: db}
}

func (p *PostgresSourcePostbacks) PostbackFor(ctx context.Context, organizationID, sourceID string) (SourcePostback, bool, error) {
	var url string
	var statuses []string
	err := p.db.QueryRow(ctx, `
		SELECT postback_url, postback_statuses FROM traffic_sources
		WHERE id = $1 AND organization_id = $2`, sourceID, organizationID).Scan(&url, &statuses)
	if errors.Is(err, pgx.ErrNoRows) {
		return SourcePostback{}, false, nil
	}
	if err != nil {
		return SourcePostback{}, false, err
	}
	sp := SourcePostback{URL: url}
	for _, st := range statuses {
		sp.Statuses = append(sp.Statuses, event.Type(st))
	}
	return sp, url != "", nil
}
