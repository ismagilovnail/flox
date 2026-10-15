package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/analytics"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/cost"
	"github.com/ismagilovnail/flox/apps/internal/push"
)

// fakePush is a scripted analytics.PushSource; calls counts its queries.
type fakePush struct {
	rows  []push.ReportStatsRow
	last  push.ReportStatsQuery
	calls int
}

func (f *fakePush) ReportStats(_ context.Context, _ string, q push.ReportStatsQuery) ([]push.ReportStatsRow, error) {
	f.calls++
	f.last = q
	return f.rows, nil
}

func pushRow(day string, keys map[push.StatsGroup]string, sent, delivered, opened, unsub int) push.ReportStatsRow {
	return push.ReportStatsRow{Day: day, Keys: keys, StatsCounts: push.StatsCounts{Sent: sent, Delivered: delivered, Opened: opened, Unsubscribed: unsub}}
}

func byCampaign(rows []analytics.ReportRow) map[string]analytics.MetricValues {
	out := map[string]analytics.MetricValues{}
	for _, r := range rows {
		out[r.DimValues[chstore.DimCampaign]] = r.Metrics
	}
	return out
}

// Grouped by campaign: each campaign gets its push numbers, a campaign
// push did not reach a real 0 (CTR "—"), and a campaign whose subscribers
// got a broadcast but that had no clicks in the range still has a row.
func TestReportPushMetricsByCampaign(t *testing.T) {
	repo := &reportFakeRepo{clicks: []chstore.ClickAgg{
		{Key: dimKey(chstore.DimCampaign, "camp-a"), Clicks: 100},
		{Key: dimKey(chstore.DimCampaign, "camp-b"), Clicks: 50},
	}}
	src := &fakePush{rows: []push.ReportStatsRow{
		pushRow("", map[push.StatsGroup]string{push.StatsByCampaign: "camp-a"}, 40, 30, 6, 2),
		pushRow("", map[push.StatsGroup]string{push.StatsByCampaign: "camp-old"}, 10, 10, 1, 0),
	}}
	svc := analytics.NewService(repo, nil).WithPush(src)
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{
		Dims: []chstore.ReportDimension{chstore.DimCampaign},
		From: from, To: from.AddDate(0, 0, 6),
		Sort: analytics.SortSpec{Key: "push_sent", Dir: "desc"},
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if len(src.last.GroupBy) != 1 || src.last.GroupBy[0] != push.StatsByCampaign || src.last.ByDay ||
		!src.last.From.Equal(from) || !src.last.To.Equal(from.AddDate(0, 0, 7)) {
		t.Fatalf("push query = %+v, want by campaign over the report's instants", src.last)
	}
	m := byCampaign(rows)
	a, b, old := m["camp-a"], m["camp-b"], m["camp-old"]
	if a.PushSent == nil || *a.PushSent != 40 || *a.PushDelivered != 30 || *a.PushOpened != 6 || *a.PushUnsubscribed != 2 ||
		a.PushCTR == nil || *a.PushCTR != 20 {
		t.Fatalf("camp-a push = %+v, want 40/30/6/2 and CTR 20", a)
	}
	if b.PushSent == nil || *b.PushSent != 0 || b.PushCTR != nil {
		t.Fatalf("camp-b push sent = %v, ctr = %v; want a real 0 and no CTR", b.PushSent, b.PushCTR)
	}
	if len(rows) != 3 || old.PushSent == nil || *old.PushSent != 10 || old.Clicks != 0 || old.Conversions == nil || *old.Conversions != 0 {
		t.Fatalf("rows = %d, camp-old = %+v; want a row of its own with push and zero traffic", len(rows), old)
	}
	if rows[0].DimValues[chstore.DimCampaign] != "camp-a" || rows[2].DimValues[chstore.DimCampaign] != "camp-b" {
		t.Fatalf("sorted by push_sent desc: %v, %v, %v", rows[0].DimValues, rows[1].DimValues, rows[2].DimValues)
	}
}

// A campaign with spend but no traffic and no push still gets a real 0
// for push, like every other row of the report — not "—".
func TestReportPushMetricsZeroOnCostOnlyRow(t *testing.T) {
	repo := &reportFakeRepo{clicks: []chstore.ClickAgg{{Key: dimKey(chstore.DimCampaign, "camp-a"), Clicks: 10}}}
	costSrc := &fakeCost{byCampaign: []cost.CampaignSpend{{CampaignID: "camp-spend", AmountUSD: 50, AllConverted: true}}}
	src := &fakePush{rows: []push.ReportStatsRow{pushRow("", map[push.StatsGroup]string{push.StatsByCampaign: "camp-a"}, 4, 4, 1, 0)}}
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows, err := analytics.NewService(repo, costSrc).WithPush(src).Report(context.Background(), "org-1", analytics.ReportInput{
		Dims: []chstore.ReportDimension{chstore.DimCampaign}, From: from, To: from.AddDate(0, 0, 6),
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	spend := byCampaign(rows)["camp-spend"]
	if spend.Cost == nil || spend.PushSent == nil || *spend.PushSent != 0 || spend.PushCTR != nil {
		t.Fatalf("camp-spend cost = %v, push sent = %v, ctr = %v; want spend, a real 0 and no CTR", spend.Cost, spend.PushSent, spend.PushCTR)
	}
}

// Days and a second push dimension line up with the report's own rows.
func TestReportPushMetricsByDayAndCountry(t *testing.T) {
	d1 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	repo := &reportFakeRepo{clicks: []chstore.ClickAgg{
		{Key: chstore.ReportKey{Day: &d1, DimValues: map[chstore.ReportDimension]string{chstore.DimCountry: "DE"}}, Clicks: 9},
	}}
	src := &fakePush{rows: []push.ReportStatsRow{
		pushRow("2026-10-01", map[push.StatsGroup]string{push.StatsByCountry: "DE"}, 5, 4, 1, 0),
		pushRow("2026-10-02", map[push.StatsGroup]string{push.StatsByCountry: "DE"}, 7, 7, 0, 1),
	}}
	svc := analytics.NewService(repo, nil).WithPush(src)
	rows, err := svc.Report(context.Background(), "org-1", analytics.ReportInput{
		Dims: []chstore.ReportDimension{chstore.DimCountry, analytics.DimDay},
		From: d1, To: d1.AddDate(0, 0, 1), Timezone: "Europe/Berlin",
		Filters: []chstore.ReportFilter{{Dimension: chstore.DimCampaign, Value: "camp-a"}},
		Sort:    analytics.SortSpec{Key: "day", Dir: "asc"},
	})
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if !src.last.ByDay || src.last.Timezone != "Europe/Berlin" || len(src.last.Filters) != 1 ||
		src.last.Filters[0] != (push.ReportStatsFilter{Group: push.StatsByCampaign, Value: "camp-a"}) {
		t.Fatalf("push query = %+v, want by day in Berlin, filtered by the campaign", src.last)
	}
	if len(rows) != 2 || rows[0].Metrics.Clicks != 9 || *rows[0].Metrics.PushSent != 5 ||
		rows[1].Day == nil || rows[1].Day.Format(time.DateOnly) != "2026-10-02" || *rows[1].Metrics.PushUnsubscribed != 1 {
		t.Fatalf("rows = %+v, want day 1 merged with its clicks and day 2 a push-only row", rows)
	}
}

// A dimension or filter push statistics do not have leaves every push
// metric nil ("—") and does not query push at all.
func TestReportPushMetricsNullWhenUnanswerable(t *testing.T) {
	for name, in := range map[string]analytics.ReportInput{
		"device dimension": {Dims: []chstore.ReportDimension{chstore.DimCampaign, chstore.DimDevice}},
		"flow filter":      {Dims: []chstore.ReportDimension{chstore.DimCampaign}, Filters: []chstore.ReportFilter{{Dimension: chstore.DimFlow, Value: "f1"}}},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &reportFakeRepo{clicks: []chstore.ClickAgg{{Key: chstore.ReportKey{DimValues: map[chstore.ReportDimension]string{chstore.DimCampaign: "camp-a", chstore.DimDevice: "mobile"}}, Clicks: 3}}}
			src := &fakePush{rows: []push.ReportStatsRow{pushRow("", map[push.StatsGroup]string{push.StatsByCampaign: "camp-a"}, 1, 1, 1, 0)}}
			in.From, in.To = time.Now().AddDate(0, 0, -1), time.Now()
			rows, err := analytics.NewService(repo, nil).WithPush(src).Report(context.Background(), "org-1", in)
			if err != nil {
				t.Fatalf("Report: %v", err)
			}
			if src.calls != 0 {
				t.Fatalf("push queried %d times, want none", src.calls)
			}
			for _, r := range rows {
				if r.Metrics.PushSent != nil || r.Metrics.PushCTR != nil {
					t.Fatalf("row %v push = %v, want nil", r.DimValues, r.Metrics.PushSent)
				}
			}
		})
	}
}
