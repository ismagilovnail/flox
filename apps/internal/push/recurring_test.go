package push_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
	"github.com/ismagilovnail/flox/apps/internal/push"
)

var everyDay = []int{1, 2, 3, 4, 5, 6, 7}

// newRecurring makes a daily template whose first slot is about two hours
// from now in the organization's zone (UTC unless the test set one), so
// the dev stack's worker (real time) leaves it alone; the test sends with
// a later "now".
func newRecurring(t *testing.T, svc *push.Service, org string, until string) (push.Broadcast, time.Time) {
	t.Helper()
	at := time.Now().Add(2 * time.Hour).UTC()
	b, err := svc.CreateBroadcast(context.Background(), org, "", push.CreateBroadcastInput{
		Name: "Daily", Title: "Hi", Target: push.TargetDestination,
		Recurrence: &push.Recurrence{Weekdays: everyDay, Time: at.Format("15:04"), Until: until},
	})
	if err != nil {
		t.Fatalf("CreateBroadcast(recurring): %v", err)
	}
	if b.NextSendAt == nil {
		t.Fatalf("recurring without a next send: %+v", b)
	}
	return b, *b.NextSendAt
}

func (f *fixture) children(t *testing.T, parent string) []push.Broadcast {
	t.Helper()
	all, _, err := f.repo.ListBroadcasts(context.Background(), f.org, pagination.Page{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	var out []push.Broadcast
	for _, b := range all {
		if b.ParentID != nil && *b.ParentID == parent {
			out = append(out, b)
		}
	}
	return out
}

func TestRecurringBroadcast(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	repo := f.repo.ScopedTo(f.org)
	svc := push.NewService(repo, true)
	f.subscribe(t, f.org, f.pwa, f.camp, "US")
	f.subscribe(t, f.org, f.pwa, f.camp, "DE")

	b, slot := newRecurring(t, svc, f.org, "")
	if b.Kind != push.KindRecurring || b.Status != push.BroadcastActive || b.Recurrence == nil ||
		b.Recurrence.Timezone != "UTC" || b.Total != 0 || slot.Before(time.Now().Add(time.Hour)) {
		t.Fatalf("template = %+v", b)
	}
	send := func(now time.Time) int {
		t.Helper()
		n, err := repo.SendRecurring(ctx, now, 20)
		if err != nil {
			t.Fatalf("SendRecurring: %v", err)
		}
		return n
	}
	if n := send(time.Now()); n != 0 {
		t.Fatalf("sent %d before the slot", n)
	}

	// A template another worker holds is skipped at once, not waited for.
	held, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := held.Exec(ctx, `SELECT 1 FROM push_broadcasts WHERE id = $1 FOR UPDATE`, b.ID); err != nil {
		t.Fatal(err)
	}
	quick, cancel := context.WithTimeout(ctx, 2*time.Second)
	n, err := repo.SendRecurring(quick, slot.Add(time.Minute), 20)
	cancel()
	_ = held.Rollback(ctx)
	if err != nil || n != 0 {
		t.Fatalf("with the template held elsewhere: %d, %v, want skipped", n, err)
	}

	// Two workers at the slot: it goes out once.
	var wg sync.WaitGroup
	var mu sync.Mutex
	total := 0
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := repo.SendRecurring(ctx, slot.Add(time.Minute), 20)
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			total += n
			mu.Unlock()
		}()
	}
	wg.Wait()
	kids := f.children(t, b.ID)
	if total != 1 || len(kids) != 1 {
		t.Fatalf("at the slot: %d sends, %d broadcasts, want 1", total, len(kids))
	}
	kid := kids[0]
	if kid.Kind != push.KindOnce || kid.Status != push.BroadcastSent || kid.Total != 2 ||
		// The name carries the slot's date in the recurrence's zone (UTC
		// here), not the machine's: in UTC+5 after 22:00 they differ.
		kid.Name != "Daily · "+slot.UTC().Format(time.DateOnly) {
		t.Fatalf("send = %+v", kid)
	}
	got, _ := svc.GetBroadcast(ctx, f.org, b.ID)
	if got.Occurrences != 1 || got.Total != 2 || got.NextSendAt == nil || !got.NextSendAt.Equal(slot.Add(24*time.Hour)) {
		t.Fatalf("template after a send = %+v", got)
	}

	// The next slot reached two hours late (worker down): skipped, moved on.
	if n := send(slot.Add(26 * time.Hour)); n != 0 || len(f.children(t, b.ID)) != 1 {
		t.Fatalf("late slot sent %d", n)
	}
	got, _ = svc.GetBroadcast(ctx, f.org, b.ID)
	if !got.NextSendAt.Equal(slot.Add(48 * time.Hour)) {
		t.Fatalf("after a skipped slot next = %v, want %v", got.NextSendAt, slot.Add(48*time.Hour))
	}

	// No one matching at a slot: nothing is created, the template moves on.
	if _, err := f.pool.Exec(ctx, `UPDATE push_subscriptions SET status = 'unsubscribed' WHERE organization_id = $1`, f.org); err != nil {
		t.Fatal(err)
	}
	if n := send(slot.Add(48*time.Hour + time.Minute)); n != 0 || len(f.children(t, b.ID)) != 1 {
		t.Fatalf("empty audience sent %d", n)
	}

	// Pause: nothing goes out; resume: next slot from now.
	var apiErr *apierror.Error
	otherOrg, _, _ := f.seedOrg(t)
	if _, err := svc.SetActive(ctx, otherOrg, b.ID, false); !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Fatalf("other organization pauses: %v, want 404", err)
	}
	if p, err := svc.SetActive(ctx, f.org, b.ID, false); err != nil || p.Status != push.BroadcastPaused {
		t.Fatalf("pause = %+v, %v", p, err)
	}
	if n := send(slot.Add(240 * time.Hour)); n != 0 {
		t.Fatalf("paused template sent %d", n)
	}
	r, err := svc.SetActive(ctx, f.org, b.ID, true)
	if err != nil || r.Status != push.BroadcastActive || r.NextSendAt == nil || !r.NextSendAt.Equal(slot) {
		t.Fatalf("resume = %+v, %v, want next at today's slot %v", r, err, slot)
	}
	if _, err := svc.SetActive(ctx, f.org, kid.ID, false); !errors.As(err, &apiErr) || apiErr.Status != 409 {
		t.Fatalf("pause a send: %v, want 409", err)
	}
}

