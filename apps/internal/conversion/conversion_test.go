package conversion_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
)

func ctx() context.Context { return context.Background() }

const orgA = "org-a"

// fakeMapper is a per-network raw-status -> CpaStatus table, mirroring
// event_mappings without a database.
type fakeMapper struct {
	mappings map[string]event.Type // key: networkID + "|" + rawStatus
}

func newFakeMapper() *fakeMapper { return &fakeMapper{mappings: map[string]event.Type{}} }

func (m *fakeMapper) set(networkID, rawStatus string, status event.Type) {
	m.mappings[networkID+"|"+rawStatus] = status
}

func (m *fakeMapper) MapStatus(_ context.Context, _, networkID, rawStatus string) (event.Type, error) {
	status, ok := m.mappings[networkID+"|"+rawStatus]
	if !ok {
		return "", conversion.ErrUnmapped
	}
	return status, nil
}

// fakeStore is an in-memory Store standing in for PostgresStore, enforcing
// the exact same dedup-key, progression and "one success per key" semantics
// so the service-level tests exercise the real contract, not a stub that
// always says yes. Like PostgresStore's outbox, it emits a success's Event
// and Delivery only when the entry actually lands as a success.
type fakeStore struct {
	mu         sync.Mutex
	success    map[string]event.Type // click id -> current status, success rows only
	dedup      map[string]bool       // dedup key -> seen (only for !AcceptDuplicates)
	records    []conversion.Entry
	events     *fakeEvents
	deliveries *fakeDeliveries
}

func newFakeStore(events *fakeEvents, deliveries *fakeDeliveries) *fakeStore {
	return &fakeStore{success: map[string]event.Type{}, dedup: map[string]bool{}, events: events, deliveries: deliveries}
}

func (s *fakeStore) LastStatus(_ context.Context, organizationID, clickID string) (event.Type, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, ok := s.success[organizationID+"|"+clickID]
	return status, ok, nil
}

func (s *fakeStore) Record(_ context.Context, e conversion.Entry) (conversion.Recorded, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, e)

	if e.Kind != conversion.ResultSuccess {
		return conversion.Recorded{ID: "id", Kind: e.Kind, Message: e.Message}, nil
	}
	if last, ok := s.success[e.OrganizationID+"|"+e.ClickID]; ok && last != event.CpaHold && e.Status == event.CpaHold {
		return conversion.Recorded{ID: "id", Kind: conversion.ResultIgnored, Message: "refused under lock"}, nil
	}

	key := e.OrganizationID + "|" + e.ClickID + "|" + string(e.Status) + "|" + e.EventRef
	if !e.AcceptDuplicates && s.dedup[key] {
		return conversion.Recorded{ID: "id", Kind: conversion.ResultDuplicate}, nil
	}
	if !e.AcceptDuplicates {
		s.dedup[key] = true
	}
	s.success[e.OrganizationID+"|"+e.ClickID] = e.Status
	if e.Event != nil && s.events != nil {
		s.events.Enqueue(*e.Event)
	}
	if e.Delivery != nil && s.deliveries != nil {
		req := *e.Delivery
		req.SourcePostbackID = "id"
		s.deliveries.Enqueue(req)
	}
	return conversion.Recorded{ID: "id", Kind: conversion.ResultSuccess, Message: e.Message}, nil
}

// fakeFX always converts at a fixed rate, or refuses currencies it wasn't
// told about — enough to exercise "no rate on file" without a database.
type fakeFX struct {
	rates map[string]float64
}

func (f *fakeFX) ToUSD(_ context.Context, currency string, amount float64, _ time.Time) (float64, bool, error) {
	if currency == "USD" {
		return amount, true, nil
	}
	rate, ok := f.rates[currency]
	if !ok {
		return 0, false, nil
	}
	return amount * rate, true, nil
}

// fakeEvents collects the CPA events fakeStore's outbox emitted.
type fakeEvents struct {
	mu     sync.Mutex
	events []event.Event
}

func (e *fakeEvents) Enqueue(ev event.Event) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, ev)
	return true
}

func (e *fakeEvents) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.events)
}

func (e *fakeEvents) last() event.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.events[len(e.events)-1]
}

// fakeDeliveries collects the deliveries fakeStore's outbox queued.
type fakeDeliveries struct {
	mu   sync.Mutex
	reqs []conversion.DeliveryRequest
}

