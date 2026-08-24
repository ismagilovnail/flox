package conversion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// PostgresStore is the durable ledger: the postbacks table, exactly as
// migration 00013 shapes it. Correct on its own, with or without a Redis
// cache in front of it (redis.go).
type PostgresStore struct {
	db *pgxpool.Pool
}

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore { return &PostgresStore{db: db} }

var _ Store = (*PostgresStore)(nil)

// LastStatus reads the most recent ResultSuccess row for this click.
//
// Scoped to result = 'success' deliberately: duplicate/ignored/error rows
// share the table (see migration 00013's comment) but never represent an
// applied status change, so they must never influence the progression
// check.
func (s *PostgresStore) LastStatus(ctx context.Context, organizationID, clickID string) (event.Type, bool, error) {
	var status string
	err := s.db.QueryRow(ctx, `
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

// FindSuccessID resolves the postbacks row a successful incoming attempt
// was recorded as, given the exact dedup key (organization_id, click_id,
// status, event_ref) plus network_id — the id apps/internal/postback's
// delivery queue needs as source_postback_id (a NOT NULL FK, migration
// 00014). Record already has this id in-process at enqueue time (see its
// own doc comment); this method exists for the one case that doesn't —
// re-enqueuing a delivery for an attempt Record finished long ago
// (apps/internal/postbacklogs' outgoing replay).
//
// eventRef is "" for every status except CPA_REDEP (§45) — a legitimate
// value, not an omitted filter, since a CPA_REDEP click can have more than
// one successful row (one per redeposit, each with its own event_ref) and
// matching on the wrong one would replay the wrong delivery.
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
func (s *PostgresStore) Record(ctx context.Context, e Entry) (id string, actual ResultKind, err error) {
	id = idgen.New()

	if e.Kind != ResultSuccess {
		if _, err := s.db.Exec(ctx, `INSERT INTO postbacks (`+insertColumns+`)
			VALUES ($1,$2,$3,$4,$5,'incoming',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
			id, e.OrganizationID, e.NetworkID, e.ClickID, string(e.Status), string(e.Kind),
			e.AcceptDuplicates, e.EventRef, e.NetworkTxnID, e.RawStatus,
			e.Revenue, e.Currency, e.USDValue, e.AttributionOutcome,
			e.AttributedClickID, e.AttributionMethod, e.TimeToConversionMS, e.Message,
		); err != nil {
			return "", "", fmt.Errorf("conversion: recording %s: %w", e.Kind, err)
		}
		return id, e.Kind, nil
	}

	var returnedID string
	err = s.db.QueryRow(ctx, `
		INSERT INTO postbacks (`+insertColumns+`)
		VALUES ($1,$2,$3,$4,$5,'incoming','success',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT (organization_id, click_id, status, event_ref)
			WHERE NOT network_accepts_duplicates AND result = 'success'
		DO NOTHING
		RETURNING id`,
		id, e.OrganizationID, e.NetworkID, e.ClickID, string(e.Status),
		e.AcceptDuplicates, e.EventRef, e.NetworkTxnID, e.RawStatus,
		e.Revenue, e.Currency, e.USDValue, e.AttributionOutcome,
		e.AttributedClickID, e.AttributionMethod, e.TimeToConversionMS, e.Message,
	).Scan(&returnedID)

	if errors.Is(err, pgx.ErrNoRows) {
		dupID := idgen.New()
		if _, dupErr := s.db.Exec(ctx, `INSERT INTO postbacks (`+insertColumns+`)
			VALUES ($1,$2,$3,$4,$5,'incoming','duplicate',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
			dupID, e.OrganizationID, e.NetworkID, e.ClickID, string(e.Status),
			e.AcceptDuplicates, e.EventRef, e.NetworkTxnID, e.RawStatus,
			e.Revenue, e.Currency, e.USDValue, e.AttributionOutcome,
			e.AttributedClickID, e.AttributionMethod, e.TimeToConversionMS,
			"dedup key (click_id, status, event_ref) already recorded",
		); dupErr != nil {
			return "", "", fmt.Errorf("conversion: logging duplicate: %w", dupErr)
		}
		return dupID, ResultDuplicate, nil
	}
	if err != nil {
		return "", "", fmt.Errorf("conversion: recording success: %w", err)
	}
	return returnedID, ResultSuccess, nil
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
		SELECT id, organization_id, accept_duplicates, status, postback_url
		FROM networks
		WHERE id = $1`,
		networkID,
	).Scan(&n.ID, &n.OrganizationID, &n.AcceptDuplicates, &n.Status, &n.PostbackURL)

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

// PostgresFX reads fx_rates (§50-FX), keyed by (currency, rate_date).
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
		WHERE currency = $1 AND rate_date = $2`,
		currency, at.UTC().Format("2006-01-02"),
	).Scan(&rate)

	if errors.Is(err, pgx.ErrNoRows) {
		// No rate on file for this currency/date. Not an error: the caller
		// stores the conversion with its original currency/amount and a
		// nil USD value rather than inventing a rate (CLAUDE.md #7).
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("conversion: querying fx rate: %w", err)
	}
	return amount * rate, true, nil
}
