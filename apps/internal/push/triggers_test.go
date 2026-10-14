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

// The triggers here wait an hour after subscription and SendTriggered is
// called with a later "now" through a repository scoped to the test's
// organization, so the dev stack's own worker (real time) leaves them
// alone and these calls leave everyone else's alone.
const hour = 60

func newTrigger(t *testing.T, svc *push.Service, org string, tr push.Trigger) push.Broadcast {
	t.Helper()
	b, err := svc.CreateBroadcast(context.Background(), org, "", push.CreateBroadcastInput{
		Name: "After an hour", Title: "Hi", Target: push.TargetDestination, Trigger: &tr,
	})
	if err != nil {
		t.Fatalf("CreateBroadcast(trigger): %v", err)
	}
	return b
}

func (f *fixture) sendTriggered(t *testing.T, repo *push.Repository, now time.Time) int {
	t.Helper()
	n, err := repo.SendTriggered(context.Background(), now, 1000)
	if err != nil {
		t.Fatalf("SendTriggered: %v", err)
	}
	return n
}

func (f *fixture) sentTo(t *testing.T, broadcastID string) map[string]bool {
	t.Helper()
	rows, err := f.pool.Query(context.Background(), `SELECT s.click_id FROM `+f.deliveries+` d
		JOIN push_subscriptions s ON s.id = d.subscription_id WHERE d.broadcast_id = $1`, broadcastID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		out[c] = true
	}
	return out
}