func (d *fakeDeliveries) Enqueue(req conversion.DeliveryRequest) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reqs = append(d.reqs, req)
}

func (d *fakeDeliveries) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.reqs)
}

func (d *fakeDeliveries) last() conversion.DeliveryRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.reqs[len(d.reqs)-1]
}

// fakeAttempts collects whatever AttemptRecords Service logged.
type fakeAttempts struct {
	mu   sync.Mutex
	recs []conversion.AttemptRecord
}

func (a *fakeAttempts) LogAttempt(_ context.Context, rec conversion.AttemptRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.recs = append(a.recs, rec)
}

func (a *fakeAttempts) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.recs)
}

// attributionOf builds a AttributionService that always attributes to one
// fixed click, or always reports OutcomeUnknownClick, for tests that don't
// care about attribution's own logic (that's attribution package's job).
type fakeAttribution struct {
	outcome attribution.Outcome
	click   attribution.Click
	err     error
}

func (a *fakeAttribution) AttributeConversion(_ context.Context, c attribution.Conversion) (attribution.Attribution, error) {
	if a.err != nil {
		return attribution.Attribution{}, a.err
	}
	if a.outcome != attribution.OutcomeAttributed {
		return attribution.Attribution{Outcome: a.outcome}, nil
	}
	return attribution.Attribution{
		Outcome:          attribution.OutcomeAttributed,
		Method:           attribution.MethodClickID,
		Click:            a.click,
		TimeToConversion: c.OccurredAt.Sub(a.click.OccurredAt),
	}, nil
}

func newHarness() (*conversion.Service, *fakeMapper, *fakeStore, *fakeEvents, *fakeDeliveries) {
	mapper := newFakeMapper()
	events := &fakeEvents{}
	deliveries := &fakeDeliveries{}
	store := newFakeStore(events, deliveries)
	attr := &fakeAttribution{
		outcome: attribution.OutcomeAttributed,
		click:   attribution.Click{ClickID: "click-1", OrganizationID: orgA, FlowID: "flow-1", LandingID: "landing-1", OfferID: "offer-1", PwaID: "pwa-1", TrafficSourceID: "source-1", OccurredAt: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)},
	}
	svc := conversion.NewService(mapper, store, &fakeFX{rates: map[string]float64{"EUR": 1.1}}, attr, &fakeAttempts{})
	return svc, mapper, store, events, deliveries
}

func postback(networkID, clickID, rawStatus string) conversion.Postback {
	return conversion.Postback{
		OrganizationID: orgA,
		NetworkID:      networkID,
		ClickID:        clickID,
		RawStatus:      rawStatus,
		OccurredAt:     time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC),
	}
}

func TestUnmappedStatusIsIgnoredAsError(t *testing.T) {
	svc, _, store, events, _ := newHarness()

	result, err := svc.Record(ctx(), postback("net-1", "click-1", "unknown-status"))
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if result.Kind != conversion.ResultError {
		t.Fatalf("Kind = %q, want error", result.Kind)
	}
	if events.count() != 0 {
		t.Fatal("no event should be emitted for an unmapped status")
	}
	if len(store.records) != 1 || store.records[0].Kind != conversion.ResultError {
		t.Fatal("the attempt must still be logged (§45: log every postback)")
	}
}

func TestMissingClickIDIsError(t *testing.T) {
	svc, mapper, _, _, _ := newHarness()
	mapper.set("net-1", "sale", event.CpaAccept)

	p := postback("net-1", "", "sale")
	result, err := svc.Record(ctx(), p)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if result.Kind != conversion.ResultError {
		t.Fatalf("Kind = %q, want error", result.Kind)
	}
}

func TestFirstConversionIsRecordedAndEmitted(t *testing.T) {
	svc, mapper, _, events, _ := newHarness()
	mapper.set("net-1", "sale", event.CpaAccept)

	result, err := svc.Record(ctx(), postback("net-1", "click-1", "sale"))
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if result.Kind != conversion.ResultSuccess {
		t.Fatalf("Kind = %q, want success", result.Kind)
	}
	if events.count() != 1 {
		t.Fatalf("events emitted = %d, want 1", events.count())
	}
	if got := events.last().Type; got != event.CpaAccept {
		t.Fatalf("emitted event type = %q, want CPA_ACCEPT", got)
	}
	// The click's routing context rides onto the conversion, so revenue
	// can be reported by flow and by landing (chstore.DimLanding).
	if ev := events.last(); ev.FlowID != "flow-1" || ev.LandingID != "landing-1" || ev.OfferID != "offer-1" || ev.PwaID != "pwa-1" || ev.TrafficSourceID != "source-1" {
		t.Fatalf("conversion lost its click's flow/landing/offer/source: flow=%q landing=%q offer=%q source=%q", ev.FlowID, ev.LandingID, ev.OfferID, ev.TrafficSourceID)
	}
}

