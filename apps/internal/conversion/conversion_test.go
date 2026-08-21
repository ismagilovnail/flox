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
// the exact same dedup-key and "one success per key" semantics so the
// service-level tests exercise the real contract, not a stub that always
// says yes.
type fakeStore struct {
	mu      sync.Mutex
	success map[string]event.Type // click id -> current status, success rows only
	dedup   map[string]bool       // dedup key -> seen (only for !AcceptDuplicates)
	records []conversion.Entry
}

func newFakeStore() *fakeStore {
	return &fakeStore{success: map[string]event.Type{}, dedup: map[string]bool{}}
}

func (s *fakeStore) LastStatus(_ context.Context, organizationID, clickID string) (event.Type, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, ok := s.success[organizationID+"|"+clickID]
	return status, ok, nil
}

func (s *fakeStore) Record(_ context.Context, e conversion.Entry) (string, conversion.ResultKind, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, e)

	if e.Kind != conversion.ResultSuccess {
		return "id", e.Kind, nil
	}

	key := e.OrganizationID + "|" + e.ClickID + "|" + string(e.Status) + "|" + e.EventRef
	if !e.AcceptDuplicates && s.dedup[key] {
		return "id", conversion.ResultDuplicate, nil
	}
	if !e.AcceptDuplicates {
		s.dedup[key] = true
	}
	s.success[e.OrganizationID+"|"+e.ClickID] = e.Status
	return "id", conversion.ResultSuccess, nil
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

// fakeEvents collects whatever the service emitted.
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

// fakeDeliveries collects whatever DeliveryRequests Service queued.
type fakeDeliveries struct {
	mu   sync.Mutex
	reqs []conversion.DeliveryRequest
}

func (d *fakeDeliveries) Enqueue(_ context.Context, req conversion.DeliveryRequest) {
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
	store := newFakeStore()
	events := &fakeEvents{}
	deliveries := &fakeDeliveries{}
	attr := &fakeAttribution{
		outcome: attribution.OutcomeAttributed,
		click:   attribution.Click{ClickID: "click-1", OrganizationID: orgA, OccurredAt: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)},
	}
	svc := conversion.NewService(mapper, store, &fakeFX{rates: map[string]float64{"EUR": 1.1}}, attr, events, deliveries)
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
	store := newFakeStore()
	events := &fakeEvents{}
	attr := &fakeAttribution{outcome: attribution.OutcomeUnknownClick}
	svc := conversion.NewService(mapper, store, &fakeFX{}, attr, events, &fakeDeliveries{})

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

func TestAttributionFailureSurfacesAsError(t *testing.T) {
	mapper := newFakeMapper()
	mapper.set("net-1", "sale", event.CpaAccept)
	store := newFakeStore()
	events := &fakeEvents{}
	boom := errors.New("resolver unavailable")
	attr := &fakeAttribution{err: boom}
	svc := conversion.NewService(mapper, store, &fakeFX{}, attr, events, &fakeDeliveries{})

	_, err := svc.Record(ctx(), postback("net-1", "click-1", "sale"))
	if err == nil {
		t.Fatal("expected an error when the attribution resolver fails — a blip must never be recorded as unattributed")
	}
	if events.count() != 0 {
		t.Fatal("no event should be emitted on infrastructure failure")
	}
}

func TestFXMissingRateStoresConversionWithoutUSDValue(t *testing.T) {
	mapper := newFakeMapper()
	mapper.set("net-1", "sale", event.CpaAccept)
	store := newFakeStore()
	events := &fakeEvents{}
	attr := &fakeAttribution{outcome: attribution.OutcomeUnknownClick}
	svc := conversion.NewService(mapper, store, &fakeFX{}, attr, events, &fakeDeliveries{}) // no rates configured

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
