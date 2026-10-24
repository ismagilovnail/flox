// Package conversion is the conversion engine (§45): it turns an
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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/macro"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
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

	// NetworkCurrency is the network's payout currency (networks.currency,
	// USD unless the operator chose another). A postback that carries
	// revenue but no currency is taken to be in it — otherwise the amount
	// has no USD value and drops out of every revenue figure (audit
	// 2026-10-15, DATA-1). Like OrganizationID it comes from the network
	// lookup, never from the request.
	NetworkCurrency string
	// currencyAssumed is set when NetworkCurrency stood in for a missing
	// currency; unpriced when the revenue got no USD value (no rate on
	// file). The outcome's message then says so (annotate).
	currencyAssumed bool
	unpriced        bool
	// awaitingMapping is set when the status has no event mapping yet: the
	// refusal is kept for internal/mappingreplay to record once it has one.
	awaitingMapping bool

	// Browser: reported by a page's script at apps/tracker's /c/{networkId}
	// (audit UNI-1 part 2) — no secret, so the engine takes only statuses
	// that map to CPA_HOLD or CPA_ACCEPT, and a click it does not know only
	// when the click is fresh enough to still be on its way (BrowserFreshClick).
	Browser bool

	// NetworkTxnID is the network's own transaction/order id, when sent.
	// Stored regardless of whether this status's dedup key uses it (§45).
	NetworkTxnID string

	// RefundAmount / RefundID: a refund (a status that maps to CPA_DECLINE
	// or CPA_TRASH) of part of the purchase, in the purchase's currency,
	// and the refund's own id so two partial refunds of one order are told
	// apart (audit UNI-4, docs/conversion.md "Refunds by order").
	RefundAmount *float64
	RefundID     string

	// PostbackURL is the network's outgoing macro template (§46), e.g.
	// "...?click_id={click_id}&status={status}" — empty when the network
	// has none. It comes from the handler's NetworkLookup, so Service needs
	// none of its own.
	PostbackURL string

	OccurredAt time.Time
}

// BrowserFreshClick is how old a click a page's script reports may be
// while the click lookup does not know it yet: the tracker's click reaches
// it within seconds, so an older unknown one is made up.
const BrowserFreshClick = 10 * time.Minute

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
	// Currency is the payout currency assumed for a postback with revenue
	// but no currency (Postback.NetworkCurrency).
	Currency string
	// BrowserConversions / BrowserDomains: whether a page's script may
	// report conversions, and from which sites (audit UNI-1 part 2).
	BrowserConversions bool
	BrowserDomains     []string
	// PostbackSecretHash authenticates an incoming postback (§54):
	// apps/tracker's PostbackHandler hashes the request's secret param and
	// compares it with this before calling Service.Record. Network has no
	// json tags: it is never serialized to a client.
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
	// RefundAmount / RefundID: see Postback.
	RefundAmount *float64
	RefundID     string
	USDValue     *float64

	AttributionOutcome string
	AttributedClickID  string
	AttributionMethod  string
	TimeToConversionMS *int64

	// ExternalClickID and EventAt are what the network sent and when the
	// conversion happened; written on every row, so a postback kept for its
	// mapping (AwaitingMapping) can be recorded later as it arrived.
	ExternalClickID string
	EventAt         time.Time
	// AwaitingMapping marks a ResultError row refused only because its
	// status has no event mapping yet (internal/mappingreplay).
	AwaitingMapping bool

	// RetryAttribution schedules the click to be looked up again
	// (AttributionRetryDelays), in the transaction that records a success:
	// set for unknown_click, whose click may simply not be in ClickHouse yet.
	RetryAttribution bool

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
	// row, and only if the row lands as ResultSuccess: a conversion is never
	// recorded without its event or its delivery (tech audit D1).
	// Delivery.SourcePostbackID is filled in by Store.
	Event    *event.Event
	Delivery *DeliveryRequest
	// SourceDelivery is the postback to the attributed click's traffic
	// source (audit 2026-10-15, INT-1), nil when the source has none for
	// this status. Written like Delivery, in the same transaction.
	SourceDelivery *DeliveryRequest
	// Pixel is the event for the pixels of the attributed click's stream
	// set (internal/pixelsend), nil when the conversion is not attributed
	// to a click that went through one. Written in the same transaction;
	// Pixel.SourceEventID is filled in by Store.
	Pixel *pixel.Event

	// Reversal is filled in by Store, under the click's lock: a CPA_DECLINE
	// or CPA_TRASH recorded while the click stands accepted reverses that
	// CPA_ACCEPT's payout (§45; tech audit T4).
	Reversal *Reversal
}