func TestTriggeredBroadcast(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	repo := f.repo.ScopedTo(f.org)
	svc := push.NewService(repo, true)

	// Subscribed before the trigger existed: never sent.
	_, oldClick := f.subscribe(t, f.org, f.pwa, f.camp, "US")
	if _, err := f.pool.Exec(ctx, `UPDATE push_subscriptions SET created_at = now() - interval '2 hours' WHERE click_id = $1`, oldClick); err != nil {
		t.Fatal(err)
	}
	b := newTrigger(t, svc, f.org, push.Trigger{DelayMinutes: hour})
	if b.Kind != push.KindTriggered || b.Status != push.BroadcastActive || b.Trigger == nil ||
		b.Trigger.DelayMinutes != hour || b.Trigger.Condition != push.ConditionNone || b.Total != 0 {
		t.Fatalf("trigger = %+v", b)
	}
	_, a := f.subscribe(t, f.org, f.pwa, f.camp, "US")
	_, c := f.subscribe(t, f.org, f.pwa, f.camp, "DE")
	now := time.Now()

	if n := f.sendTriggered(t, repo, now); n != 0 {
		t.Fatalf("sent %d before anyone was due", n)
	}
	if n := f.sendTriggered(t, repo, now.Add(61*time.Minute)); n != 2 {
		t.Fatalf("due send = %d, want 2", n)
	}
	if got := f.sentTo(t, b.ID); len(got) != 2 || !got[a] || !got[c] || got[oldClick] {
		t.Fatalf("sent to %v", got)
	}
	if n := f.sendTriggered(t, repo, now.Add(90*time.Minute)); n != 0 {
		t.Fatalf("second send = %d, want once per subscriber", n)
	}
	got, _ := svc.GetBroadcast(ctx, f.org, b.ID)
	if got.Total != 2 || got.Status != push.BroadcastActive {
		t.Fatalf("after sends = %+v", got)
	}

	// Due more than TriggerWindow ago (a long pause, an outage): skipped.
	_, late := f.subscribe(t, f.org, f.pwa, f.camp, "US")
	if n := f.sendTriggered(t, repo, now.Add(26*time.Hour)); n != 0 || f.sentTo(t, b.ID)[late] {
		t.Fatalf("sent %d to a subscriber due a day ago", n)
	}

	// Paused sends nothing; resumed, the subscribers due meanwhile go out.
	var apiErr *apierror.Error
	if _, err := svc.SetActive(ctx, f.org, b.ID, true); !errors.As(err, &apiErr) || apiErr.Status != 409 {
		t.Fatalf("resume an active trigger: %v, want 409", err)
	}
	otherOrg, _, _ := f.seedOrg(t)
	if _, err := svc.SetActive(ctx, otherOrg, b.ID, false); !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Fatalf("other organization pauses: %v, want 404", err)
	}
	if p, err := svc.SetActive(ctx, f.org, b.ID, false); err != nil || p.Status != push.BroadcastPaused {
		t.Fatalf("pause = %+v, %v", p, err)
	}
	// Push metrics count a trigger's deliveries on the day they went out:
	// make the first two yesterday's.
	if _, err := f.pool.Exec(ctx, `UPDATE push_broadcast_stats SET bucket = bucket - interval '1 day' WHERE broadcast_id = $1`, b.ID); err != nil {
		t.Fatal(err)
	}
	_, d := f.subscribe(t, f.org, f.pwa, f.camp, "US")
	if n := f.sendTriggered(t, repo, now.Add(62*time.Minute)); n != 0 {
		t.Fatalf("paused trigger sent %d", n)
	}
	if _, err := svc.SetActive(ctx, f.org, b.ID, true); err != nil {
		t.Fatal(err)
	}
	// late (due at now+60m, skipped at now+26h) is within its window now.
	if n := f.sendTriggered(t, repo, now.Add(62*time.Minute)); n != 2 || !f.sentTo(t, b.ID)[d] || !f.sentTo(t, b.ID)[late] {
		t.Fatalf("resumed send = %d, want the two due meanwhile", n)
	}

	today := time.Now().UTC()
	rows, total, err := svc.Stats(ctx, f.org, today.AddDate(0, 0, -1), today, "UTC", push.StatsByDay)
	if err != nil {
		t.Fatal(err)
	}
	if total.Sent != 4 || len(rows) != 2 || rows[0].Sent != 2 || rows[1].Sent != 2 || rows[1].Key != today.Format(time.DateOnly) {
		t.Fatalf("by day = %+v (total %+v), want 2 yesterday and 2 today", rows, total)
	}

	// A one-off broadcast is not a trigger.
	once, err := svc.CreateBroadcast(ctx, f.org, "", push.CreateBroadcastInput{Name: "Now", Title: "Hi", Target: push.TargetDestination})
	if err != nil {
		t.Fatal(err)
	}
	if once.Kind != push.KindOnce || once.Trigger != nil {
		t.Fatalf("one-off = %+v", once)
	}
	if _, err := svc.SetActive(ctx, f.org, once.ID, false); !errors.As(err, &apiErr) || apiErr.Status != 409 {
		t.Fatalf("pause a one-off: %v, want 409", err)
	}
}

