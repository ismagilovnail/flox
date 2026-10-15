package push_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/push"
)

// A scheduled broadcast queues nothing until it is due; SendDue then fixes
// the audience as it is at that moment, with each delivery's PWA, campaign
// and country and the per-group sent counts. The broadcasts here are due
// an hour from now and SendDue is called with a later "now", so the dev
// stack's own worker, which sends with the real time, leaves them alone.
func TestScheduledBroadcastGoesOutWhenDue(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.subscribe(t, f.org, f.pwa, f.camp, "US")
	f.subscribe(t, f.org, f.pwa, f.camp, "US")

	sendAt := time.Now().Add(time.Hour).Truncate(time.Second)
	svc := push.NewService(f.repo, true)
	b, err := svc.CreateBroadcast(ctx, f.org, "", push.CreateBroadcastInput{
		Name: "Later", Title: "Hi", Target: push.TargetDestination, SendAt: &sendAt,
	})
	if err != nil {
		t.Fatalf("CreateBroadcast: %v", err)
	}
	if b.Status != push.BroadcastScheduled || b.SendAt == nil || !b.SendAt.Equal(sendAt) || b.SentAt != nil || b.Total != 0 {
		t.Fatalf("scheduled broadcast = %+v", b)
	}
	if n := f.count(t, `SELECT count(*) FROM `+f.deliveries+` WHERE broadcast_id = $1`, b.ID); n != 0 {
		t.Fatalf("%d deliveries queued before the send time", n)
	}

	if _, err := f.repo.ScopedTo(f.org).SendDue(ctx, time.Now(), 50); err != nil {
		t.Fatalf("SendDue (not yet due): %v", err)
	}
	if got, _ := svc.GetBroadcast(ctx, f.org, b.ID); got.Status != push.BroadcastScheduled {
		t.Fatalf("sent before its time: %+v", got)
	}

	// A subscriber who joins before the send time gets it too.
	f.subscribe(t, f.org, f.pwa, f.camp, "DE")
	if _, err := f.repo.ScopedTo(f.org).SendDue(ctx, sendAt.Add(time.Minute), 50); err != nil {
		t.Fatalf("SendDue: %v", err)
	}
	got, _ := svc.GetBroadcast(ctx, f.org, b.ID)
	if got.Status != push.BroadcastSent || got.SentAt == nil || got.Total != 3 {
		t.Fatalf("after SendDue = %+v, want sent to 3", got)
	}
	if n := f.count(t, `SELECT count(*) FROM `+f.deliveries+` WHERE broadcast_id = $1 AND pwa_id = $2 AND campaign_id = $3 AND country = 'US'`, b.ID, f.pwa, f.camp); n != 2 {
		t.Fatalf("US deliveries with their dimensions = %d, want 2", n)
	}
	if n := f.count(t, `SELECT sent FROM push_broadcast_stats WHERE broadcast_id = $1 AND country = 'DE'`, b.ID); n != 1 {
		t.Fatalf("DE sent = %d, want 1", n)
	}

	// Sent once: due again later, nothing more is queued.
	if _, err := f.repo.ScopedTo(f.org).SendDue(ctx, sendAt.Add(time.Hour), 50); err != nil {
		t.Fatalf("SendDue again: %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM `+f.deliveries+` WHERE broadcast_id = $1`, b.ID); n != 3 {
		t.Fatalf("deliveries after a second SendDue = %d, want 3", n)
	}
}

func TestCancelScheduledBroadcast(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.subscribe(t, f.org, f.pwa, f.camp, "US")
	otherOrg, _, _ := f.seedOrg(t)
	svc := push.NewService(f.repo, true)

	sendAt := time.Now().Add(time.Hour)
	b, err := svc.CreateBroadcast(ctx, f.org, "", push.CreateBroadcastInput{Name: "Later", Title: "Hi", Target: push.TargetDestination, SendAt: &sendAt})
	if err != nil {
		t.Fatal(err)
	}
	var apiErr *apierror.Error
	if _, err := svc.CancelBroadcast(ctx, otherOrg, b.ID); !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Fatalf("other organization cancels: %v, want 404", err)
	}
	canceled, err := svc.CancelBroadcast(ctx, f.org, b.ID)
	if err != nil || canceled.Status != push.BroadcastCanceled {
		t.Fatalf("CancelBroadcast = %+v, %v", canceled, err)
	}
	if _, err := svc.CancelBroadcast(ctx, f.org, b.ID); !errors.As(err, &apiErr) || apiErr.Status != 409 {
		t.Fatalf("cancel twice: %v, want 409", err)
	}
	if _, err := f.repo.ScopedTo(f.org).SendDue(ctx, sendAt.Add(time.Minute), 50); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetBroadcast(ctx, f.org, b.ID); got.Status != push.BroadcastCanceled || got.Total != 0 {
		t.Fatalf("canceled broadcast went out: %+v", got)
	}

	now, err := svc.CreateBroadcast(ctx, f.org, "", push.CreateBroadcastInput{Name: "Now", Title: "Hi", Target: push.TargetDestination})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelBroadcast(ctx, f.org, now.ID); !errors.As(err, &apiErr) || apiErr.Status != 409 {
		t.Fatalf("cancel a sent broadcast: %v, want 409", err)
	}
}

func TestScheduleValidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	svc := push.NewService(f.repo, true)
	valid := push.CreateBroadcastInput{Name: "Later", Title: "Hi", Target: push.TargetDestination}

	var verr *apierror.Error
	for name, at := range map[string]time.Time{
		"past":         time.Now().Add(-time.Hour),
		"seconds away": time.Now().Add(10 * time.Second),
		"too far":      time.Now().Add(31 * 24 * time.Hour),
	} {
		in := valid
		in.SendAt = &at
		if _, err := svc.CreateBroadcast(ctx, f.org, "", in); !errors.As(err, &verr) || verr.Fields["sendAt"] == "" {
			t.Errorf("%s: %v, want a sendAt error", name, err)
		}
	}
	// No subscriber yet is fine for a scheduled broadcast: the audience is
	// counted when it goes out.
	at := time.Now().Add(2 * time.Hour)
	in := valid
	in.SendAt = &at
	if b, err := svc.CreateBroadcast(ctx, f.org, "", in); err != nil || b.Status != push.BroadcastScheduled {
		t.Fatalf("scheduled with no subscribers yet: %+v, %v", b, err)
	}
	if _, err := push.NewService(f.repo, false).CreateBroadcast(ctx, f.org, "", in); !errors.As(err, &verr) || verr.Status != 503 {
		t.Fatalf("unconfigured: %v, want 503", err)
	}
}