// Reversal is the CPA_ACCEPT payout a decline takes back: the accept's own
// amount and currency, and its USD value at the accept's rate, so the two
// net to exactly zero. Revenue/USDValue are nil when the accept carried
// none.
type Reversal struct {
	Revenue  *float64
	Currency string
	USDValue *float64
	// TargetID is the success row reversed (postbacks.reverses_postback_id);
	// Kind its refund_kind: "" the reversal of the click's accept, a
	// decision on the click; "repeat" a repeat purchase's refund;
	// "partial" a partial refund of the accept (audit UNI-4).
	TargetID string
	Kind     string
}

// reversesAccept reports whether a status, recorded after a CPA_ACCEPT,
// takes the accept's payout back. Redeposits are separate deposits and are
// never reversed by it.
func reversesAccept(status event.Type) bool {
	return status == event.CpaDecline || status == event.CpaTrash
}

// Recorded is what Store.Record actually wrote.
type Recorded struct {
	ID      string
	Kind    ResultKind
	Message string
	// EventRef is the key Store recorded under when it differs from the
	// entry's: a re-approval or its reversal (redecisionFor).
	EventRef string
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

// DeliveryRequest is the outgoing postback (internal/postback, §46)
// a successfully recorded conversion queues.
type DeliveryRequest struct {
	OrganizationID string
	// NetworkID is set for the network's outgoing postback,
	// TrafficSourceID for the postback to the click's traffic source —
	// exactly one of them.
	NetworkID       string
	TrafficSourceID string
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

// PixelWriter queues a conversion's event for its stream set's pixels inside
// the transaction that records it — implemented by internal/pixelsend.
type PixelWriter interface {
	InsertTx(ctx context.Context, tx pgx.Tx, ev pixel.Event) error
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
	// preview is the store's dry-run side (WithPreview), for Check.
	preview Previewer
	// sources looks up the traffic source's postback (WithSourcePostbacks);
	// nil: no postbacks to sources.
	sources SourcePostbacks
}

// SourcePostback is a traffic source's postback: the URL template and the
// statuses it is sent for (traffic_sources.postback_url,
// postback_statuses; audit 2026-10-15, INT-1).
type SourcePostback struct {
	URL      string
	Statuses []event.Type
}

// SourcePostbacks finds a traffic source's postback; ok false when the
// source has none (or is not the organization's).
type SourcePostbacks interface {
	PostbackFor(ctx context.Context, organizationID, sourceID string) (SourcePostback, bool, error)
}

// WithSourcePostbacks makes the engine queue the postback to a converting
// click's traffic source.
func (s *Service) WithSourcePostbacks(sp SourcePostbacks) *Service {
	s.sources = sp
	return s
}

// sourceDelivery is the postback to the attributed click's traffic source
// for this status, or nil. Macro-resolved like a network's
// ({external_click_id} is the source's own click id the click carried).
func (s *Service) sourceDelivery(ctx context.Context, p Postback, attr attribution.Attribution, status event.Type, revenue *float64) (*DeliveryRequest, error) {
	if s.sources == nil || !attr.Outcome.Attributed() || attr.Click.TrafficSourceID == "" {
		return nil, nil
	}
	sp, ok, err := s.sources.PostbackFor(ctx, p.OrganizationID, attr.Click.TrafficSourceID)
	if err != nil {
		return nil, fmt.Errorf("conversion: looking up the traffic source's postback: %w", err)
	}
	if !ok || sp.URL == "" || !slices.Contains(sp.Statuses, status) {
		return nil, nil
	}
	return &DeliveryRequest{
		OrganizationID:  p.OrganizationID,
		TrafficSourceID: attr.Click.TrafficSourceID,
		ClickID:         p.ClickID,
		Status:          status,
		URL:             macro.ResolveURL(sp.URL, deliveryMacroValues(p, attr, status, revenue)),
	}, nil
}

func NewService(mapper Mapper, store Store, fx FXConverter, attr attribution.AttributionService, attempts AttemptLogger) *Service {
	return &Service{mapper: mapper, store: store, fx: fx, attribution: attr, attempts: attempts}
}

// logAttempt reports one outcome to the postback attempt audit log. Called
// from every exit point of Record (recorder.commit, recorder.refuse)
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
		RawStatus:      p.RawStatus,
		Result:         result,
		Message:        message,
		Revenue:        p.Revenue,
		Currency:       p.Currency,
		OccurredAt:     p.OccurredAt,
	})
}

