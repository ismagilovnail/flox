package attribution_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/event"
)

func ctx() context.Context { return context.Background() }

const (
	orgA = "org-a"
	orgB = "org-b"
)

var clickAt = time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)

func click(org, clickID, externalID string) attribution.Click {
	return attribution.Click{
		ClickID:         clickID,
		OrganizationID:  org,
		CampaignID:      "camp-1",
		StreamSetID:     "set-1",
		FlowID:          "flow-1",
		ExternalClickID: externalID,
		OccurredAt:      clickAt,
		Country:         "US",
		Device:          "mobile",
	}
}

func conversion(org, clickID, externalID string) attribution.Conversion {
	return attribution.Conversion{
		OrganizationID:  org,
		ClickID:         clickID,
		ExternalClickID: externalID,
		Status:          event.CpaHold,
		OccurredAt:      clickAt.Add(2 * time.Hour),
	}
}

func serviceWith(clicks ...attribution.Click) *attribution.Service {
	r := attribution.NewMemoryResolver()
	for _, c := range clicks {
		r.Record(c)
	}
	return attribution.NewService(r)
}

func TestAttributeByClickID(t *testing.T) {
	s := serviceWith(click(orgA, "click-1", "fbclid-1"))

	got, err := s.AttributeConversion(ctx(), conversion(orgA, "click-1", ""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Outcome != attribution.OutcomeAttributed {
		t.Fatalf("outcome = %q, want attributed (%s)", got.Outcome, got.Reason)
	}
	if got.Method != attribution.MethodClickID {
		t.Errorf("method = %q, want click_id", got.Method)
	}
	if got.Click.CampaignID != "camp-1" || got.Click.FlowID != "flow-1" {
		t.Errorf("click dimensions not carried forward: %+v", got.Click)
	}
	if got.TimeToConversion != 2*time.Hour {
		t.Errorf("time to conversion = %v, want 2h", got.TimeToConversion)
	}
}

func TestAttributeByExternalClickID_WhenUnique(t *testing.T) {
	s := serviceWith(click(orgA, "click-1", "fbclid-1"))

	got, err := s.AttributeConversion(ctx(), conversion(orgA, "", "fbclid-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Outcome != attribution.OutcomeAttributed || got.Method != attribution.MethodExternalClickID {
		t.Fatalf("got outcome %q method %q, want attributed via external_click_id (%s)",
			got.Outcome, got.Method, got.Reason)
	}
	if got.Click.ClickID != "click-1" {
		t.Errorf("resolved click = %q, want click-1", got.Click.ClickID)
	}
}

func TestAmbiguousExternalClickID_IsRefusedNotGuessed(t *testing.T) {
	// The same fbclid on two clicks is ordinary: a redirect chain, a prefetch,
	// or a genuine second visit all produce it. Picking "the most recent"
	// would look sensible and would credit the wrong click a fraction of the
	// time, with nothing in any report indicating it happened (§44).
	later := click(orgA, "click-2", "fbclid-shared")
	later.OccurredAt = clickAt.Add(time.Minute)
	s := serviceWith(click(orgA, "click-1", "fbclid-shared"), later)

	got, err := s.AttributeConversion(ctx(), conversion(orgA, "", "fbclid-shared"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Outcome != attribution.OutcomeAmbiguous {
		t.Fatalf("outcome = %q, want ambiguous (%s)", got.Outcome, got.Reason)
	}
	if got.Click.ClickID != "" {
		t.Errorf("an ambiguous result must carry no click, got %q", got.Click.ClickID)
	}
	if got.Method != attribution.MethodNone {
		t.Errorf("method = %q, want none", got.Method)
	}
}

func TestUnknownClickID_DoesNotFallBackToExternalID(t *testing.T) {
	// The network echoed back an identifier we minted, and we cannot find it.
	// That claim is suspect; re-matching it on the weaker, caller-supplied
	// external id would hide exactly the case worth investigating.
	s := serviceWith(click(orgA, "click-1", "fbclid-1"))

	got, err := s.AttributeConversion(ctx(), conversion(orgA, "click-does-not-exist", "fbclid-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Outcome != attribution.OutcomeUnknownClick {
		t.Fatalf("outcome = %q, want unknown_click (%s)", got.Outcome, got.Reason)
	}
	if got.Click.ClickID != "" {
		t.Fatalf("must not have silently attributed to %q via the external id", got.Click.ClickID)
	}
}

func TestNoIdentifiers_IsItsOwnOutcome(t *testing.T) {
	// Usually a postback template misconfigured in the network's dashboard.
	// Distinct from "we looked and found nothing", because the fix is
	// different and an operator needs to tell them apart.
	s := serviceWith(click(orgA, "click-1", "fbclid-1"))

	got, err := s.AttributeConversion(ctx(), conversion(orgA, "", ""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Outcome != attribution.OutcomeNoIdentifier {
		t.Fatalf("outcome = %q, want no_identifier (%s)", got.Outcome, got.Reason)
	}
}

func TestWhitespaceOnlyIdentifiersCountAsAbsent(t *testing.T) {
	// Postback templates are assembled by hand in someone else's dashboard;
	// "click_id= " arrives. Treating that as a real id would produce a
	// confident unknown_click instead of the truthful no_identifier.
	s := serviceWith(click(orgA, "click-1", "fbclid-1"))

	got, err := s.AttributeConversion(ctx(), conversion(orgA, "   ", "\t"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Outcome != attribution.OutcomeNoIdentifier {
		t.Fatalf("outcome = %q, want no_identifier (%s)", got.Outcome, got.Reason)
	}
}

// --- tenant isolation (DoD for data phases, CLAUDE.md #5) ---------------

func TestTenantIsolation_ClickIDOfAnotherOrgIsNotAttributed(t *testing.T) {
	s := serviceWith(click(orgB, "click-1", "fbclid-1"))

	got, err := s.AttributeConversion(ctx(), conversion(orgA, "click-1", ""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Outcome.Attributed() {
		t.Fatalf("org A attributed a conversion to org B's click: %+v", got.Click)
	}
	if got.Outcome != attribution.OutcomeUnknownClick {
		t.Fatalf("outcome = %q, want unknown_click — another tenant's click must be "+
			"indistinguishable from a nonexistent one", got.Outcome)
	}
	if got.Click.OrganizationID != "" {
		t.Fatalf("leaked another organization's click: %+v", got.Click)
	}
}

func TestTenantIsolation_ExternalClickIDOfAnotherOrgIsNotAttributed(t *testing.T) {
	s := serviceWith(click(orgB, "click-1", "fbclid-shared"))

	got, err := s.AttributeConversion(ctx(), conversion(orgA, "", "fbclid-shared"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Outcome.Attributed() || got.Click.OrganizationID != "" {
		t.Fatalf("org A attributed a conversion to org B's click: %+v", got)
	}
}

func TestTenantIsolation_SameExternalIDInTwoOrgsIsNotAmbiguous(t *testing.T) {
	// Each organization sees exactly one click, so each attributes cleanly.
	// If the resolver ever stopped scoping by organization, both would start
	// reporting "ambiguous" — a silent, cross-tenant failure that would look
	// like a data-quality problem rather than an isolation breach.
	s := serviceWith(
		click(orgA, "click-a", "fbclid-shared"),
		click(orgB, "click-b", "fbclid-shared"),
	)

	for org, wantClick := range map[string]string{orgA: "click-a", orgB: "click-b"} {
		got, err := s.AttributeConversion(ctx(), conversion(org, "", "fbclid-shared"))
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", org, err)
		}
		if got.Outcome != attribution.OutcomeAttributed || got.Click.ClickID != wantClick {
			t.Fatalf("%s: got outcome %q click %q, want attributed to %q (%s)",
				org, got.Outcome, got.Click.ClickID, wantClick, got.Reason)
		}
	}
}

func TestMissingOrganizationIsRefused(t *testing.T) {
	// The only alternative to refusing is searching every organization, which
	// is a cross-tenant leak that would behave like a working feature.
	s := serviceWith(click(orgA, "click-1", ""))

	_, err := s.AttributeConversion(ctx(), conversion("", "click-1", ""))
	if !errors.Is(err, attribution.ErrNoOrganization) {
		t.Fatalf("want ErrNoOrganization, got %v", err)
	}
}

// --- resolver failures are not "unattributed" ---------------------------

type failingResolver struct{ err error }

func (f failingResolver) ByClickID(context.Context, string, string) (attribution.Click, error) {
	return attribution.Click{}, f.err
}

func (f failingResolver) ByExternalClickID(context.Context, string, string) ([]attribution.Click, error) {
	return nil, f.err
}

func TestResolverFailureIsAnErrorNotAnUnattributedConversion(t *testing.T) {
	// Recording a database blip as "unattributed" would permanently write off
	// a real conversion; the caller has to be able to retry instead.
	boom := errors.New("clickhouse unavailable")
	s := attribution.NewService(failingResolver{err: boom})

	for name, conv := range map[string]attribution.Conversion{
		"by click id":    conversion(orgA, "click-1", ""),
		"by external id": conversion(orgA, "", "fbclid-1"),
	} {
		got, err := s.AttributeConversion(ctx(), conv)
		if !errors.Is(err, boom) {
			t.Fatalf("%s: want the resolver error to surface, got %v", name, err)
		}
		if got.Outcome != "" {
			t.Fatalf("%s: want no outcome alongside an error, got %q", name, got.Outcome)
		}
	}
}

// --- diagnostics --------------------------------------------------------

func TestNegativeTimeToConversionIsReportedNotSuppressed(t *testing.T) {
	// A conversion timestamped before its click means clock skew against the
	// network, or a replayed postback. Clamping it to zero would erase the
	// one signal that says so.
	s := serviceWith(click(orgA, "click-1", ""))

	conv := conversion(orgA, "click-1", "")
	conv.OccurredAt = clickAt.Add(-30 * time.Second)

	got, err := s.AttributeConversion(ctx(), conv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.TimeToConversion != -30*time.Second {
		t.Fatalf("time to conversion = %v, want -30s preserved", got.TimeToConversion)
	}
	if !got.Outcome.Attributed() {
		t.Errorf("skew is a diagnostic, not grounds for refusing a matched click")
	}
}

func TestEveryOutcomeCarriesAReason(t *testing.T) {
	// The postback log is where a disputed payout gets re-argued, so no
	// decision may be recorded without a stated why (§72's spirit).
	s := serviceWith(
		click(orgA, "click-1", "fbclid-1"),
		click(orgA, "click-2", "fbclid-shared"),
		click(orgA, "click-3", "fbclid-shared"),
	)

	for name, conv := range map[string]attribution.Conversion{
		"attributed":    conversion(orgA, "click-1", ""),
		"unknown":       conversion(orgA, "nope", ""),
		"ambiguous":     conversion(orgA, "", "fbclid-shared"),
		"no identifier": conversion(orgA, "", ""),
	} {
		got, err := s.AttributeConversion(ctx(), conv)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}
		if got.Reason == "" {
			t.Errorf("%s: outcome %q recorded with no reason", name, got.Outcome)
		}
	}
}

func TestMemoryResolverDoesNotIndexEmptyExternalIDs(t *testing.T) {
	// An absent network id is absent, not a value that every subs-less click
	// shares (§42). Indexing "" would make every Facebook click that arrived
	// without an fbclid mutually ambiguous.
	r := attribution.NewMemoryResolver()
	r.Record(click(orgA, "click-1", ""))
	r.Record(click(orgA, "click-2", ""))

	found, err := r.ByExternalClickID(ctx(), orgA, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("empty external id matched %d clicks, want 0", len(found))
	}
}
