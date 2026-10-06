// Package conversion is Phase 23's conversion engine (§45): it turns an
// inbound postback into a recorded, deduplicated, correctly-attributed CPA
// event, or an honestly-logged reason it wasn't.
//
// Like internal/attribution, this package is pure orchestration — no HTTP,
// no database driver, no clock of its own. It reads and writes through
// Store, Mapper, NetworkLookup and FXConverter, which the caller supplies.
// (DeliveryWriter is the one interface that names pgx.Tx: it runs inside the
// Postgres store's own transaction.)
//
// The two money-correctness rules this package exists to enforce (CLAUDE.md
// non-negotiable #3):
//
//  1. DEDUP KEY is (click_id, status, event_ref), never click_id alone and
//     never (click_id, status) alone. event_ref is the network's
//     transaction id for CPA_REDEP (the one repeatable status) and empty
//     string for everything else, even when the network sent one.
//  2. STATUS PROGRESSION never returns to CPA_HOLD once a click has any
//     other recorded status. Nightly partner replays re-send the original
//     CPA_HOLD hours after approval; applying it would move an approved
//     conversion back to pending. This check is independent of dedup and
//     fires even for networks with acceptDuplicates set — that flag is
//     about duplicate deliveries, not about time travel.
package conversion

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/macro"
)

// Postback is the inbound claim, already stripped of HTTP concerns.
//
// OrganizationID and AcceptDuplicates come from looking up NetworkID — never
// from the request body (CLAUDE.md #5, §36-TENANCY) — which is the caller's
// job via NetworkLookup before constructing this.
type Postback struct {
	OrganizationID   string
	NetworkID        string
	AcceptDuplicates bool

	ClickID         string
	ExternalClickID string

	// RawStatus is exactly what the network sent, before Event Mapping
	// translates it.
	RawStatus string

	// Revenue/Currency are nil/empty when the network's postback carries no
	// money (e.g. a CPA_HOLD registration ping). A present-but-zero revenue
	// is NOT the same as absent — CLAUDE.md #6's "cost or it doesn't exist"
	// spirit applies here too.
	Revenue  *float64
	Currency string

	// NetworkTxnID is the network's own transaction/order id, when sent.
	// Stored regardless of whether this status's dedup key uses it (§45).
	NetworkTxnID string

	// PostbackURL is the network's outgoing macro template (§46, Phase 24),
	// e.g. "...?click_id={click_id}&status={status}" — empty when the
	// network has none configured. Threaded through from the same
	// NetworkLookup the handler already did for OrganizationID, so Service
	// still never needs its own NetworkLookup dependency.
	PostbackURL string

	OccurredAt time.Time
}

// ResultKind is the closed set of outcomes a postback can end in — a
// dashboard filter ("today's ignored postbacks"), not free text.
type ResultKind string

const (
	// ResultSuccess: a new conversion was recorded.
	ResultSuccess ResultKind = "success"
	// ResultDuplicate: the dedup key was already seen; nothing new recorded.
	ResultDuplicate ResultKind = "duplicate"
	// ResultIgnored: understood and logged, but deliberately not applied —
	// today this is only the STATUS PROGRESSION refusal.
	ResultIgnored ResultKind = "ignored"
	// ResultError: could not be processed (no event mapping, missing
	// required fields).
	ResultError ResultKind = "error"
)

// Result is what the caller (the HTTP handler) reports back.
type Result struct {
	ID          string
	Kind        ResultKind
	Status      event.Type
	Attribution attribution.Attribution
	Message     string
}

// Network is what NetworkLookup returns — just enough to scope the postback
// to a tenant and apply its dedup override.
type Network struct {
	ID               string
	OrganizationID   string
	AcceptDuplicates bool
	Status           string
	// PostbackURL is the outgoing macro template (§46) — empty means the
	// network has no outgoing integration configured, and the handler must
	// not populate Postback.PostbackURL in that case.
	PostbackURL string
	// PostbackSecretHash authenticates an INCOMING postback (the opposite
	// direction from PostbackURL) — §54/Phase 30: apps/tracker's
	// PostbackHandler hashes the request's own secret param and compares
	// it against this before ever calling Service.Record. Never exposed
	// outside this package (Network carries no json tags at all — it's
	// never serialized to a client response, only consumed internally).
	PostbackSecretHash string
}