// Record runs one postback through validation, status mapping, the
// progression check, attribution and FX normalization; on success the
// ledger row, the CPA event and the outgoing postback are committed in one
// transaction (Store.Record). It never returns an error for a postback it
// understood and handled (unmapped status, missing fields, progression
// refusal are all ResultKind values, not errors) — err is reserved for
// infrastructure failure, where the honest answer is "retry me," not a
// fabricated result.
func (s *Service) Record(ctx context.Context, p Postback) (Result, error) {
	return s.run(ctx, p, recorder{s})
}

// outcome is the last step of run: what happens to a postback once it is
// decided. recorder writes it (Record); previewer only says what would be
// written (Check).
type outcome interface {
	refuse(ctx context.Context, p Postback, clickID string, status event.Type, eventRef string, kind ResultKind, message string) (Result, error)
	commit(ctx context.Context, p Postback, entry Entry, attr attribution.Attribution) (Result, error)
}

// run is the one decision path of a postback, for Record and Check alike:
// validation, status mapping, the progression fast path, attribution and
// FX normalization, then out.
func (s *Service) run(ctx context.Context, p Postback, out outcome) (Result, error) {
	p = p.trimmed().withNetworkCurrency()

	if refusal := validate(p); refusal != "" {
		// A postback without a status is logged without its click.
		clickID := p.ClickID
		if p.RawStatus == "" {
			clickID = ""
		}
		return out.refuse(ctx, p, clickID, "", "", ResultError, refusal)
	}

	status, err := s.mapper.MapStatus(ctx, p.OrganizationID, p.NetworkID, p.RawStatus)
	if errors.Is(err, ErrUnmapped) {
		// Kept, not lost: the network takes the 200 as delivered and will
		// not send it again (audit 2026-10-15, DATA-4).
		p.awaitingMapping = true
		return out.refuse(ctx, p, p.ClickID, "", "", ResultError, unmappedMessage(p.RawStatus))
	}
	if err != nil {
		return Result{}, fmt.Errorf("conversion: mapping status: %w", err)
	}
	eventRef := eventRefFor(status, p.NetworkTxnID)
	if p.RefundAmount != nil && !reversesAccept(status) {
		return out.refuse(ctx, p, p.ClickID, status, eventRef, ResultError,
			fmt.Sprintf("refund_amount is for a refund (a status that maps to CPA_DECLINE or CPA_TRASH), not %s", status))
	}
	if p.Browser && status != event.CpaHold && status != event.CpaAccept {
		return out.refuse(ctx, p, p.ClickID, status, eventRef, ResultError,
			fmt.Sprintf("a page's script can report only a lead or a purchase (CPA_HOLD, CPA_ACCEPT), not %s — send it from the site's server", status))
	}

	refusal, err := s.checkProgression(ctx, p, status)
	if err != nil {
		return Result{}, err
	}
	if refusal != "" {
		return out.refuse(ctx, p, p.ClickID, status, eventRef, ResultIgnored, refusal)
	}

	attr, err := s.attribute(ctx, p, status)
	if err != nil {
		return Result{}, err
	}
	if p.Browser && attr.Outcome == attribution.OutcomeUnknownClick {
		// A fresh click may still be on its way to the click lookup and
		// is retried like any other; an old unknown one from a page is
		// made up.
		if made, ok := idgen.Time(p.ClickID); !ok || p.OccurredAt.Sub(made) > BrowserFreshClick {
			return out.refuse(ctx, p, p.ClickID, status, eventRef, ResultError, "unknown click: a page's script can report only a click FLOX recorded")
		}
	}
	usdValue, revenue := s.convertRevenue(ctx, p)
	p.unpriced = revenue != nil && usdValue == nil

	entry := newEntry(p, status, eventRef, attr, revenue, usdValue)
	if entry.SourceDelivery, err = s.sourceDelivery(ctx, p, attr, status, revenue); err != nil {
		return Result{}, err
	}
	return out.commit(ctx, p, entry, attr)
}

