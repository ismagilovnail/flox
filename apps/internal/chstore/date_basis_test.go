package chstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// dateBasisFixture: a Monday 22:00 UTC click converting on Wednesday 10:00
// for $100, a Tuesday 03:00 click converting two hours later for $7, and a
// $5 conversion on Wednesday 11:00 whose click was never found.
type dateBasisFixture struct {
	orgID, campaignID string
	mon               time.Time
}

func seedDateBasisFixture(t *testing.T, ctx context.Context, store *chstore.EventStore) dateBasisFixture {
	t.Helper()
	fx := dateBasisFixture{orgID: idgen.New(), campaignID: idgen.New(), mon: time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)}
	late, early := idgen.New(), idgen.New()
	lateClick := fx.mon.Add(22 * time.Hour)
	earlyClick := fx.mon.Add(27 * time.Hour)
	lateConv := fx.mon.Add(58 * time.Hour)
	earlyConv := earlyClick.Add(2 * time.Hour)
	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: lateClick, OrganizationID: fx.orgID, CampaignID: fx.campaignID, ClickID: late, OS: "linux"},
		{Type: event.SourceClick, EventAt: earlyClick, OrganizationID: fx.orgID, CampaignID: fx.campaignID, ClickID: early, OS: "ios"},
		{Type: event.CpaAccept, EventAt: lateConv, OrganizationID: fx.orgID, CampaignID: fx.campaignID, ClickID: late,
			USDValue: 100, HasUSDValue: true, AttributionOutcome: "attributed", TimeToConversionMS: lateConv.Sub(lateClick).Milliseconds()},
		{Type: event.CpaAccept, EventAt: earlyConv, OrganizationID: fx.orgID, CampaignID: fx.campaignID, ClickID: early,
			USDValue: 7, HasUSDValue: true, AttributionOutcome: "attributed", TimeToConversionMS: earlyConv.Sub(earlyClick).Milliseconds()},
		{Type: event.CpaAccept, EventAt: fx.mon.Add(59 * time.Hour), OrganizationID: fx.orgID, ClickID: idgen.New(),
			USDValue: 5, HasUSDValue: true, AttributionOutcome: "unknown_click"},
	}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}
	return fx
}

type dayTotals struct {
	revenue map[string]float64
	dated   map[string]uint64
}

func conversionsByDay(t *testing.T, ctx context.Context, store *chstore.EventStore, q chstore.ReportQuery) dayTotals {
	t.Helper()
	q.ByDay = true
	aggs, err := store.ReportConversions(ctx, q)
	if err != nil {
		t.Fatalf("ReportConversions (%q): %v", q.DateBasis, err)
	}
	out := dayTotals{revenue: map[string]float64{}, dated: map[string]uint64{}}
	for _, a := range aggs {
		d := a.Key.Day.Format("2006-01-02")
		out.revenue[d] += a.RevenueUSD
		out.dated[d] += a.DatedByConversion
	}
	return out
}

func assertDays(t *testing.T, what string, got map[string]float64, want map[string]float64) {
	t.Helper()
	for day, v := range want {
		if got[day] != v {
			t.Fatalf("%s: %s = %v, want %v (all: %v)", what, day, got[day], v, got)
		}
	}
	for day, v := range got {
		if _, ok := want[day]; !ok && v != 0 {
			t.Fatalf("%s: unexpected %s = %v (all: %v)", what, day, v, got)
		}
	}
}