var ErrNetworkNotFound = errors.New("conversion: network not found")

// NetworkLookup resolves a postback URL's {networkId} to the organization
// that owns it. This — not the request body — is where OrganizationID comes
// from (CLAUDE.md #5).
type NetworkLookup interface {
	ByID(ctx context.Context, networkID string) (Network, error)
}

var ErrUnmapped = errors.New("conversion: no event mapping for this network status")

// Mapper translates a network's raw status string to the canonical §43
// CpaStatus, per that network's configured Event Mapping table.
type Mapper interface {
	MapStatus(ctx context.Context, organizationID, networkID, rawStatus string) (event.Type, error)
}

// Entry is one row of the durable ledger: an already-decided outcome, ready
// to persist. Built by Service, written by Store.
type Entry struct {
	OrganizationID   string
	NetworkID        string
	AcceptDuplicates bool

	ClickID  string
	Status   event.Type
	EventRef string

	NetworkTxnID string
	RawStatus    string
	Revenue      *float64
	Currency     string
	USDValue     *float64

	AttributionOutcome string
	AttributedClickID  string
	AttributionMethod  string
	TimeToConversionMS *int64

	// Kind is the INTENDED outcome. ResultSuccess is arbitrated by the
	// dedup unique constraint (Store may downgrade it to ResultDuplicate)
	// and by the progression rule, re-checked under the click's lock (Store
	// may downgrade it to ResultIgnored); ResultIgnored/ResultError are
	// recorded exactly as given.
	Kind    ResultKind
	Message string

	// Event and Delivery are the side effects of a ResultSuccess entry — the
	// CPA event for ClickHouse and the outgoing postback, nil when the
	// network has none. Store writes them in the same transaction as the
	// row, and only if the row lands as ResultSuccess: a recorded conversion
	// without its event, or its delivery, can no longer happen (audit
	// 2026-10-06, D1). Delivery.SourcePostbackID is filled in by Store.
	Event    *event.Event
	Delivery *DeliveryRequest
}

// Recorded is what Store.Record actually wrote.
type Recorded struct {
	ID      string
	Kind    ResultKind
	Message string
}

// Store is the durable ledger — Postgres today (postgres.go), read through
// a Redis cache for the progression check (redis.go). Store alone, without
// any cache in front of it, is already correct; Redis exists purely to save
// the round trip on the fast path (§45).
type Store interface {
	// LastStatus is the most recently recorded ResultSuccess status for
	// this click, used by the progression check. false means no successful
	// conversion has been recorded for this click yet.
	LastStatus(ctx context.Context, organizationID, clickID string) (event.Type, bool, error)

	// Record persists e and returns the outcome that actually happened,
	// which for e.Kind == ResultSuccess may be downgraded to
	// ResultDuplicate by the dedup constraint or to ResultIgnored by the
	// progression rule. A success is written with its Event and Delivery
	// atomically; an error means nothing was written. Idempotent-safe: a
	// caller that retries after an error may call it again without risking
	// a double-count, because the dedup constraint (or, for Ignored/Error,
	// the caller's own re-derivation of the same Entry) makes the retry
	// either a no-op or a repeat log line.
	Record(ctx context.Context, e Entry) (Recorded, error)
}

// FXConverter normalizes revenue to USD using the rate on the event's own
// date — never the current rate (§50-FX, CLAUDE.md #7).
type FXConverter interface {
	// ToUSD returns ok=false (not an error) when no rate is on file for
	// (currency, at.Date()) — a missing rate is never invented, and the
	// conversion is still stored with its original currency/amount and a
	// nil USD value rather than being dropped.
	ToUSD(ctx context.Context, currency string, amount float64, at time.Time) (usd float64, ok bool, err error)
}

