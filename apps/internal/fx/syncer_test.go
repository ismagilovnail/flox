package fx_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/fx"
)

type fakeStore struct {
	latest, oldest       time.Time
	hasLatest, hasOldest bool
	upserted             []fx.Rate
	repaired             bool
}

func (s *fakeStore) LatestDate(context.Context) (time.Time, bool, error) {
	return s.latest, s.hasLatest, nil
}
func (s *fakeStore) OldestMissingDate(context.Context) (time.Time, bool, error) {
	return s.oldest, s.hasOldest, nil
}
func (s *fakeStore) Upsert(_ context.Context, r []fx.Rate) error {
	s.upserted = append(s.upserted, r...)
	return nil
}
func (s *fakeStore) RepairUSD(context.Context) (fx.Repaired, error) {
	s.repaired = true
	return fx.Repaired{}, nil
}

type fakeProvider struct {
	since time.Time
	err   error
}

func (p *fakeProvider) Rates(_ context.Context, since time.Time) ([]fx.Rate, error) {
	p.since = since
	if p.err != nil {
		return nil, p.err
	}
	return []fx.Rate{{Currency: "EUR", Date: since, RateToUSD: 1.1}}, nil
}

var now = time.Date(2026, 10, 6, 15, 0, 0, 0, time.UTC)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func run(t *testing.T, store *fakeStore, p *fakeProvider) error {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return fx.NewSyncer(p, store, logger).WithClock(func() time.Time { return now }).RunOnce(context.Background())
}

func TestSyncerFetchesFromWhereItIsNeeded(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store fakeStore
		want  time.Time
	}{
		{"empty table: initial lookback", fakeStore{}, day(2026, 7, 8)},
		{"rates on file: re-read the last stale window", fakeStore{latest: day(2026, 10, 3), hasLatest: true}, day(2026, 9, 26)},
		{"an older event lacks USD: reach back for it", fakeStore{
			latest: day(2026, 10, 3), hasLatest: true, oldest: day(2025, 1, 10), hasOldest: true,
		}, day(2025, 1, 3)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, p := tc.store, &fakeProvider{}
			if err := run(t, &store, p); err != nil {
				t.Fatalf("RunOnce: %v", err)
			}
			if !p.since.Equal(tc.want) {
				t.Fatalf("since = %s, want %s", p.since.Format(time.DateOnly), tc.want.Format(time.DateOnly))
			}
			if len(store.upserted) != 1 || !store.repaired {
				t.Fatalf("upserted=%d repaired=%v, want the fetched rate stored and a repair run", len(store.upserted), store.repaired)
			}
		})
	}
}

func TestSyncerProviderFailureChangesNothing(t *testing.T) {
	store := &fakeStore{}
	if err := run(t, store, &fakeProvider{err: errors.New("ecb down")}); err == nil {
		t.Fatal("provider failure was not reported")
	}
	if len(store.upserted) != 0 || store.repaired {
		t.Fatal("a failed fetch still wrote or repaired")
	}
}

// Combine (audit DATA-8): one provider down still stores the others'
// rates; every provider down is an error.
func TestCombineKeepsTheRatesOfTheProvidersThatAnswered(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	ecb, cbr := &fakeProvider{}, &fakeProvider{err: errors.New("cbr down")}
	rates, err := fx.Combine(logger, map[string]fx.Provider{"ecb": ecb, "cbr": cbr}).Rates(context.Background(), since)
	if err != nil || len(rates) != 1 || rates[0].Currency != "EUR" {
		t.Fatalf("one down: rates = %+v, err = %v; want the other's", rates, err)
	}
	if !ecb.since.Equal(since) || !cbr.since.Equal(since) {
		t.Fatalf("since passed = %v / %v", ecb.since, cbr.since)
	}
	_, err = fx.Combine(logger, map[string]fx.Provider{"ecb": &fakeProvider{err: errors.New("ecb down")}, "cbr": cbr}).Rates(context.Background(), since)
	if err == nil {
		t.Fatal("every provider down: want an error")
	}
}