// trimmed is p with every network-supplied string trimmed — once, here, so
// no later step has to remember to.
func (p Postback) trimmed() Postback {
	p.ClickID = strings.TrimSpace(p.ClickID)
	p.ExternalClickID = strings.TrimSpace(p.ExternalClickID)
	p.RawStatus = strings.TrimSpace(p.RawStatus)
	p.Currency = strings.ToUpper(strings.TrimSpace(p.Currency))
	p.NetworkTxnID = strings.TrimSpace(p.NetworkTxnID)
	p.PostbackURL = strings.TrimSpace(p.PostbackURL)
	return p
}

// withNetworkCurrency fills a missing currency from the network's when the
// postback carries revenue. A postback without revenue keeps no currency:
// there is nothing to price.
func (p Postback) withNetworkCurrency() Postback {
	if p.Revenue == nil || p.Currency != "" {
		return p
	}
	if c := strings.ToUpper(strings.TrimSpace(p.NetworkCurrency)); c != "" {
		p.Currency = c
		p.currencyAssumed = true
	}
	return p
}

// ledgerCurrency is the currency a refused postback's row keeps. One kept
// for its mapping keeps what the network sent, so its replay assumes the
// network's currency afresh — and says so.
func (p Postback) ledgerCurrency() string {
	if p.awaitingMapping && p.currencyAssumed {
		return ""
	}
	return p.Currency
}

// annotate adds to an outcome's message that the currency was assumed, or
// that the revenue could not be priced in USD, so the postback log shows
// why an amount is (or is not) in the reports.
func (p Postback) annotate(message string) string {
	var notes []string
	if p.Browser {
		notes = append(notes, "reported by the page's script, not the site's server")
	}
	if p.currencyAssumed {
		notes = append(notes, fmt.Sprintf("currency not sent, the network's %s assumed", p.Currency))
	}
	if p.unpriced {
		if p.Currency == "" {
			notes = append(notes, "revenue without a currency is left out of revenue reports")
		} else {
			notes = append(notes, fmt.Sprintf("no %s rate for %s, revenue is left out of revenue reports", p.Currency, p.OccurredAt.UTC().Format("2006-01-02")))
		}
	}
	for _, n := range notes {
		if message == "" {
			message = n
		} else {
			message += "; " + n
		}
	}
	return message
}

// validate returns why a postback cannot be recorded, or "". The HTTP
// handler already answers 400 for a bad amount; checking it here guards
// every other caller (postback replay) too.
func validate(p Postback) string {
	switch {
	case p.ClickID == "":
		return "missing click_id"
	case p.RawStatus == "":
		return "missing status"
	}
	if p.Revenue != nil {
		if err := CheckRevenue(*p.Revenue); err != nil {
			return err.Error()
		}
	}
	if p.RefundAmount != nil {
		if err := CheckRevenue(*p.RefundAmount); err != nil {
			return strings.Replace(err.Error(), "revenue", "refund_amount", 1)
		}
		if *p.RefundAmount == 0 {
			return "refund_amount must be more than 0"
		}
	}
	return ""
}

// checkProgression is the fast path of §45's progression rule, possibly
// answered from the Redis cache: it skips attribution for an obvious
// replay. Store re-checks under the click's lock, which is what actually
// decides when two postbacks for one click race.
func (s *Service) checkProgression(ctx context.Context, p Postback, status event.Type) (refusal string, err error) {
	last, hasLast, err := s.store.LastStatus(ctx, p.OrganizationID, p.ClickID)
	if err != nil {
		return "", fmt.Errorf("conversion: checking progression: %w", err)
	}
	if refusesProgression(last, hasLast, status) {
		return progressionRefusal(p.ClickID, last), nil
	}
	return "", nil
}