// DeliveryRequest is the outgoing postback (internal/postback, Phase 24/§46)
// a successfully recorded conversion queues.
type DeliveryRequest struct {
	OrganizationID string
	NetworkID      string
	// SourcePostbackID is the postbacks row this delivery was triggered by
	// — the FK internal/postback's queue traces every attempt back to.
	SourcePostbackID string
	ClickID          string
	Status           event.Type
	// URL is already macro-resolved (see conversion.go's buildDeliveryURL);
	// the delivery engine dispatches it as-is and never re-templates.
	URL string
}

// DeliveryWriter queues an outgoing postback inside the transaction that
// records its source conversion — implemented by internal/postback, which
// owns the delivery queue, so this package never imports it.
type DeliveryWriter interface {
	InsertTx(ctx context.Context, tx pgx.Tx, req DeliveryRequest) error
}

// AttemptRecord is what Service reports to the postback attempt audit log
// (internal/postbacklog, §48's postback_events) after EVERY incoming
// postback — success, duplicate, ignored, and error alike, mirroring §45's
// "log every postback... with replay ability".
type AttemptRecord struct {
	OrganizationID string
	NetworkID      string
	ClickID        string
	Status         event.Type
	EventRef       string
	RawStatus      string
	Result         ResultKind
	Message        string
	Revenue        *float64
	Currency       string
	OccurredAt     time.Time
}

// AttemptLogger is the narrow slice of internal/postbacklog this package
// needs — a decoupled interface with no error return: an already-processed
// postback (whatever its outcome)
// must never be reported differently to the network just because this
// secondary audit log's queue insert stumbled.
type AttemptLogger interface {
	LogAttempt(ctx context.Context, rec AttemptRecord)
}

// Service is the conversion engine.
//
// It deliberately has no NetworkLookup: resolving {networkId} to an
// organization happens once, in the HTTP handler, before a Postback even
// exists (CLAUDE.md #5 — OrganizationID must come from the credential, not
// be re-derived deeper in the stack where it's easier to forget the scope).
type Service struct {
	mapper      Mapper
	store       Store
	fx          FXConverter
	attribution attribution.AttributionService
	attempts    AttemptLogger
}

func NewService(mapper Mapper, store Store, fx FXConverter, attr attribution.AttributionService, attempts AttemptLogger) *Service {
	return &Service{mapper: mapper, store: store, fx: fx, attribution: attr, attempts: attempts}
}

// logAttempt reports one outcome to the postback attempt audit log. Called
// from every exit point of Record (the main path, logError, logIgnored)
// with whatever fields are known at that point — an error path before
// mapping has no Status yet, and that's fine, an empty Status is itself
// informative (§48's postback_events schema allows it, same as the
// underlying Postgres ledger does for 'error' rows).
func (s *Service) logAttempt(ctx context.Context, p Postback, clickID string, status event.Type, eventRef string, result ResultKind, message string) {
	s.attempts.LogAttempt(ctx, AttemptRecord{
		OrganizationID: p.OrganizationID,
		NetworkID:      p.NetworkID,
		ClickID:        clickID,
		Status:         status,
		EventRef:       eventRef,
		RawStatus:      strings.TrimSpace(p.RawStatus),
		Result:         result,
		Message:        message,
		Revenue:        p.Revenue,
		Currency:       strings.TrimSpace(p.Currency),
		OccurredAt:     p.OccurredAt,
	})
}