// TestDedupKeyIsClickStatusEventRef is the money-correctness test CLAUDE.md
// #3 calls out explicitly: dedup on click_id alone drops the deposit after
// the registration, and dedup on (click_id, status) drops every redeposit
// after the first. Neither may happen.
func TestDedupKeyIsClickStatusEventRef(t *testing.T) {
	svc, mapper, _, events, _ := newHarness()
	mapper.set("net-1", "reg", event.CpaHold)
	mapper.set("net-1", "sale", event.CpaAccept)
	mapper.set("net-1", "rebill", event.CpaRedep)

	steps := []struct {
		name       string
		rawStatus  string
		txnID      string
		wantKind   conversion.ResultKind
		wantEvents int
	}{
		{"HOLD recorded", "reg", "", conversion.ResultSuccess, 1},
		{"ACCEPT recorded (would be dropped by click_id-alone dedup)", "sale", "", conversion.ResultSuccess, 2},
		{"first REDEP recorded", "rebill", "txn-1", conversion.ResultSuccess, 3},
		{"second REDEP, different txn id, recorded (would be dropped by (click_id,status) dedup)", "rebill", "txn-2", conversion.ResultSuccess, 4},
		{"replay of the first REDEP's txn id is a true duplicate", "rebill", "txn-1", conversion.ResultDuplicate, 4},
	}

	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			p := postback("net-1", "click-1", step.rawStatus)
			p.NetworkTxnID = step.txnID
			result, err := svc.Record(ctx(), p)
			if err != nil {
				t.Fatalf("Record: %v", err)
			}
			if result.Kind != step.wantKind {
				t.Fatalf("Kind = %q, want %q", result.Kind, step.wantKind)
			}
			if events.count() != step.wantEvents {
				t.Fatalf("events emitted = %d, want %d", events.count(), step.wantEvents)
			}
		})
	}
}

// TestRedepWithNoTxnIDRecordsExactlyOne: §45's deliberate asymmetric
// failure mode — a missed redeposit (support ticket) is preferred over a
// double-counted one (bad invoice) when the network sends no txn id at all.
func TestRedepWithNoTxnIDRecordsExactlyOne(t *testing.T) {
	svc, mapper, _, events, _ := newHarness()
	mapper.set("net-1", "rebill", event.CpaRedep)

	first, err := svc.Record(ctx(), postback("net-1", "click-1", "rebill"))
	if err != nil || first.Kind != conversion.ResultSuccess {
		t.Fatalf("first redeposit: kind=%v err=%v, want success", first.Kind, err)
	}

	second, err := svc.Record(ctx(), postback("net-1", "click-1", "rebill"))
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if second.Kind != conversion.ResultDuplicate {
		t.Fatalf("second txn-id-less redeposit: kind = %q, want duplicate", second.Kind)
	}
	if events.count() != 1 {
		t.Fatalf("events emitted = %d, want exactly 1", events.count())
	}
}

// TestStatusProgressionRefusesReturnToHold is §45's STATUS PROGRESSION rule.
func TestStatusProgressionRefusesReturnToHold(t *testing.T) {
	svc, mapper, _, events, _ := newHarness()
	mapper.set("net-1", "reg", event.CpaHold)
	mapper.set("net-1", "sale", event.CpaAccept)

	if _, err := svc.Record(ctx(), postback("net-1", "click-1", "reg")); err != nil {
		t.Fatalf("recording HOLD: %v", err)
	}
	if _, err := svc.Record(ctx(), postback("net-1", "click-1", "sale")); err != nil {
		t.Fatalf("recording ACCEPT: %v", err)
	}

	// The network's nightly replay job re-sends the original HOLD.
	replay, err := svc.Record(ctx(), postback("net-1", "click-1", "reg"))
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if replay.Kind != conversion.ResultIgnored {
		t.Fatalf("Kind = %q, want ignored", replay.Kind)
	}
	if events.count() != 2 {
		t.Fatalf("events emitted = %d, want 2 (the replay must not emit a third)", events.count())
	}
}