// attribute finds the click the conversion belongs to. A failing click
// store (ClickHouse down or not connected yet) is not an error: the
// conversion is recorded as unknown_click with a retry, like a click that
// has not arrived yet, and the worker attributes it once the store answers
// — the network gets its answer without waiting on ClickHouse (audit
// 2026-10-06, T1).
func (s *Service) attribute(ctx context.Context, p Postback, status event.Type) (attribution.Attribution, error) {
	attr, err := s.attribution.AttributeConversion(ctx, attribution.Conversion{
		OrganizationID:  p.OrganizationID,
		ClickID:         p.ClickID,
		ExternalClickID: p.ExternalClickID,
		Status:          status,
		OccurredAt:      p.OccurredAt,
	})
	if errors.Is(err, attribution.ErrNoOrganization) {
		return attribution.Attribution{}, fmt.Errorf("conversion: attributing: %w", err)
	}
	if err != nil {
		attr = attribution.Attribution{
			Outcome: attribution.OutcomeUnknownClick,
			Method:  attribution.MethodNone,
			Reason:  fmt.Sprintf("click lookup failed, retried later: %v", err),
		}
	}
	return attr, nil
}

// newEntry is the ledger row of an accepted postback, with the CPA event
// and the outgoing postback that commit together with it.
func newEntry(p Postback, status event.Type, eventRef string, attr attribution.Attribution, revenue, usdValue *float64) Entry {
	var ttcMS *int64
	if attr.Outcome.Attributed() {
		ms := attr.TimeToConversion.Milliseconds()
		ttcMS = &ms
	}
	entry := Entry{
		OrganizationID:     p.OrganizationID,
		NetworkID:          p.NetworkID,
		AcceptDuplicates:   p.AcceptDuplicates,
		ClickID:            p.ClickID,
		Status:             status,
		EventRef:           eventRef,
		RefundAmount:       p.RefundAmount,
		RefundID:           p.RefundID,
		NetworkTxnID:       p.NetworkTxnID,
		RawStatus:          p.RawStatus,
		Revenue:            revenue,
		Currency:           p.Currency,
		USDValue:           usdValue,
		AttributionOutcome: string(attr.Outcome),
		AttributedClickID:  attr.Click.ClickID,
		AttributionMethod:  string(attr.Method),
		TimeToConversionMS: ttcMS,
		RetryAttribution:   attr.Outcome == attribution.OutcomeUnknownClick,
		Kind:               ResultSuccess,
		Message:            attr.Reason,
	}
	ev := buildEvent(p, attr, status, eventRef, revenue, usdValue)
	entry.Event = &ev
	entry.Pixel = pixelEvent(p, attr, status, revenue, usdValue)
	if p.PostbackURL != "" {
		entry.Delivery = &DeliveryRequest{
			OrganizationID: p.OrganizationID,
			NetworkID:      p.NetworkID,
			ClickID:        p.ClickID,
			Status:         status,
			URL:            macro.ResolveURL(p.PostbackURL, deliveryMacroValues(p, attr, status, revenue)),
		}
	}
	return entry
}

// recorder is Record's outcome: the ledger, the outbox and the attempt log.
type recorder struct{ s *Service }

// commit writes an accepted postback. Store may still refuse it under the
// click's lock, or note that a decline reversed an accept's payout; its
// message is then the one reported.
func (r recorder) commit(ctx context.Context, p Postback, entry Entry, attr attribution.Attribution) (Result, error) {
	metrics.AttributionOutcomesTotal.WithLabelValues(string(attr.Outcome)).Inc()
	rec, err := r.s.store.Record(ctx, entry)
	if err != nil {
		return Result{}, fmt.Errorf("conversion: recording: %w", err)
	}
	message := committedMessage(p, entry, rec)
	eventRef := entry.EventRef
	if rec.EventRef != "" {
		eventRef = rec.EventRef // a re-approval's (Store, under the lock)
	}
	r.s.logAttempt(ctx, p, entry.ClickID, entry.Status, eventRef, rec.Kind, message)
	return Result{ID: rec.ID, Kind: rec.Kind, Status: entry.Status, Attribution: attr, Message: message}, nil
}

// committedMessage is what an accepted postback's outcome says: the
// store's message when it applied or refused it (a duplicate's says why —
// for a REDEP without txn_id, which parameter was missing), the
// attribution reason otherwise, with the currency notes.
func committedMessage(p Postback, entry Entry, rec Recorded) string {
	message := entry.Message
	// The store's own word whatever it decided — including a refund it
	// refused under the lock (audit UNI-4).
	if rec.Kind == ResultIgnored || rec.Kind == ResultSuccess || rec.Kind == ResultDuplicate || rec.Kind == ResultError {
		message = rec.Message
	}
	return p.annotate(message)
}