// Record runs one postback through mapping, the progression check,
// attribution, FX normalization and the dedup ledger; on success the CPA
// event and the outgoing postback are committed with the ledger row. It never returns an error for a postback it understood and
// handled (unmapped status, missing fields, progression refusal are all
// ResultKind values, not errors) — err is reserved for infrastructure
// failure, where the honest answer is "retry me," not a fabricated result.
func (s *Service) Record(ctx context.Context, p Postback) (Result, error) {
	clickID := strings.TrimSpace(p.ClickID)
	rawStatus := strings.TrimSpace(p.RawStatus)

	if clickID == "" {
		return s.logError(ctx, p, "", "", "missing click_id")
	}
	if rawStatus == "" {
		return s.logError(ctx, p, "", "", "missing status")
	}
	// The HTTP handler already answers 400 for these; this guards every other
	// caller (postback replay) so a bad amount never reaches the ledger.
	if p.Revenue != nil {
		if err := CheckRevenue(*p.Revenue); err != nil {
			return s.logError(ctx, p, clickID, "", err.Error())
		}
	}

	status, err := s.mapper.MapStatus(ctx, p.OrganizationID, p.NetworkID, rawStatus)
	if errors.Is(err, ErrUnmapped) {
		return s.logError(ctx, p, clickID, "",
			fmt.Sprintf("no event mapping configured for network status %q", rawStatus))
	}
	if err != nil {
		return Result{}, fmt.Errorf("conversion: mapping status: %w", err)
	}

	eventRef := eventRefFor(status, p.NetworkTxnID)

	// Fast path, possibly from the Redis cache: skips attribution for an
	// obvious replay. Store re-checks under the click's lock, which is what
	// actually decides when two postbacks for one click race.
	last, hasLast, err := s.store.LastStatus(ctx, p.OrganizationID, clickID)
	if err != nil {
		return Result{}, fmt.Errorf("conversion: checking progression: %w", err)
	}
	if refusesProgression(last, hasLast, status) {
		return s.logIgnored(ctx, p, clickID, status, eventRef, progressionRefusal(clickID, last))
	}

	attr, err := s.attribution.AttributeConversion(ctx, attribution.Conversion{
		OrganizationID:  p.OrganizationID,
		ClickID:         clickID,
		ExternalClickID: strings.TrimSpace(p.ExternalClickID),
		Status:          status,
		OccurredAt:      p.OccurredAt,
	})
	if err != nil {
		return Result{}, fmt.Errorf("conversion: attributing: %w", err)
	}

	usdValue, revenue := s.convertRevenue(ctx, p)

	var ttcMS *int64
	if attr.Outcome.Attributed() {
		ms := attr.TimeToConversion.Milliseconds()
		ttcMS = &ms
	}

	entry := Entry{
		OrganizationID:     p.OrganizationID,
		NetworkID:          p.NetworkID,
		AcceptDuplicates:   p.AcceptDuplicates,
		ClickID:            clickID,
		Status:             status,
		EventRef:           eventRef,
		NetworkTxnID:       strings.TrimSpace(p.NetworkTxnID),
		RawStatus:          rawStatus,
		Revenue:            revenue,
		Currency:           strings.TrimSpace(p.Currency),
		USDValue:           usdValue,
		AttributionOutcome: string(attr.Outcome),
		AttributedClickID:  attr.Click.ClickID,
		AttributionMethod:  string(attr.Method),
		TimeToConversionMS: ttcMS,
		Kind:               ResultSuccess,
		Message:            attr.Reason,
	}
	ev := buildEvent(p, attr, status, eventRef, revenue, usdValue)
	entry.Event = &ev
	if url := strings.TrimSpace(p.PostbackURL); url != "" {
		entry.Delivery = &DeliveryRequest{
			OrganizationID: p.OrganizationID,
			NetworkID:      p.NetworkID,
			ClickID:        clickID,
			Status:         status,
			URL:            macro.ResolveURL(url, deliveryMacroValues(p, attr, status, revenue)),
		}
	}

	rec, err := s.store.Record(ctx, entry)
	if err != nil {
		return Result{}, fmt.Errorf("conversion: recording: %w", err)
	}
	message := entry.Message
	if rec.Kind == ResultIgnored {
		message = rec.Message
	}

	s.logAttempt(ctx, p, clickID, status, eventRef, rec.Kind, message)

	return Result{ID: rec.ID, Kind: rec.Kind, Status: status, Attribution: attr, Message: message}, nil
}

// MaxRevenue bounds one conversion's payout. postbacks.revenue is
// numeric(14,4) (just under 1e10); anything near that is a broken macro, not
// a sale.
const MaxRevenue = 1e9

// ErrInvalidRevenue is returned by ParseRevenue/CheckRevenue.
var ErrInvalidRevenue = errors.New("conversion: invalid revenue")