// The condition looks at the subscriber's click's recorded conversions at
// send time: by click_id or attributed_click_id, result success only.
func TestTriggerCondition(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	repo := f.repo.ScopedTo(f.org)
	svc := push.NewService(repo, true)
	network := idgen.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, 'n')`, network, f.org); err != nil {
		t.Fatal(err)
	}
	postback := func(clickID, attributed, status, result string) {
		t.Helper()
		if _, err := f.pool.Exec(ctx, `INSERT INTO postbacks (id, organization_id, network_id, click_id, attributed_click_id, status, direction, result)
			VALUES ($1, $2, $3, $4, $5, $6, 'incoming', $7)`, idgen.New(), f.org, network, clickID, attributed, status, result); err != nil {
			t.Fatal(err)
		}
	}

	without := newTrigger(t, svc, f.org, push.Trigger{DelayMinutes: hour, Condition: push.ConditionWithout, Statuses: []string{"CPA_ACCEPT", "CPA_REDEP"}})
	with := newTrigger(t, svc, f.org, push.Trigger{DelayMinutes: hour, Condition: push.ConditionWith, Statuses: []string{"CPA_ACCEPT"}})
	_, none := f.subscribe(t, f.org, f.pwa, f.camp, "US")
	_, accepted := f.subscribe(t, f.org, f.pwa, f.camp, "US")
	_, attributed := f.subscribe(t, f.org, f.pwa, f.camp, "US")
	_, held := f.subscribe(t, f.org, f.pwa, f.camp, "US")
	_, failed := f.subscribe(t, f.org, f.pwa, f.camp, "US")
	postback(accepted, accepted, "CPA_ACCEPT", "success")
	postback(idgen.New(), attributed, "CPA_ACCEPT", "success")
	postback(held, held, "CPA_HOLD", "success")
	postback(failed, failed, "CPA_ACCEPT", "duplicate")
	// Another organization's conversion on the same click id does not count.
	otherOrg, _, _ := f.seedOrg(t)
	otherNetwork := idgen.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, 'n')`, otherNetwork, otherOrg); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO postbacks (id, organization_id, network_id, click_id, status, direction, result)
		VALUES ($1, $2, $3, $4, 'CPA_ACCEPT', 'incoming', 'success')`, idgen.New(), otherOrg, otherNetwork, none); err != nil {
		t.Fatal(err)
	}

	f.sendTriggered(t, repo, time.Now().Add(61*time.Minute))
	if got := f.sentTo(t, without.ID); len(got) != 3 || !got[none] || !got[held] || !got[failed] {
		t.Fatalf("without a deposit sent to %v, want none/held/failed", got)
	}
	if got := f.sentTo(t, with.ID); len(got) != 2 || !got[accepted] || !got[attributed] {
		t.Fatalf("with a deposit sent to %v, want accepted/attributed", got)
	}
}

func TestTriggerValidation(t *testing.T) {
	f := newFixture(t)
	svc := push.NewService(f.repo.ScopedTo(f.org), true)
	at := time.Now().Add(time.Hour)
	var verr *apierror.Error
	for name, tc := range map[string]struct {
		tr     push.Trigger
		sendAt *time.Time
		field  string
	}{
		"delay too short":      {push.Trigger{DelayMinutes: 1}, nil, "trigger.delayMinutes"},
		"delay too long":       {push.Trigger{DelayMinutes: 31 * 24 * 60}, nil, "trigger.delayMinutes"},
		"condition, no status": {push.Trigger{DelayMinutes: hour, Condition: push.ConditionWith}, nil, "trigger.statuses"},
		"unknown status":       {push.Trigger{DelayMinutes: hour, Condition: push.ConditionWithout, Statuses: []string{"LAND_VIEW"}}, nil, "trigger.statuses"},
		"status, no condition": {push.Trigger{DelayMinutes: hour, Statuses: []string{"CPA_ACCEPT"}}, nil, "trigger.statuses"},
		"unknown condition":    {push.Trigger{DelayMinutes: hour, Condition: "maybe"}, nil, "trigger.condition"},
		"with sendAt":          {push.Trigger{DelayMinutes: hour}, &at, "sendAt"},
	} {
		tr := tc.tr
		_, err := svc.CreateBroadcast(context.Background(), f.org, "", push.CreateBroadcastInput{
			Name: "Trigger", Title: "Hi", Target: push.TargetDestination, Trigger: &tr, SendAt: tc.sendAt,
		})
		if !errors.As(err, &verr) || verr.Fields[tc.field] == "" {
			t.Errorf("%s: %v, want a %s error", name, err, tc.field)
		}
	}
	// No subscriber yet is fine: a trigger's audience is who subscribes.
	if b, err := svc.CreateBroadcast(context.Background(), f.org, "", push.CreateBroadcastInput{
		Name: "Trigger", Title: "Hi", Target: push.TargetDestination, Trigger: &push.Trigger{DelayMinutes: hour},
	}); err != nil || b.Status != push.BroadcastActive {
		t.Fatalf("trigger with no subscribers yet = %+v, %v", b, err)
	}
}