// Push metrics come from push_broadcast_stats: per group, sent at queue
// time and opened once per delivery; only this organization's sent
// broadcasts, by their send day in the given timezone.
func TestStatsByGroup(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, clickUS := f.subscribe(t, f.org, f.pwa, f.camp, "US")
	f.subscribe(t, f.org, f.pwa, f.camp, "US")
	f.subscribe(t, f.org, f.pwa, idgen.New(), "DE")
	otherOrg, otherPwa, otherCamp := f.seedOrg(t)
	f.subscribe(t, otherOrg, otherPwa, otherCamp, "US")
	svc := push.NewService(f.repo, true)

	b, err := svc.CreateBroadcast(ctx, f.org, "", push.CreateBroadcastInput{Name: "Promo", Title: "Hi", Target: push.TargetDestination})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateBroadcast(ctx, otherOrg, "", push.CreateBroadcastInput{Name: "Theirs", Title: "Hi", Target: push.TargetDestination}); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if _, err := svc.CreateBroadcast(ctx, f.org, "", push.CreateBroadcastInput{Name: "Scheduled", Title: "Hi", Target: push.TargetDestination, SendAt: &later}); err != nil {
		t.Fatal(err)
	}
	var deliveryUS string
	if err := f.pool.QueryRow(ctx, `SELECT d.id FROM `+f.deliveries+` d JOIN push_subscriptions s ON s.id = d.subscription_id
		WHERE d.broadcast_id = $1 AND s.click_id = $2`, b.ID, clickUS).Scan(&deliveryUS); err != nil {
		t.Fatal(err)
	}
	for range 2 { // counted once
		if _, _, err := f.repo.RecordOpen(ctx, f.org, clickUS, deliveryUS); err != nil {
			t.Fatal(err)
		}
	}

	today := time.Now().UTC()
	rows, total, err := svc.Stats(ctx, f.org, today, today, "UTC", push.StatsByCountry)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if total.Sent != 3 || total.Opened != 1 {
		t.Fatalf("totals = %+v, want sent 3 opened 1 (other organization and the scheduled one left out)", total)
	}
	byKey := map[string]push.StatsRow{}
	for _, r := range rows {
		byKey[r.Key] = r
	}
	if len(rows) != 2 || byKey["US"].Sent != 2 || byKey["US"].Opened != 1 || byKey["DE"].Sent != 1 || byKey["DE"].Opened != 0 {
		t.Fatalf("by country = %+v", rows)
	}

	rows, _, err = svc.Stats(ctx, f.org, today, today, "UTC", push.StatsByBroadcast)
	if err != nil || len(rows) != 1 || rows[0].Key != b.ID || rows[0].Label != "Promo" {
		t.Fatalf("by broadcast = %+v, %v", rows, err)
	}
	rows, _, err = svc.Stats(ctx, f.org, today, today, "UTC", push.StatsByDay)
	if err != nil || len(rows) != 1 || rows[0].Key != today.Format(time.DateOnly) {
		t.Fatalf("by day = %+v, %v", rows, err)
	}
	if rows, _, err := svc.Stats(ctx, f.org, today.AddDate(0, 0, -10), today.AddDate(0, 0, -1), "UTC", push.StatsByDay); err != nil || len(rows) != 0 {
		t.Fatalf("a range before the send = %+v, %v, want nothing", rows, err)
	}

	var verr *apierror.Error
	for name, call := range map[string]func() error{
		"groupBy": func() error { _, _, err := svc.Stats(ctx, f.org, today, today, "UTC", "browser"); return err },
		"timezone": func() error {
			_, _, err := svc.Stats(ctx, f.org, today, today, "Mars/Olympus", push.StatsByDay)
			return err
		},
		"to": func() error {
			_, _, err := svc.Stats(ctx, f.org, today, today.AddDate(0, 0, -1), "UTC", push.StatsByDay)
			return err
		},
	} {
		if err := call(); !errors.As(err, &verr) || verr.Fields[name] == "" {
			t.Errorf("%s: %v, want a %s field error", name, err, name)
		}
	}
}

func (f *fixture) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