// TestProgressionRuleIsIndependentOfAcceptDuplicates: the per-network
// override is about duplicate deliveries, not about time travel (§45).
func TestProgressionRuleIsIndependentOfAcceptDuplicates(t *testing.T) {
	svc, mapper, _, events, _ := newHarness()
	mapper.set("net-1", "reg", event.CpaHold)
	mapper.set("net-1", "sale", event.CpaAccept)

	hold := postback("net-1", "click-1", "reg")
	hold.AcceptDuplicates = true
	if _, err := svc.Record(ctx(), hold); err != nil {
		t.Fatalf("recording HOLD: %v", err)
	}

	accept := postback("net-1", "click-1", "sale")
	accept.AcceptDuplicates = true
	if _, err := svc.Record(ctx(), accept); err != nil {
		t.Fatalf("recording ACCEPT: %v", err)
	}

	replay := postback("net-1", "click-1", "reg")
	replay.AcceptDuplicates = true
	result, err := svc.Record(ctx(), replay)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if result.Kind != conversion.ResultIgnored {
		t.Fatalf("Kind = %q, want ignored even with acceptDuplicates set", result.Kind)
	}
	if events.count() != 2 {
		t.Fatalf("events emitted = %d, want 2", events.count())
	}
}

// TestProgressionAllowsEverythingElse: a reversal (ACCEPT -> DECLINE) is not
// refused by the progression rule — only a return to CPA_HOLD is. This does
// NOT extend to "and therefore gets its own dedup-distinct event": whether an
// undone reversal (DECLINE -> ACCEPT again) produces a second recorded event
// depends on the dedup key too, and CPA_ACCEPT's event_ref is fixed empty by
// §45, so that combination is a real spec gap this package does not attempt
// to resolve by inventing a key component (§45 explicitly forbids
// substituting a locally generated sequence number for exactly this reason).
func TestProgressionAllowsEverythingElse(t *testing.T) {
	svc, mapper, _, events, _ := newHarness()
	mapper.set("net-1", "sale", event.CpaAccept)
	mapper.set("net-1", "chargeback", event.CpaDecline)

	for _, raw := range []string{"sale", "chargeback"} {
		result, err := svc.Record(ctx(), postback("net-1", "click-1", raw))
		if err != nil {
			t.Fatalf("recording %q: %v", raw, err)
		}
		if result.Kind != conversion.ResultSuccess {
			t.Fatalf("recording %q: Kind = %q, want success", raw, result.Kind)
		}
	}
	if events.count() != 2 {
		t.Fatalf("events emitted = %d, want 2 (ACCEPT -> DECLINE both apply)", events.count())
	}
}

func TestUnattributedConversionIsStillRecorded(t *testing.T) {
	mapper := newFakeMapper()
	mapper.set("net-1", "sale", event.CpaAccept)
	events := &fakeEvents{}
	store := newFakeStore(events, nil)
	attr := &fakeAttribution{outcome: attribution.OutcomeUnknownClick}
	svc := conversion.NewService(mapper, store, &fakeFX{}, attr, &fakeAttempts{})

	result, err := svc.Record(ctx(), postback("net-1", "click-missing", "sale"))
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if result.Kind != conversion.ResultSuccess {
		t.Fatalf("Kind = %q, want success — an unattributed conversion is still a conversion", result.Kind)
	}
	if events.count() != 1 {
		t.Fatalf("events emitted = %d, want 1", events.count())
	}
	if got := events.last().AttributionOutcome; got != string(attribution.OutcomeUnknownClick) {
		t.Fatalf("AttributionOutcome = %q, want unknown_click", got)
	}
}

