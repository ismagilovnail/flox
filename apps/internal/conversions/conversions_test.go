package conversions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/conversions"
	"github.com/ismagilovnail/flox/apps/internal/event"
)

// fakeRepo is an in-memory conversions.Repository, avoiding the need for a
// real ClickHouse connection to test Service's own logic (range/limit
// validation, funnel+conversion merge/sort, the no-events-found 404).
type fakeRepo struct {
	conversionsByOrg   map[string][]chstore.ConversionEvent
	funnelByClick      map[string][]chstore.FunnelEvent
	conversionsByClick map[string][]chstore.ConversionEvent
	// statusByOrg/offersByOrg are keyed by organization first: like the real
	// queries, a click_id in another org must never leak into a result.
	statusByOrg map[string][]chstore.StatusStep
	offersByOrg map[string]map[string]string
}

func (f *fakeRepo) ListConversions(_ context.Context, orgID string, from, to time.Time, clickID string, limit, offset int) ([]chstore.ConversionEvent, error) {
	rows := f.conversionsByOrg[orgID]
	// newest first, matching the real chstore query's ORDER BY
	sorted := make([]chstore.ConversionEvent, len(rows))
	copy(sorted, rows)
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].EventAt.After(sorted[i].EventAt) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	if offset >= len(sorted) {
		return nil, nil
	}
	end := offset + limit
	if end > len(sorted) {
		end = len(sorted)
	}
	return sorted[offset:end], nil
}

func (f *fakeRepo) CountConversions(_ context.Context, orgID string, from, to time.Time, clickID string) (int, error) {
	return len(f.conversionsByOrg[orgID]), nil
}

func (f *fakeRepo) ConversionsByClickID(_ context.Context, orgID, clickID string) ([]chstore.ConversionEvent, error) {
	return f.conversionsByClick[clickID], nil
}

func (f *fakeRepo) FunnelByClickID(_ context.Context, orgID, clickID string) ([]chstore.FunnelEvent, error) {
	return f.funnelByClick[clickID], nil
}

func (f *fakeRepo) StatusHistoryByClickIDs(_ context.Context, orgID string, clickIDs []string) ([]chstore.StatusStep, error) {
	want := map[string]bool{}
	for _, id := range clickIDs {
		want[id] = true
	}
	var out []chstore.StatusStep
	for _, st := range f.statusByOrg[orgID] {
		if want[st.ClickID] {
			out = append(out, st)
		}
	}
	return out, nil
}

func (f *fakeRepo) OffersByClickIDs(_ context.Context, orgID string, clickIDs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range clickIDs {
		if o, ok := f.offersByOrg[orgID][id]; ok {
			out[id] = o
		}
	}
	return out, nil
}