// ReportStats (the analytics report builder's push columns) groups by any
// of campaign, country and PWA, cuts days in the report's zone, filters by
// equality and never reads another organization's rows.
func TestReportStats(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	svc := push.NewService(f.repo, true)
	// Broadcasts need subscribers; what they send today is outside the
	// March range below.
	f.subscribe(t, f.org, f.pwa, f.camp, "US")
	b, err := svc.CreateBroadcast(ctx, f.org, "", push.CreateBroadcastInput{Name: "Old", Title: "Hi", Target: push.TargetDestination})
	if err != nil {
		t.Fatal(err)
	}
	otherOrg, otherPwa, otherCamp := f.seedOrg(t)
	f.subscribe(t, otherOrg, otherPwa, otherCamp, "US")
	ob, err := svc.CreateBroadcast(ctx, otherOrg, "", push.CreateBroadcastInput{Name: "Theirs", Title: "Hi", Target: push.TargetDestination})
	if err != nil {
		t.Fatal(err)
	}
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, r := range []struct {
		org, broadcast, bucket, campaign, country string
		sent, delivered, opened                   int
	}{
		{f.org, b.ID, "2026-03-10T23:30:00Z", "camp-a", "DE", 5, 4, 1},
		{f.org, b.ID, "2026-03-11T00:15:00Z", "camp-a", "US", 3, 3, 0},
		{f.org, b.ID, "2026-03-11T10:00:00Z", "camp-b", "DE", 2, 2, 2},
		{otherOrg, ob.ID, "2026-03-11T10:00:00Z", "camp-a", "DE", 100, 100, 100},
	} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO push_broadcast_stats (organization_id, broadcast_id, bucket, pwa_id, campaign_id, country, sent, delivered, opened)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, r.org, r.broadcast, at(r.bucket), f.pwa, r.campaign, r.country, r.sent, r.delivered, r.opened); err != nil {
			t.Fatal(err)
		}
	}
	type key struct{ day, campaign string }
	sum := func(rows []push.ReportStatsRow) map[key]int {
		out := map[key]int{}
		for _, r := range rows {
			out[key{r.Day, r.Keys[push.StatsByCampaign]}] += r.Sent
		}
		return out
	}

	// Berlin is UTC+1 in March: 23:30Z on the 10th is the 11th there.
	rows, err := f.repo.ReportStats(ctx, f.org, push.ReportStatsQuery{
		From: at("2026-03-10T23:00:00Z"), To: at("2026-03-12T23:00:00Z"), Timezone: "Europe/Berlin",
		ByDay: true, GroupBy: []push.StatsGroup{push.StatsByCampaign},
	})
	if err != nil {
		t.Fatalf("ReportStats: %v", err)
	}
	if got := sum(rows); len(got) != 2 || got[key{"2026-03-11", "camp-a"}] != 8 || got[key{"2026-03-11", "camp-b"}] != 2 {
		t.Fatalf("by day in Berlin = %v, want camp-a 8 and camp-b 2 on the 11th", got)
	}
	rows, err = f.repo.ReportStats(ctx, f.org, push.ReportStatsQuery{
		From: at("2026-03-10T00:00:00Z"), To: at("2026-03-12T00:00:00Z"), ByDay: true, GroupBy: []push.StatsGroup{push.StatsByCampaign},
	})
	if got := sum(rows); err != nil || got[key{"2026-03-10", "camp-a"}] != 5 || got[key{"2026-03-11", "camp-a"}] != 3 {
		t.Fatalf("by day in UTC = %v, %v; want camp-a 5 on the 10th and 3 on the 11th", got, err)
	}

	rows, err = f.repo.ReportStats(ctx, f.org, push.ReportStatsQuery{
		From: at("2026-03-01T00:00:00Z"), To: at("2026-04-01T00:00:00Z"),
		GroupBy: []push.StatsGroup{push.StatsByCampaign, push.StatsByPwa},
		Filters: []push.ReportStatsFilter{{Group: push.StatsByCountry, Value: "DE"}},
	})
	if err != nil {
		t.Fatalf("ReportStats: %v", err)
	}
	if got := sum(rows); len(got) != 2 || got[key{"", "camp-a"}] != 5 || got[key{"", "camp-b"}] != 2 || rows[0].Keys[push.StatsByPwa] != f.pwa {
		t.Fatalf("DE by campaign and PWA = %+v, want camp-a 5 and camp-b 2 (the other organization's 100 left out)", rows)
	}
	for _, r := range rows {
		if r.Keys[push.StatsByCampaign] == "camp-b" && (r.Delivered != 2 || r.Opened != 2) {
			t.Fatalf("camp-b counters = %+v", r.StatsCounts)
		}
	}

	if _, err := f.repo.ReportStats(ctx, f.org, push.ReportStatsQuery{GroupBy: []push.StatsGroup{push.StatsByBroadcast}}); err == nil {
		t.Fatal("grouping by broadcast succeeded, want an error (not a report dimension)")
	}
}