// A failed click lookup (ClickHouse down or not connected yet) is recorded
// as unknown_click with a retry, not answered 500: the worker attributes it
// once ClickHouse answers (audit 2026-10-06, T1).
func TestAttributionFailureRecordsUnknownClickWithRetry(t *testing.T) {
	mapper := newFakeMapper()
	mapper.set("net-1", "sale", event.CpaAccept)
	events := &fakeEvents{}
	store := newFakeStore(events, nil)
	attr := &fakeAttribution{err: errors.New("resolver unavailable")}
	svc := conversion.NewService(mapper, store, &fakeFX{}, attr, &fakeAttempts{})

	res, err := svc.Record(ctx(), postback("net-1", "click-1", "sale"))
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if res.Kind != conversion.ResultSuccess || res.Attribution.Outcome != attribution.OutcomeUnknownClick {
		t.Fatalf("got %s/%s, want success/unknown_click", res.Kind, res.Attribution.Outcome)
	}
	if got := store.records[len(store.records)-1]; !got.RetryAttribution {
		t.Error("entry must schedule an attribution retry")
	}
	if events.count() != 1 {
		t.Errorf("events = %d, want the conversion's event emitted at once", events.count())
	}
}

func TestUnknownClickSchedulesRetryAttributedDoesNot(t *testing.T) {
	for outcome, want := range map[attribution.Outcome]bool{
		attribution.OutcomeUnknownClick: true,
		attribution.OutcomeAttributed:   false,
	} {
		mapper := newFakeMapper()
		mapper.set("net-1", "sale", event.CpaAccept)
		store := newFakeStore(&fakeEvents{}, nil)
		attr := &fakeAttribution{outcome: outcome, click: attribution.Click{ClickID: "click-1", OrganizationID: orgA}}
		svc := conversion.NewService(mapper, store, &fakeFX{}, attr, &fakeAttempts{})
		if _, err := svc.Record(ctx(), postback("net-1", "click-1", "sale")); err != nil {
			t.Fatalf("%s: Record: %v", outcome, err)
		}
		if got := store.records[len(store.records)-1].RetryAttribution; got != want {
			t.Errorf("%s: RetryAttribution = %v, want %v", outcome, got, want)
		}
	}
}

func TestAttributionWithoutOrganizationIsAnError(t *testing.T) {
	mapper := newFakeMapper()
	mapper.set("net-1", "sale", event.CpaAccept)
	events := &fakeEvents{}
	attr := &fakeAttribution{err: attribution.ErrNoOrganization}
	svc := conversion.NewService(mapper, newFakeStore(events, nil), &fakeFX{}, attr, &fakeAttempts{})

	if _, err := svc.Record(ctx(), postback("net-1", "click-1", "sale")); !errors.Is(err, attribution.ErrNoOrganization) {
		t.Fatalf("err = %v, want ErrNoOrganization: an unscoped lookup is a bug, not a retry", err)
	}
	if events.count() != 0 {
		t.Fatal("no event on a programming error")
	}
}

func TestFXMissingRateStoresConversionWithoutUSDValue(t *testing.T) {
	mapper := newFakeMapper()
	mapper.set("net-1", "sale", event.CpaAccept)
	events := &fakeEvents{}
	store := newFakeStore(events, nil)
	attr := &fakeAttribution{outcome: attribution.OutcomeUnknownClick}
	svc := conversion.NewService(mapper, store, &fakeFX{}, attr, &fakeAttempts{}) // no rates configured

	p := postback("net-1", "click-1", "sale")
	revenue := 100.0
	p.Revenue = &revenue
	p.Currency = "TRY"

	result, err := svc.Record(ctx(), p)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if result.Kind != conversion.ResultSuccess {
		t.Fatalf("Kind = %q, want success — a missing FX rate must never drop the conversion", result.Kind)
	}
	ev := events.last()
	if ev.Revenue != revenue || ev.Currency != "TRY" {
		t.Fatalf("original revenue/currency not preserved: got %v %v", ev.Revenue, ev.Currency)
	}
	if ev.HasUSDValue {
		t.Fatal("HasUSDValue = true, want false — no rate was on file, never invent one")
	}
}