// ParseRevenue parses a postback's revenue parameter and applies
// CheckRevenue. strconv.ParseFloat alone accepts "NaN" and "Inf": one NaN
// payout turned every ClickHouse revenue sum it touched into NaN, and a value
// past numeric(14,4) failed the insert with a 500 the network retried forever
// (audit 2026-10-06, S3).
func ParseRevenue(raw string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q is not a number", ErrInvalidRevenue, raw)
	}
	if err := CheckRevenue(v); err != nil {
		return 0, err
	}
	return v, nil
}

// CheckRevenue accepts a finite payout in [0, MaxRevenue]. A reversal is a
// CPA_DECLINE status (§45), never a negative amount.
func CheckRevenue(v float64) error {
	switch {
	case math.IsNaN(v) || math.IsInf(v, 0):
		return fmt.Errorf("%w: must be a finite number", ErrInvalidRevenue)
	case v < 0:
		return fmt.Errorf("%w: must not be negative (a reversal is a decline status)", ErrInvalidRevenue)
	case v > MaxRevenue:
		return fmt.Errorf("%w: above the %.0f limit", ErrInvalidRevenue, MaxRevenue)
	}
	return nil
}

// progressionRefusal is the message of a postback refused by the §45
// progression rule — shared by Service's fast path and Store's locked check.
func progressionRefusal(clickID string, last event.Type) string {
	return fmt.Sprintf("refusing to move click %q from %s back to CPA_HOLD (§45 status progression)", clickID, last)
}

// deliveryMacroValues builds the macro.Values an outgoing postback URL is
// resolved against. Campaign/country/device/subs come from the attributed
// click when there is one — an unattributed conversion (docs/attribution.md)
// still gets delivered, just without those dimensions filled in.
func deliveryMacroValues(p Postback, attr attribution.Attribution, status event.Type, revenue *float64) macro.Values {
	v := macro.Values{
		ClickID:  strings.TrimSpace(p.ClickID),
		Status:   string(status),
		Currency: strings.TrimSpace(p.Currency),
	}
	if revenue != nil {
		v.Revenue = strconv.FormatFloat(*revenue, 'f', -1, 64)
	}
	if attr.Outcome.Attributed() {
		v.CampaignID = attr.Click.CampaignID
		v.OfferID = attr.Click.OfferID
		v.Country = attr.Click.Country
		v.Device = attr.Click.Device
		v.Subs = attr.Click.Subs
	}
	return v
}

func (s *Service) convertRevenue(ctx context.Context, p Postback) (usdValue, revenue *float64) {
	if p.Revenue == nil {
		return nil, nil
	}
	revenue = p.Revenue
	currency := strings.TrimSpace(p.Currency)
	if currency == "" {
		return nil, revenue
	}
	usd, ok, err := s.fx.ToUSD(ctx, currency, *p.Revenue, p.OccurredAt)
	if err != nil || !ok {
		// No rate on file, or the lookup itself failed: store the
		// conversion anyway with a nil USD value rather than inventing a
		// rate or dropping the conversion (CLAUDE.md #7).
		return nil, revenue
	}
	return &usd, revenue
}

func (s *Service) logError(ctx context.Context, p Postback, clickID string, status event.Type, message string) (Result, error) {
	entry := Entry{
		OrganizationID:   p.OrganizationID,
		NetworkID:        p.NetworkID,
		AcceptDuplicates: p.AcceptDuplicates,
		ClickID:          clickID,
		Status:           status,
		RawStatus:        strings.TrimSpace(p.RawStatus),
		Revenue:          p.Revenue,
		Currency:         strings.TrimSpace(p.Currency),
		NetworkTxnID:     strings.TrimSpace(p.NetworkTxnID),
		Kind:             ResultError,
		Message:          message,
	}
	rec, err := s.store.Record(ctx, entry)
	if err != nil {
		return Result{}, fmt.Errorf("conversion: logging error: %w", err)
	}
	s.logAttempt(ctx, p, clickID, status, "", ResultError, message)
	return Result{ID: rec.ID, Kind: ResultError, Status: status, Message: message}, nil
}

