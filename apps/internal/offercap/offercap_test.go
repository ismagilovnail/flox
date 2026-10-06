package offercap_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/offercap"
)

type fakeSource struct{ limits []offercap.Limit }

func (f *fakeSource) Limits(context.Context, time.Time) ([]offercap.Limit, error) {
	return f.limits, nil
}

type fakeCounter struct {
	counts map[string]uint64
	err    error
	got    []chstore.OfferConversionTarget
}

func (f *fakeCounter) NewConversionsByOffer(_ context.Context, t []chstore.OfferConversionTarget) (map[string]uint64, error) {
	f.got = t
	return f.counts, f.err
}

func TestRefreshCapsOnlyOffersAtOrOverTheLimit(t *testing.T) {
	src := &fakeSource{limits: []offercap.Limit{
		{OrganizationID: "o1", OfferID: "under", Cap: 10},
		{OrganizationID: "o1", OfferID: "at", Cap: 5},
		{OrganizationID: "o2", OfferID: "over", Cap: 2},
		{OrganizationID: "o2", OfferID: "zero", Cap: 0},
		{OrganizationID: "o2", OfferID: "none-yet", Cap: 3},
	}}
	ctr := &fakeCounter{counts: map[string]uint64{"o1/under": 9, "o1/at": 5, "o2/over": 7}}
	m := offercap.NewMonitor(src, ctr, slog.Default())

	if err := m.Refresh(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	got := m.Capped()
	for _, id := range []string{"at", "over", "zero"} {
		if _, ok := got[id]; !ok {
			t.Errorf("%s should be capped, got %v", id, got)
		}
	}
	for _, id := range []string{"under", "none-yet"} {
		if _, ok := got[id]; ok {
			t.Errorf("%s should not be capped, got %v", id, got)
		}
	}
}

func TestRefreshFailureKeepsThePreviousSnapshot(t *testing.T) {
	src := &fakeSource{limits: []offercap.Limit{{OrganizationID: "o1", OfferID: "a", Cap: 1}}}
	ctr := &fakeCounter{counts: map[string]uint64{"o1/a": 1}}
	m := offercap.NewMonitor(src, ctr, slog.Default())
	if err := m.Refresh(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}

	ctr.err = errors.New("clickhouse down")
	if err := m.Refresh(context.Background(), time.Now()); err == nil {
		t.Fatal("want an error")
	}
	if _, ok := m.Capped()["a"]; !ok {
		t.Fatal("an analytics outage must not lift an existing cap")
	}
}

func TestNilMonitorHasNoCaps(t *testing.T) {
	var m *offercap.Monitor
	if m.Capped() != nil {
		t.Fatal("nil monitor must report no caps")
	}
}

func TestStartOfDayFollowsTheOrganizationTimezone(t *testing.T) {
	now := time.Date(2026, 10, 6, 22, 30, 0, 0, time.UTC) // 03:30 on the 7th in Yekaterinburg (UTC+5)
	got := offercap.StartOfDay(now, "Asia/Yekaterinburg")
	want := time.Date(2026, 10, 7, 0, 0, 0, 0, time.FixedZone("", 5*3600))
	if !got.Equal(want) {
		t.Fatalf("StartOfDay = %v, want %v", got, want)
	}
	if utc := offercap.StartOfDay(now, "Nope/Nowhere"); !utc.Equal(time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unknown zone should fall back to UTC, got %v", utc)
	}
}