func TestEventRefFor(t *testing.T) {
	// eventRefFor is unexported; exercised indirectly through Record's
	// dedup behavior in TestDedupKeyIsClickStatusEventRef and
	// TestRedepWithNoTxnIDRecordsExactlyOne above. This test documents the
	// non-REDEP side explicitly: a txn id sent on a non-repeatable status
	// must never enter the dedup key, or every network retry with a fresh
	// txn id would become a new conversion.
	svc, mapper, _, events, _ := newHarness()
	mapper.set("net-1", "sale", event.CpaAccept)

	first := postback("net-1", "click-1", "sale")
	first.NetworkTxnID = "attempt-1"
	if _, err := svc.Record(ctx(), first); err != nil {
		t.Fatalf("Record: %v", err)
	}

	retry := postback("net-1", "click-1", "sale")
	retry.NetworkTxnID = "attempt-2" // network's retry, fresh txn id
	result, err := svc.Record(ctx(), retry)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if result.Kind != conversion.ResultDuplicate {
		t.Fatalf("Kind = %q, want duplicate — CPA_ACCEPT's event_ref must ignore the txn id", result.Kind)
	}
	if events.count() != 1 {
		t.Fatalf("events emitted = %d, want 1", events.count())
	}
}

func TestDeliveryEnqueuedOnSuccessWithMacroResolvedURL(t *testing.T) {
	svc, mapper, _, _, deliveries := newHarness()
	mapper.set("net-1", "sale", event.CpaAccept)

	p := postback("net-1", "click-1", "sale")
	p.PostbackURL = "https://net.example/pb?click_id={click_id}&status={status}&revenue={revenue}&currency={currency}"
	revenue := 42.5
	p.Revenue = &revenue
	p.Currency = "USD"

	if _, err := svc.Record(ctx(), p); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if deliveries.count() != 1 {
		t.Fatalf("deliveries enqueued = %d, want 1", deliveries.count())
	}
	req := deliveries.last()
	want := "https://net.example/pb?click_id=click-1&status=CPA_ACCEPT&revenue=42.5&currency=USD"
	if req.URL != want {
		t.Fatalf("delivery URL = %q, want %q", req.URL, want)
	}
	if req.NetworkID != "net-1" || req.OrganizationID != orgA || req.ClickID != "click-1" || req.Status != event.CpaAccept {
		t.Fatalf("delivery request fields wrong: %+v", req)
	}
}

func TestNoDeliveryWhenPostbackURLNotConfigured(t *testing.T) {
	svc, mapper, _, _, deliveries := newHarness()
	mapper.set("net-1", "sale", event.CpaAccept)

	p := postback("net-1", "click-1", "sale") // p.PostbackURL left empty
	if _, err := svc.Record(ctx(), p); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if deliveries.count() != 0 {
		t.Fatalf("deliveries enqueued = %d, want 0 when the network has no postback_url", deliveries.count())
	}
}

func TestNoDeliveryOnDuplicateIgnoredOrError(t *testing.T) {
	svc, mapper, _, _, deliveries := newHarness()
	mapper.set("net-1", "reg", event.CpaHold)
	mapper.set("net-1", "sale", event.CpaAccept)

	hold := postback("net-1", "click-1", "reg")
	hold.PostbackURL = "https://net.example/pb?click_id={click_id}"
	if _, err := svc.Record(ctx(), hold); err != nil {
		t.Fatalf("recording HOLD: %v", err)
	}
	if deliveries.count() != 1 {
		t.Fatalf("deliveries after HOLD = %d, want 1", deliveries.count())
	}

	// Duplicate of the same HOLD.
	if _, err := svc.Record(ctx(), hold); err != nil {
		t.Fatalf("recording duplicate HOLD: %v", err)
	}
	if deliveries.count() != 1 {
		t.Fatalf("deliveries after duplicate HOLD = %d, want still 1", deliveries.count())
	}

	accept := postback("net-1", "click-1", "sale")
	accept.PostbackURL = "https://net.example/pb?click_id={click_id}"
	if _, err := svc.Record(ctx(), accept); err != nil {
		t.Fatalf("recording ACCEPT: %v", err)
	}
	if deliveries.count() != 2 {
		t.Fatalf("deliveries after ACCEPT = %d, want 2", deliveries.count())
	}

	// Progression-ignored replay of HOLD.
	if _, err := svc.Record(ctx(), hold); err != nil {
		t.Fatalf("recording replayed HOLD: %v", err)
	}
	if deliveries.count() != 2 {
		t.Fatalf("deliveries after ignored HOLD replay = %d, want still 2", deliveries.count())
	}

	// Unmapped status -> error.
	unmapped := postback("net-1", "click-1", "bogus")
	unmapped.PostbackURL = "https://net.example/pb?click_id={click_id}"
	if _, err := svc.Record(ctx(), unmapped); err != nil {
		t.Fatalf("recording unmapped status: %v", err)
	}
	if deliveries.count() != 2 {
		t.Fatalf("deliveries after unmapped-status error = %d, want still 2", deliveries.count())
	}
}