// An end date: the last slot goes out, then the template is finished. The
// organization's zone is the template's.
func TestRecurringUntil(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE organizations SET timezone = 'Asia/Kathmandu' WHERE id = $1`, f.org); err != nil {
		t.Fatal(err)
	}
	repo := f.repo.ScopedTo(f.org)
	svc := push.NewService(repo, true)
	f.subscribe(t, f.org, f.pwa, f.camp, "US")

	kathmandu, _ := time.LoadLocation("Asia/Kathmandu")
	at := time.Now().Add(2 * time.Hour).In(kathmandu)
	b, err := svc.CreateBroadcast(ctx, f.org, "", push.CreateBroadcastInput{
		Name: "Once more", Title: "Hi", Target: push.TargetDestination,
		Recurrence: &push.Recurrence{Weekdays: everyDay, Time: at.Format("15:04"), Timezone: "Europe/London", Until: at.Format(time.DateOnly)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if b.Recurrence.Timezone != "Asia/Kathmandu" {
		t.Fatalf("timezone = %q, want the organization's", b.Recurrence.Timezone)
	}
	if n, err := repo.SendRecurring(ctx, b.NextSendAt.Add(time.Minute), 20); err != nil || n != 1 {
		t.Fatalf("last slot: %d, %v", n, err)
	}
	got, _ := svc.GetBroadcast(ctx, f.org, b.ID)
	if got.Status != push.BroadcastFinished || got.NextSendAt != nil || got.Occurrences != 1 {
		t.Fatalf("after the end date = %+v", got)
	}
	var apiErr *apierror.Error
	if _, err := svc.SetActive(ctx, f.org, b.ID, true); !errors.As(err, &apiErr) || apiErr.Status != 409 {
		t.Fatalf("resume a finished one: %v, want 409", err)
	}
}

func TestRecurringValidation(t *testing.T) {
	f := newFixture(t)
	svc := push.NewService(f.repo.ScopedTo(f.org), true)
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly)
	at := time.Now().Add(time.Hour)
	var verr *apierror.Error
	for name, tc := range map[string]struct {
		rec     push.Recurrence
		sendAt  *time.Time
		trigger *push.Trigger
		field   string
	}{
		"no weekdays":     {push.Recurrence{Time: "19:00"}, nil, nil, "recurrence.weekdays"},
		"weekday 8":       {push.Recurrence{Weekdays: []int{8}, Time: "19:00"}, nil, nil, "recurrence.weekdays"},
		"weekday twice":   {push.Recurrence{Weekdays: []int{1, 1}, Time: "19:00"}, nil, nil, "recurrence.weekdays"},
		"bad time":        {push.Recurrence{Weekdays: everyDay, Time: "7pm"}, nil, nil, "recurrence.time"},
		"bad until":       {push.Recurrence{Weekdays: everyDay, Time: "19:00", Until: "soon"}, nil, nil, "recurrence.until"},
		"until yesterday": {push.Recurrence{Weekdays: everyDay, Time: "19:00", Until: yesterday}, nil, nil, "recurrence.until"},
		"with sendAt":     {push.Recurrence{Weekdays: everyDay, Time: "19:00"}, &at, nil, "recurrence"},
		"with trigger":    {push.Recurrence{Weekdays: everyDay, Time: "19:00"}, nil, &push.Trigger{DelayMinutes: 60}, "recurrence"},
	} {
		rec := tc.rec
		_, err := svc.CreateBroadcast(context.Background(), f.org, "", push.CreateBroadcastInput{
			Name: "Recurring", Title: "Hi", Target: push.TargetDestination, Recurrence: &rec, SendAt: tc.sendAt, Trigger: tc.trigger,
		})
		if !errors.As(err, &verr) || verr.Fields[tc.field] == "" {
			t.Errorf("%s: %v, want a %s error", name, err, tc.field)
		}
	}
}
