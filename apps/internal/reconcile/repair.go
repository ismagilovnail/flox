package reconcile

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// MaxRepairsPerRun bounds how many events one run rewrites: a systematic
// problem must page someone, not churn the whole store.
const MaxRepairsPerRun = 1000

// Repaired summarizes one repair.
type Repaired struct {
	Reemitted, Deleted int
	// Unmatched counts ClickHouse rows without an event_id (written before
	// event ids existed): they cannot be tied to a Postgres row, so they are
	// left alone and their bucket keeps differing.
	Unmatched int
	// Skipped counts actions beyond MaxRepairsPerRun, left for later runs.
	Skipped int
}

// Repair makes ClickHouse match Postgres for each mismatched bucket, event
// by event: a conversion missing from ClickHouse is re-emitted; one with
// several copies, or a different USD value, has every copy deleted and is
// re-emitted once; an event Postgres never recorded is deleted. Its LTV rows
// go with it, and the daily aggregate rows it fed are recomputed. Re-emitted
// events are rebuilt by conversion.EventFor from the postbacks row and the
// attributed click — the event the record path queued.
//
// This is also how amounts that got their USD value after the fact (an FX
// rate arriving late, fx.RepairUSD) reach the reports: the bucket's USD sum
// differs, and the event is rewritten.
func (r *Reconciler) Repair(ctx context.Context, mismatches []Mismatch, from, to time.Time) (Repaired, error) {
	var out Repaired
	budget := MaxRepairsPerRun
	for _, m := range mismatches {
		day, err := time.Parse(time.DateOnly, m.Day)
		if err != nil {
			return out, fmt.Errorf("reconcile: bucket day %q: %w", m.Day, err)
		}
		lo, hi := maxTime(day, from), minTime(day.AddDate(0, 0, 1), to)

		rows, err := r.conversions.StoredSuccesses(ctx, m.OrganizationID, event.Type(m.Status), lo, hi)
		if err != nil {
			return out, err
		}
		stored, err := r.events.ConversionEventsByID(ctx, m.OrganizationID, m.Status, lo, hi)
		if err != nil {
			return out, err
		}

		var deletes []string
		var reemit []conversion.StoredConversion
		daily := map[chstore.DailyBucket]bool{}
		bucketDay := day
		for _, row := range rows {
			got, ok := stored[row.ID]
			want := expectedUSD(row)
			switch {
			case !ok:
				reemit = append(reemit, row)
			case got.Copies != 1 || math.Abs(got.USD-want) > 0.01:
				deletes = append(deletes, row.ID)
				reemit = append(reemit, row)
				daily[chstore.DailyBucket{CampaignID: got.CampaignID, Type: m.Status, Day: bucketDay}] = true
			}
		}
		known := map[string]bool{}
		for _, row := range rows {
			known[row.ID] = true
		}
		for id, got := range stored {
			switch {
			case id == "":
				out.Unmatched += int(got.Copies)
			case !known[id]:
				deletes = append(deletes, id)
				daily[chstore.DailyBucket{CampaignID: got.CampaignID, Type: m.Status, Day: bucketDay}] = true
			}
		}

		if n := len(deletes) + len(reemit); n > budget {
			out.Skipped += n
			continue
		} else {
			budget -= n
		}

		if err := r.events.DeleteConversionEvents(ctx, m.OrganizationID, deletes); err != nil {
			return out, err
		}
		events := make([]event.Event, 0, len(reemit))
		for _, row := range reemit {
			click, err := r.attributedClick(ctx, row)
			if err != nil {
				return out, err
			}
			ev := conversion.EventFor(row, click)
			events = append(events, ev)
			daily[chstore.DailyBucket{CampaignID: ev.CampaignID, Type: m.Status, Day: bucketDay}] = true
		}
		if len(events) > 0 {
			// Its own batch id: these rows are a new write, not a retry of
			// the batch that lost them.
			if err := r.events.InsertBatch(chstore.WithBatchID(ctx, "repair-"+idgen.New()), events); err != nil {
				return out, fmt.Errorf("reconcile: re-emitting events: %w", err)
			}
		}
		buckets := make([]chstore.DailyBucket, 0, len(daily))
		for b := range daily {
			buckets = append(buckets, b)
		}
		if err := r.events.RebuildDailyConversionBuckets(ctx, m.OrganizationID, buckets); err != nil {
			return out, err
		}

		out.Reemitted += len(events)
		out.Deleted += len(deletes)
		metrics.ReconciliationRepairedTotal.WithLabelValues("reemitted").Add(float64(len(events)))
		metrics.ReconciliationRepairedTotal.WithLabelValues("deleted").Add(float64(len(deletes)))
		r.logger.Warn("repaired clickhouse conversions from postgres",
			"organization_id", m.OrganizationID, "day", m.Day, "status", m.Status,
			"reemitted", len(events), "deleted", len(deletes))
	}
	return out, nil
}

// expectedUSD is what one postbacks row contributes to a report's revenue —
// the same rule Compare sums with.
func expectedUSD(c conversion.StoredConversion) float64 {
	if c.Status == event.CpaDecline || c.Status == event.CpaTrash {
		if c.Reversal != nil && c.Reversal.USDValue != nil {
			return -*c.Reversal.USDValue
		}
		return 0
	}
	if c.USDValue != nil {
		return *c.USDValue
	}
	return 0
}

func (r *Reconciler) attributedClick(ctx context.Context, c conversion.StoredConversion) (*attribution.Click, error) {
	if c.AttributedClickID == "" {
		return nil, nil
	}
	click, err := r.clicks.ByClickID(ctx, c.OrganizationID, c.AttributedClickID)
	if errors.Is(err, attribution.ErrClickNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reconcile: loading attributed click: %w", err)
	}
	return &click, nil
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