// TestAttemptLoggedForEveryOutcome is §48's postback_events requirement at
// the Service level: every exit point of Record — success, duplicate,
// ignored, error — must report to the attempt audit log, not just the
// ones that also succeed durably.
func TestAttemptLoggedForEveryOutcome(t *testing.T) {
	mapper := newFakeMapper()
	mapper.set("net-1", "reg", event.CpaHold)
	mapper.set("net-1", "sale", event.CpaAccept)
	events := &fakeEvents{}
	store := newFakeStore(events, nil)
	attempts := &fakeAttempts{}
	attr := &fakeAttribution{outcome: attribution.OutcomeUnknownClick}
	svc := conversion.NewService(mapper, store, &fakeFX{}, attr, attempts)

	// success
	if _, err := svc.Record(ctx(), postback("net-1", "click-1", "reg")); err != nil {
		t.Fatalf("recording HOLD: %v", err)
	}
	// duplicate
	if _, err := svc.Record(ctx(), postback("net-1", "click-1", "reg")); err != nil {
		t.Fatalf("recording duplicate HOLD: %v", err)
	}
	// success (a second, different key) then ignored (replayed HOLD)
	if _, err := svc.Record(ctx(), postback("net-1", "click-1", "sale")); err != nil {
		t.Fatalf("recording ACCEPT: %v", err)
	}
	if _, err := svc.Record(ctx(), postback("net-1", "click-1", "reg")); err != nil {
		t.Fatalf("recording replayed HOLD: %v", err)
	}
	// error (unmapped)
	if _, err := svc.Record(ctx(), postback("net-1", "click-1", "bogus")); err != nil {
		t.Fatalf("recording unmapped status: %v", err)
	}

	if attempts.count() != 5 {
		t.Fatalf("attempts logged = %d, want 5 (one per Record call, regardless of outcome)", attempts.count())
	}
	kinds := map[conversion.ResultKind]int{}
	for _, r := range attempts.recs {
		kinds[r.Result]++
	}
	if kinds[conversion.ResultSuccess] != 2 || kinds[conversion.ResultDuplicate] != 1 ||
		kinds[conversion.ResultIgnored] != 1 || kinds[conversion.ResultError] != 1 {
		t.Fatalf("attempt outcomes = %+v, want 2 success, 1 duplicate, 1 ignored, 1 error", kinds)
	}
}

// racingStore reports no prior status on the fast path but refuses under its
// lock — the shape of a CPA_HOLD replay that lost a race to a CPA_ACCEPT
// (audit 2026-10-06, D3). The store's decision is the one that counts.
type racingStore struct{ *fakeStore }

func (racingStore) LastStatus(context.Context, string, string) (event.Type, bool, error) {
	return "", false, nil
}

func (s racingStore) Record(ctx context.Context, e conversion.Entry) (conversion.Recorded, error) {
	if e.Kind == conversion.ResultSuccess {
		return conversion.Recorded{ID: "id", Kind: conversion.ResultIgnored, Message: "refused under lock"}, nil
	}
	return s.fakeStore.Record(ctx, e)
}

func TestStoreRefusalUnderLockWins(t *testing.T) {
	events, deliveries := &fakeEvents{}, &fakeDeliveries{}
	mapper := newFakeMapper()
	mapper.set("net-1", "lead", event.CpaHold)
	svc := conversion.NewService(mapper, racingStore{newFakeStore(events, deliveries)}, &fakeFX{},
		&fakeAttribution{outcome: attribution.OutcomeUnknownClick}, &fakeAttempts{})

	p := postback("net-1", "click-1", "lead")
	p.PostbackURL = "https://net.example/pb?click_id={click_id}"
	result, err := svc.Record(ctx(), p)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if result.Kind != conversion.ResultIgnored || result.Message != "refused under lock" {
		t.Fatalf("result = %+v, want ignored with the store's message", result)
	}
	if events.count() != 0 || deliveries.count() != 0 {
		t.Fatalf("side effects of a refused postback: events=%d deliveries=%d, want 0/0", events.count(), deliveries.count())
	}
}