func (s *Service) logIgnored(ctx context.Context, p Postback, clickID string, status event.Type, eventRef, message string) (Result, error) {
	entry := Entry{
		OrganizationID:   p.OrganizationID,
		NetworkID:        p.NetworkID,
		AcceptDuplicates: p.AcceptDuplicates,
		ClickID:          clickID,
		Status:           status,
		EventRef:         eventRef,
		RawStatus:        strings.TrimSpace(p.RawStatus),
		Revenue:          p.Revenue,
		Currency:         strings.TrimSpace(p.Currency),
		NetworkTxnID:     strings.TrimSpace(p.NetworkTxnID),
		Kind:             ResultIgnored,
		Message:          message,
	}
	rec, err := s.store.Record(ctx, entry)
	if err != nil {
		return Result{}, fmt.Errorf("conversion: logging ignored: %w", err)
	}
	s.logAttempt(ctx, p, clickID, status, eventRef, ResultIgnored, message)
	return Result{ID: rec.ID, Kind: ResultIgnored, Status: status, Message: message}, nil
}

func buildEvent(p Postback, attr attribution.Attribution, status event.Type, eventRef string, revenue, usdValue *float64) event.Event {
	ev := event.Event{
		Type:           status,
		EventAt:        p.OccurredAt,
		OrganizationID: p.OrganizationID,
		ClickID:        strings.TrimSpace(p.ClickID),
		NetworkID:      p.NetworkID,

		Currency:     strings.TrimSpace(p.Currency),
		EventRef:     eventRef,
		NetworkTxnID: strings.TrimSpace(p.NetworkTxnID),

		AttributionOutcome: string(attr.Outcome),
		AttributionMethod:  string(attr.Method),
		TimeToConversionMS: attr.TimeToConversion.Milliseconds(),
	}
	if revenue != nil {
		ev.Revenue = *revenue
	}
	if usdValue != nil {
		ev.USDValue = *usdValue
		ev.HasUSDValue = true
	}
	if attr.Outcome.Attributed() {
		click := attr.Click
		ev.CampaignID = click.CampaignID
		ev.StreamSetID = click.StreamSetID
		ev.FlowID = click.FlowID
		ev.LandingID = click.LandingID
		ev.OfferID = click.OfferID
		ev.PwaID = click.PwaID
		ev.TrafficSourceID = click.TrafficSourceID
		ev.Country = click.Country
		ev.Region = click.Region
		ev.City = click.City
		ev.Device = click.Device
		ev.Platform = click.Platform
		ev.OS = click.OS
		ev.Browser = click.Browser
		ev.UTMSource = click.UTMSource
		ev.UTMMedium = click.UTMMedium
		ev.UTMCampaign = click.UTMCampaign
		ev.UTMContent = click.UTMContent
		ev.UTMTerm = click.UTMTerm
		ev.Subs = click.Subs
		ev.ExternalClickID = click.ExternalClickID
	}
	return ev
}

// eventRefFor is the dedup key's third component (§45).
//
// CPA_REDEP is the only repeatable status — a second deposit by the same
// user is a second, legitimate conversion, and the network's transaction id
// is the only thing that tells it apart from a re-send of the first. No txn
// id sent means event_ref = "", which means exactly one redeposit is
// recorded per click: a missed redeposit is a support ticket, a
// double-counted one is an incorrect invoice, so the failure is aimed at the
// recoverable side.
//
// Every other status is non-repeatable: event_ref is ALWAYS "", even when
// the network sends a transaction id, because networks commonly retry with
// a fresh txn id per attempt and including it here would turn every retry
// into a new conversion.
func eventRefFor(status event.Type, networkTxnID string) string {
	if status != event.CpaRedep {
		return ""
	}
	return strings.TrimSpace(networkTxnID)
}

// refusesProgression is §45's STATUS PROGRESSION rule: the only refused
// transition is back to CPA_HOLD. Approvals really do get reversed
// (chargebacks -> CPA_DECLINE after CPA_ACCEPT) and reversals really do get
// undone (CPA_ACCEPT after CPA_DECLINE) — only the return to "not decided
// yet" is meaningless, because a click that already has any other status has
// already been decided once.
func refusesProgression(last event.Type, hasLast bool, next event.Type) bool {
	return hasLast && next == event.CpaHold && last != event.CpaHold
}