// refuse writes a postback refused before attribution — kind is
// ResultError (unusable input) or ResultIgnored (progression) — to the
// ledger and the attempt log.
func (r recorder) refuse(ctx context.Context, p Postback, clickID string, status event.Type, eventRef string, kind ResultKind, message string) (Result, error) {
	rec, err := r.s.store.Record(ctx, Entry{
		OrganizationID:   p.OrganizationID,
		NetworkID:        p.NetworkID,
		AcceptDuplicates: p.AcceptDuplicates,
		ClickID:          clickID,
		Status:           status,
		EventRef:         eventRef,
		RawStatus:        p.RawStatus,
		Revenue:          p.Revenue,
		Currency:         p.ledgerCurrency(),
		NetworkTxnID:     p.NetworkTxnID,
		ExternalClickID:  p.ExternalClickID,
		EventAt:          p.OccurredAt,
		AwaitingMapping:  p.awaitingMapping,
		Kind:             kind,
		Message:          message,
	})
	if err != nil {
		return Result{}, fmt.Errorf("conversion: logging %s: %w", kind, err)
	}
	message = refusalMessage(p, kind, message)
	r.s.logAttempt(ctx, p, clickID, status, eventRef, kind, message)
	return Result{ID: rec.ID, Kind: kind, Status: status, Message: message}, nil
}

// refusalMessage is what a refusal says. A postback refused as unusable
// records no money, so how its money would have been priced is beside the
// point.
func refusalMessage(p Postback, kind ResultKind, message string) string {
	if kind != ResultError {
		message = p.annotate(message)
	}
	return message
}

// AttributionRetryDelays is when an unknown_click conversion's click is
// looked up again: the first delay after it is recorded, each next one after
// a miss — about 7 hours in all, enough for a long ClickHouse backlog. After
// the last miss unknown_click is final (internal/reattribute).
var AttributionRetryDelays = []time.Duration{
	30 * time.Second, 2 * time.Minute, 10 * time.Minute, time.Hour, 6 * time.Hour,
}

// MappingReplayWindow is how long a postback whose status has no event
// mapping waits for one (internal/mappingreplay); older ones stay refused.
const MappingReplayWindow = 30 * 24 * time.Hour

// unmappedMessage is what a postback refused for its unmapped status says:
// that it waits, and for how long.
func unmappedMessage(rawStatus string) string {
	return fmt.Sprintf("no event mapping configured for network status %q — kept: recorded once the status is mapped (within %d days)",
		rawStatus, int(MappingReplayWindow.Hours()/24))
}

// MaxRevenue bounds one conversion's payout. postbacks.revenue is
// numeric(14,4) (just under 1e10); anything near that is a broken macro, not
// a sale.
const MaxRevenue = 1e9

// ErrInvalidRevenue is returned by ParseRevenue/CheckRevenue.
var ErrInvalidRevenue = errors.New("conversion: invalid revenue")

// ParseRevenue parses a postback's revenue parameter and applies
// CheckRevenue. strconv.ParseFloat alone accepts "NaN" and "Inf": one NaN
// payout turns every ClickHouse revenue sum it touches into NaN, and a value
// past numeric(14,4) fails the insert with a 500 the network retries forever
// (tech audit S3).
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

// Bounds on a network-supplied event time (ParseEventTime).
const (
	// MaxEventAge: an older event is a broken macro or a replay of history
	// that belongs in a reconciliation, not a postback; it would also
	// rewrite reports and FX for a period long closed.
	MaxEventAge = 90 * 24 * time.Hour
	// eventClockSkew: how far ahead of us a network's clock may run; such
	// an event is taken as now.
	eventClockSkew = 5 * time.Minute
)

// ErrInvalidEventTime is returned by ParseEventTime.
var ErrInvalidEventTime = errors.New("conversion: invalid event_time")