// Tech audit AN5: by click, revenue lands on the day of its click; a
// conversion without one keeps its own day and is counted as such.
func TestConversionsDatedByClick(t *testing.T) {
	store := chstore.NewRawEventStore(mustConn(t))
	ctx := context.Background()
	fx := seedDateBasisFixture(t, ctx, store)
	week := chstore.ReportQuery{OrganizationID: fx.orgID, From: fx.mon, To: fx.mon.Add(3 * 24 * time.Hour)}

	byConv := conversionsByDay(t, ctx, store, week)
	assertDays(t, "by conversion", byConv.revenue, map[string]float64{"2026-08-18": 7, "2026-08-19": 105})
	if byConv.dated["2026-08-19"] != 0 {
		t.Fatalf("by conversion, datedByConversion = %v, want 0", byConv.dated)
	}

	week.DateBasis = chstore.DateByClick
	byClick := conversionsByDay(t, ctx, store, week)
	assertDays(t, "by click", byClick.revenue, map[string]float64{"2026-08-17": 100, "2026-08-18": 7, "2026-08-19": 5})
	if byClick.dated["2026-08-19"] != 1 || byClick.dated["2026-08-17"] != 0 {
		t.Fatalf("by click, datedByConversion = %v, want 1 on 2026-08-19 only", byClick.dated)
	}

	// The range is the click's: Monday alone holds the Wednesday deposit.
	monday := chstore.ReportQuery{OrganizationID: fx.orgID, From: fx.mon, To: fx.mon.Add(24 * time.Hour), DateBasis: chstore.DateByClick}
	assertDays(t, "Monday by click", conversionsByDay(t, ctx, store, monday).revenue, map[string]float64{"2026-08-17": 100})
	monday.DateBasis = chstore.DateByConversion
	assertDays(t, "Monday by conversion", conversionsByDay(t, ctx, store, monday).revenue, map[string]float64{})

	// Days are cut in the report's zone: Monday 22:00 UTC is Tuesday in Moscow.
	msk, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	local := chstore.ReportQuery{OrganizationID: fx.orgID, Timezone: "Europe/Moscow", DateBasis: chstore.DateByClick,
		From: time.Date(2026, 8, 17, 0, 0, 0, 0, msk).UTC(), To: time.Date(2026, 8, 20, 0, 0, 0, 0, msk).UTC()}
	assertDays(t, "by click in Moscow", conversionsByDay(t, ctx, store, local).revenue, map[string]float64{"2026-08-18": 107, "2026-08-19": 5})
}

// Over a range holding every click and conversion, the two bases only move
// revenue between days, never change the total.
func TestDateBasesAgreeOnTotals(t *testing.T) {
	store := chstore.NewRawEventStore(mustConn(t))
	ctx := context.Background()
	fx := seedDateBasisFixture(t, ctx, store)
	for _, basis := range []chstore.DateBasis{chstore.DateByConversion, chstore.DateByClick} {
		aggs, err := store.ReportConversions(ctx, chstore.ReportQuery{OrganizationID: fx.orgID, From: fx.mon, To: fx.mon.Add(3 * 24 * time.Hour), DateBasis: basis})
		if err != nil {
			t.Fatal(err)
		}
		if len(aggs) != 1 || aggs[0].RevenueUSD != 112 || aggs[0].Conversions != 3 {
			t.Fatalf("%s: %+v, want one row of 3 conversions, $112", basis, aggs)
		}
	}
}

// The os/browser path joins clicks; by click it dates the same way.
func TestConversionsDatedByClickViaClickJoin(t *testing.T) {
	store := chstore.NewRawEventStore(mustConn(t))
	ctx := context.Background()
	fx := seedDateBasisFixture(t, ctx, store)
	aggs, err := store.ReportConversions(ctx, chstore.ReportQuery{
		OrganizationID: fx.orgID, Dims: []chstore.ReportDimension{chstore.DimOS}, ByDay: true,
		From: fx.mon, To: fx.mon.Add(24 * time.Hour), DateBasis: chstore.DateByClick,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(aggs) != 1 || aggs[0].Key.DimValues[chstore.DimOS] != "linux" || aggs[0].RevenueUSD != 100 || aggs[0].Key.Day.Format("2006-01-02") != "2026-08-17" {
		t.Fatalf("Monday by click and os: %+v, want linux $100 on 2026-08-17", aggs)
	}
}

func TestCampaignDailyRevenueDatedByClick(t *testing.T) {
	store := chstore.NewRawEventStore(mustConn(t))
	ctx := context.Background()
	fx := seedDateBasisFixture(t, ctx, store)
	wed := fx.mon.Add(48 * time.Hour)
	for basis, want := range map[chstore.DateBasis]map[string]float64{
		chstore.DateByConversion: {"2026-08-18": 7, "2026-08-19": 100},
		chstore.DateByClick:      {"2026-08-17": 100, "2026-08-18": 7},
	} {
		rows, err := store.DailyCampaignRevenue(ctx, fx.orgID, fx.campaignID, fx.mon, wed, basis)
		if err != nil {
			t.Fatalf("%s: %v", basis, err)
		}
		got := map[string]float64{}
		for _, r := range rows {
			got[r.Day.Format("2006-01-02")] += r.RevenueUSD
		}
		assertDays(t, "campaign daily revenue "+string(basis), got, want)
	}
}