func TestListEnrichesRowsWithFlowOfferSubsAndStatusHistory(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	row := chstore.ConversionEvent{
		EventAt: t0.Add(48 * time.Hour), Type: event.CpaAccept, ClickID: "c1", CampaignID: "camp", NetworkID: "net",
		FlowID: "flow-1", Revenue: 12.5, Currency: "EUR", NetworkTxnID: "txn-9", UTMSource: "facebook",
	}
	row.Subs[0], row.Subs[9] = "creative-7", "last"
	repo := &fakeRepo{
		conversionsByOrg: map[string][]chstore.ConversionEvent{"org1": {row, {EventAt: t0, Type: event.CpaHold, ClickID: "c2"}}},
		statusByOrg: map[string][]chstore.StatusStep{"org1": {
			{ClickID: "c1", Type: event.CpaHold, EventAt: t0},
			{ClickID: "c1", Type: event.CpaAccept, EventAt: t0.Add(48 * time.Hour)},
			// another organization's step for the same click id must not show up
		}, "org2": {{ClickID: "c1", Type: event.CpaTrash, EventAt: t0}}},
		offersByOrg: map[string]map[string]string{"org1": {"c1": "offer-3"}, "org2": {"c1": "offer-evil"}},
	}
	svc := conversions.NewService(repo)

	res, err := svc.List(context.Background(), "org1", t0.AddDate(0, 0, -1), t0.AddDate(0, 0, 5), "", 0, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byClick := map[string]conversions.Conversion{}
	for _, c := range res.Conversions {
		byClick[c.ClickID] = c
	}
	c1 := byClick["c1"]
	if c1.FlowID != "flow-1" || c1.OfferID != "offer-3" || c1.NetworkTxnID != "txn-9" || c1.UTMSource != "facebook" ||
		c1.Sub1 != "creative-7" || c1.Sub10 != "last" || c1.Currency != "EUR" {
		t.Fatalf("c1 = %+v, want flow/offer/txn/utm/subs filled", c1)
	}
	if len(c1.History) != 2 || c1.History[0].Type != event.CpaHold || c1.History[1].Type != event.CpaAccept {
		t.Fatalf("c1.History = %+v, want HOLD then ACCEPT and nothing from org2", c1.History)
	}
	c2 := byClick["c2"]
	if c2.OfferID != "" || c2.History == nil || len(c2.History) != 0 {
		t.Fatalf("c2 = %+v, want no offer and an empty (non-nil) history", c2)
	}
}

func TestListValidatesDateRangeAndClampsLimit(t *testing.T) {
	repo := &fakeRepo{conversionsByOrg: map[string][]chstore.ConversionEvent{
		"org1": {{EventAt: time.Now(), Type: event.CpaHold, ClickID: "c1"}},
	}}
	svc := conversions.NewService(repo)
	ctx := context.Background()
	now := time.Now()

	if _, err := svc.List(ctx, "", now.AddDate(0, 0, -1), now, "", 0, 0); err == nil {
		t.Fatal("List with empty org id: want an error")
	}

	if _, err := svc.List(ctx, "org1", now, now.AddDate(0, 0, -1), "", 0, 0); err == nil {
		t.Fatal("List with to before from: want an error")
	}

	if _, err := svc.List(ctx, "org1", now.AddDate(0, 0, -200), now, "", 0, 0); err == nil {
		t.Fatal("List with a 200-day range: want a validation error (cap is 90 days)")
	}

	result, err := svc.List(ctx, "org1", now.AddDate(0, 0, -1), now, "", 0, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Total != 1 || len(result.Conversions) != 1 {
		t.Fatalf("List result = %+v, want 1 conversion", result)
	}
}

func TestTimelineMergesFunnelAndConversionEventsChronologically(t *testing.T) {
	base := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	repo := &fakeRepo{
		funnelByClick: map[string][]chstore.FunnelEvent{
			"click1": {
				{EventAt: base, Type: event.SourceClick, CampaignID: "camp1"},
				{EventAt: base.Add(1 * time.Hour), Type: event.LandView, CampaignID: "camp1"},
			},
		},
		conversionsByClick: map[string][]chstore.ConversionEvent{
			"click1": {
				{EventAt: base.Add(2 * time.Hour), Type: event.CpaHold, CampaignID: "camp1", NetworkID: "net1"},
			},
		},
	}
	svc := conversions.NewService(repo)

	timeline, err := svc.Timeline(context.Background(), "org1", "click1")
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if timeline.CampaignID != "camp1" || timeline.NetworkID != "net1" {
		t.Fatalf("Timeline campaign/network = %q/%q, want camp1/net1", timeline.CampaignID, timeline.NetworkID)
	}
	if len(timeline.Events) != 3 {
		t.Fatalf("Timeline.Events = %d, want 3 (2 funnel + 1 conversion)", len(timeline.Events))
	}
	wantTypes := []event.Type{event.SourceClick, event.LandView, event.CpaHold}
	for i, w := range wantTypes {
		if timeline.Events[i].Type != w {
			t.Fatalf("Timeline.Events[%d].Type = %q, want %q (chronological)", i, timeline.Events[i].Type, w)
		}
	}
	if !timeline.Events[2].IsConversion {
		t.Fatal("the CPA_HOLD entry must have IsConversion = true")
	}
	if timeline.Events[0].IsConversion {
		t.Fatal("the SOURCE_CLICK entry must have IsConversion = false")
	}
}

func TestTimelineNotFoundWhenClickIDHasNoEvents(t *testing.T) {
	repo := &fakeRepo{}
	svc := conversions.NewService(repo)

	_, err := svc.Timeline(context.Background(), "org1", "no-such-click")

	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "not_found" {
		t.Fatalf("Timeline for an unknown click id: err = %v, want a not_found apierror", err)
	}
}