// ParseEventTime reads a postback's event_time — when the conversion
// happened at the network, as opposed to when its postback reached us, so
// nightly replays and slow networks land on the right day and FX date
// (tech audit T3). Unix
// seconds or milliseconds, RFC 3339, or "YYYY-MM-DD HH:MM:SS" in UTC. Up
// to 5 minutes ahead of now counts as now; further ahead, or older than
// MaxEventAge, is refused.
func ParseEventTime(raw string, now time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	var t time.Time
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		switch {
		case len(raw) >= 13:
			t = time.UnixMilli(n)
		default:
			t = time.Unix(n, 0)
		}
	} else if v, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		t = v
	} else if v, err := time.Parse(time.DateTime, raw); err == nil {
		t = v
	} else {
		return time.Time{}, fmt.Errorf("%w: %q is not unix seconds or milliseconds, RFC 3339, or YYYY-MM-DD HH:MM:SS (UTC)", ErrInvalidEventTime, raw)
	}
	t = t.UTC()
	switch {
	case t.After(now.Add(eventClockSkew)):
		return time.Time{}, fmt.Errorf("%w: %s is in the future", ErrInvalidEventTime, t.Format(time.RFC3339))
	case t.After(now):
		return now.UTC(), nil
	case now.Sub(t) > MaxEventAge:
		return time.Time{}, fmt.Errorf("%w: %s is older than %d days", ErrInvalidEventTime, t.Format(time.RFC3339), int(MaxEventAge.Hours()/24))
	}
	return t, nil
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
		ClickID:  p.ClickID,
		Status:   string(status),
		Currency: p.Currency,
	}
	if revenue != nil {
		v.Revenue = strconv.FormatFloat(*revenue, 'f', -1, 64)
	}
	if attr.Outcome.Attributed() {
		v.CampaignID = attr.Click.CampaignID
		v.OfferID = attr.Click.OfferID
		v.Country = attr.Click.Country
		v.Device = attr.Click.Device
		v.ExternalClickID = attr.Click.ExternalClickID
		v.Subs = attr.Click.Subs
	}
	return v
}

// pixelEvent is the conversion as its click's stream set's pixels report
// it, or nil: pixels belong to a stream set, so only a conversion attributed
// to a click that went through one has any.
func pixelEvent(p Postback, attr attribution.Attribution, status event.Type, revenue, usdValue *float64) *pixel.Event {
	if !attr.Outcome.Attributed() || attr.Click.StreamSetID == "" {
		return nil
	}
	c := attr.Click
	return &pixel.Event{
		OrganizationID: p.OrganizationID,
		StreamSetID:    c.StreamSetID,
		Type:           status,
		EventAt:        p.OccurredAt,
		ClickID:        c.ClickID,
		ClickAt:        c.OccurredAt,
		Revenue:        revenue,
		Currency:       p.Currency,
		USDValue:       usdValue,
		IP:             c.IP,
		UserAgent:      c.UserAgent,
		FBClickID:      c.FBClickID,
		TTClickID:      c.TTClickID,
		Macros:         deliveryMacroValues(p, attr, status, revenue),
	}
}

// PixelEventFor is the pixel event of a stored conversion attributed after
// it was recorded (internal/reattribute), or nil when its click went
// through no stream set. Its pixels had nothing to go on before, so they
// get it now.
func PixelEventFor(c StoredConversion, click attribution.Click) *pixel.Event {
	p := Postback{OrganizationID: c.OrganizationID, ClickID: c.ClickID, Currency: c.Currency, OccurredAt: c.EventAt}
	attr := attribution.Attribution{Outcome: attribution.Outcome(c.AttributionOutcome), Click: click}
	ev := pixelEvent(p, attr, c.Status, c.Revenue, c.USDValue)
	if ev != nil {
		ev.SourceEventID = c.ID
	}
	return ev
}

func (s *Service) convertRevenue(ctx context.Context, p Postback) (usdValue, revenue *float64) {
	if p.Revenue == nil {
		return nil, nil
	}
	revenue = p.Revenue
	if p.Currency == "" {
		return nil, revenue
	}
	usd, ok, err := s.fx.ToUSD(ctx, p.Currency, *p.Revenue, p.OccurredAt)
	if err != nil || !ok {
		// No rate on file, or the lookup itself failed: store the
		// conversion anyway with a nil USD value rather than inventing a
		// rate or dropping the conversion (CLAUDE.md #7).
		return nil, revenue
	}
	return &usd, revenue
}

func buildEvent(p Postback, attr attribution.Attribution, status event.Type, eventRef string, revenue, usdValue *float64) event.Event {
	return newEvent(eventInput{
		OrganizationID: p.OrganizationID,
		ClickID:        p.ClickID,
		NetworkID:      p.NetworkID,
		Status:         status,
		EventRef:       eventRef,
		NetworkTxnID:   p.NetworkTxnID,
		Currency:       p.Currency,
		Revenue:        revenue,
		USDValue:       usdValue,
		At:             p.OccurredAt,
	}, attr)
}

// eventInput is what a CPA event is built from — a live postback (buildEvent)
// or a stored one (EventFor): one builder, so a repaired event is exactly
// the one the record path would have queued.
type eventInput struct {
	OrganizationID, ClickID, NetworkID string
	Status                             event.Type
	EventRef, NetworkTxnID, Currency   string
	Revenue, USDValue                  *float64
	At                                 time.Time
}

func newEvent(in eventInput, attr attribution.Attribution) event.Event {
	status, revenue, usdValue := in.Status, in.Revenue, in.USDValue
	ev := event.Event{
		Type:           status,
		EventAt:        in.At,
		OrganizationID: in.OrganizationID,
		ClickID:        in.ClickID,
		NetworkID:      in.NetworkID,

		Currency:     in.Currency,
		EventRef:     in.EventRef,
		NetworkTxnID: in.NetworkTxnID,

		AttributionOutcome: string(attr.Outcome),
		AttributionMethod:  string(attr.Method),
		TimeToConversionMS: attr.TimeToConversion.Milliseconds(),
	}
	if reversesAccept(status) {
		// A decline's analytics revenue is only ever the reversal Store
		// puts here — never what the network sent with it, which reports
		// would otherwise add to revenue now that they net declines in.
		ev.Currency = ""
	} else {
		if revenue != nil {
			ev.Revenue = *revenue
		}
		if usdValue != nil {
			ev.USDValue = *usdValue
			ev.HasUSDValue = true
		}
	}
	if attr.Outcome.Attributed() {
		attr.Click.CopyTo(&ev)
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
// into a new conversion. The one exception, a re-approval after a
// reversal and the reversal of that re-approval, needs the click's history
// and is decided by Store under the click's lock (redecisionFor, §45
// RE-APPROVAL).
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

// applyReversal books a decline's reversal on its event: the accept's
// payout, negated, in the accept's currency (conversion.PostgresStore and
// EventFor).
func applyReversal(ev *event.Event, r *Reversal) {
	if r == nil {
		return
	}
	ev.Currency = r.Currency
	if r.Revenue != nil {
		ev.Revenue = -*r.Revenue
	}
	if r.USDValue != nil {
		ev.USDValue, ev.HasUSDValue = -*r.USDValue, true
	}
}

// StoredConversion is a recorded success row as Postgres keeps it
// (PostgresStore.StoredSuccesses) — enough to rebuild its CPA event.
type StoredConversion struct {
	ID, OrganizationID, NetworkID, ClickID string
	Status                                 event.Type
	EventRef, NetworkTxnID, Currency       string
	Revenue, USDValue                      *float64
	Reversal                               *Reversal
	AttributionOutcome, AttributionMethod  string
	AttributedClickID                      string
	TimeToConversionMS                     *int64
	EventAt                                time.Time
}

// EventFor rebuilds the CPA event a stored conversion queued when it was
// recorded — same builder, same reversal rule, same id (its postbacks row) —
// for the reconciliation to put back what ClickHouse lost or garbled.
// click is the attributed click (nil when there was none or it is gone).
func EventFor(c StoredConversion, click *attribution.Click) event.Event {
	attr := attribution.Attribution{
		Outcome: attribution.Outcome(c.AttributionOutcome),
		Method:  attribution.Method(c.AttributionMethod),
	}
	if c.TimeToConversionMS != nil {
		attr.TimeToConversion = time.Duration(*c.TimeToConversionMS) * time.Millisecond
	}
	if click != nil {
		attr.Click = *click
	} else if attr.Outcome.Attributed() {
		attr.Outcome = attribution.OutcomeUnknownClick // dimensions cannot be restored
	}
	ev := newEvent(eventInput{
		OrganizationID: c.OrganizationID,
		ClickID:        c.ClickID,
		NetworkID:      c.NetworkID,
		Status:         c.Status,
		EventRef:       c.EventRef,
		NetworkTxnID:   c.NetworkTxnID,
		Currency:       c.Currency,
		Revenue:        c.Revenue,
		USDValue:       c.USDValue,
		At:             c.EventAt,
	}, attr)
	applyReversal(&ev, c.Reversal)
	ev.ID = c.ID
	return ev
}
