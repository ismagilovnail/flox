package ltv_test

import (
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/ltv"
)

func at(day int) time.Time {
	return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC).AddDate(0, 0, day)
}

func deposit(day int, typ event.Type, usd float64) chstore.Deposit {
	return chstore.Deposit{EventAt: at(day), Type: typ, USDValue: usd, HasUSDValue: true}
}

func history(clickID string, deposits ...chstore.Deposit) chstore.ClickHistory {
	return chstore.ClickHistory{ClickID: clickID, CampaignID: "camp-1", NetworkID: "net-1", Country: "US", Deposits: deposits}
}

// TestFTDWindowBucketing is the money test: an FTD click with one deposit
// in every window (plus one beyond day 90) must land each dollar in
// exactly the right bucket, and ltv_total must be the capped-at-90 sum.
func TestFTDWindowBucketing(t *testing.T) {
	h := history("c1",
		deposit(0, event.CpaAccept, 10),  // FTD itself: day 0
		deposit(0, event.CpaRedep, 5),    // same-day redep: also day 0
		deposit(1, event.CpaRedep, 20),   // day 1: d1_7
		deposit(7, event.CpaRedep, 21),   // day 7: still d1_7 (inclusive upper bound)
		deposit(8, event.CpaRedep, 30),   // day 8: d8_30
		deposit(30, event.CpaRedep, 31),  // day 30: still d8_30
		deposit(31, event.CpaRedep, 40),  // day 31: d31_90
		deposit(90, event.CpaRedep, 41),  // day 90: still d31_90
		deposit(91, event.CpaRedep, 999), // day 91: beyond every window, excluded
	)

	cohorts := ltv.ComputeFTDCohorts([]chstore.ClickHistory{h}, ltv.PeriodDay, at(200))
	if len(cohorts) != 1 {
		t.Fatalf("cohorts = %d, want 1", len(cohorts))
	}
	c := cohorts[0]

	want := map[ltv.WindowKey]float64{
		ltv.WindowD0:     15, // 10 + 5
		ltv.WindowD1_7:   41, // 20 + 21
		ltv.WindowD8_30:  61, // 30 + 31
		ltv.WindowD31_90: 81, // 40 + 41
	}
	for w, wantRevenue := range want {
		got := c.Windows[w]
		if got.RevenueUSD != wantRevenue {
			t.Errorf("window %s revenue = %v, want %v", w, got.RevenueUSD, wantRevenue)
		}
		if !got.Complete {
			t.Errorf("window %s Complete = false at asOf=day 200, want true", w)
		}
	}

	wantTotal := 15.0 + 41 + 61 + 81 // excludes the day-91 deposit
	if c.LTVTotalUSD != wantTotal {
		t.Fatalf("LTVTotalUSD = %v, want %v (day-91 deposit must be excluded)", c.LTVTotalUSD, wantTotal)
	}
	if c.LTVPerAnchorUSD != wantTotal {
		t.Fatalf("LTVPerAnchorUSD = %v, want %v (one click, so per-anchor == total)", c.LTVPerAnchorUSD, wantTotal)
	}
	// TotalDeposits/TotalDepositRevenueUSD are NOT window-capped — the
	// day-91 deposit counts here even though it's excluded from LTVTotalUSD.
	if c.TotalDeposits != 9 {
		t.Fatalf("TotalDeposits = %d, want 9 (all ACCEPT+REDEP rows, uncapped)", c.TotalDeposits)
	}
	wantDepositRevenue := wantTotal + 999
	if c.TotalDepositRevenueUSD != wantDepositRevenue {
		t.Fatalf("TotalDepositRevenueUSD = %v, want %v", c.TotalDepositRevenueUSD, wantDepositRevenue)
	}
}

// TestWindowIncompleteness: a window whose time span hasn't elapsed yet
// must be marked incomplete, with whatever partial revenue it has so far
// — never silently reported as a clean zero.
func TestWindowIncompleteness(t *testing.T) {
	h := history("c1",
		deposit(0, event.CpaAccept, 10),
		deposit(5, event.CpaRedep, 7), // partial d1_7 data
	)

	// asOf = day 4: the FTD is only 4 days old. d0's own 1-day span has
	// elapsed (day 0 is fully in the past by day 4), but d1_7 (spans
	// through day 7) has not, and neither has d8_30 or d31_90. The day-5
	// deposit already exists in the data either way (asOf only controls
	// the Complete flag, never which rows were fetched) — its revenue
	// shows up in the still-incomplete d1_7 window, which is exactly
	// §26.5's acceptance criterion: incomplete windows show their partial
	// total, not a zero.
	cohorts := ltv.ComputeFTDCohorts([]chstore.ClickHistory{h}, ltv.PeriodDay, at(4))
	c := cohorts[0]

	if !c.Windows[ltv.WindowD0].Complete {
		t.Error("d0 should be complete by day 4 (anchor + 1 day has passed)")
	}
	if c.Windows[ltv.WindowD1_7].Complete {
		t.Error("d1_7 should NOT be complete by day 4 (spans through day 7)")
	}
	if c.Windows[ltv.WindowD1_7].RevenueUSD != 7 {
		t.Errorf("d1_7 revenue at day 4 = %v, want 7 (partial data shown despite being incomplete)", c.Windows[ltv.WindowD1_7].RevenueUSD)
	}
	if c.Windows[ltv.WindowD8_30].Complete || c.Windows[ltv.WindowD31_90].Complete {
		t.Error("d8_30 and d31_90 must not be complete by day 4")
	}

	// Now evaluate as of day 10: d1_7 has fully elapsed and its revenue
	// shows, but d8_30/d31_90 still haven't.
	cohorts = ltv.ComputeFTDCohorts([]chstore.ClickHistory{h}, ltv.PeriodDay, at(10))
	c = cohorts[0]
	if !c.Windows[ltv.WindowD1_7].Complete {
		t.Error("d1_7 should be complete by day 10")
	}
	if c.Windows[ltv.WindowD1_7].RevenueUSD != 7 {
		t.Errorf("d1_7 revenue at day 10 = %v, want 7", c.Windows[ltv.WindowD1_7].RevenueUSD)
	}
	if c.Windows[ltv.WindowD8_30].Complete {
		t.Error("d8_30 should still be incomplete by day 10")
	}
}

// TestConservativeCompletenessAcrossCohortMembers: a "week" cohort with
// members on different calendar days must not call a window complete
// until it's complete for the YOUNGEST member.
func TestConservativeCompletenessAcrossCohortMembers(t *testing.T) {
	// 2026-01-01 (day 0) is a Thursday, ISO week 1; day 3 (Sunday) is the
	// last day still in week 1 — day 4 (Monday) would already be week 2.
	early := history("c1", deposit(0, event.CpaAccept, 1))
	late := history("c2", deposit(3, event.CpaAccept, 1)) // same ISO week as day 0

	// asOf = day 2: d0 (span 1 day) is complete for "early" (anchor day 0,
	// 2 days elapsed) but NOT for "late" (anchor day 3, still in the future
	// relative to asOf).
	cohorts := ltv.ComputeFTDCohorts([]chstore.ClickHistory{early, late}, ltv.PeriodWeek, at(2))
	if len(cohorts) != 1 {
		t.Fatalf("cohorts = %d, want 1 (both clicks in the same ISO week)", len(cohorts))
	}
	c := cohorts[0]
	if c.AnchorCount != 2 {
		t.Fatalf("AnchorCount = %d, want 2", c.AnchorCount)
	}
	if c.Windows[ltv.WindowD0].Complete {
		t.Error("d0 must not be complete: the youngest member (anchor day 3) hasn't reached its own day-1 yet as of day 2")
	}
}

// TestFTDToRedepAndDepToRedepRates verifies §26.5's exact formulas:
// ftd_to_redep_rate = redep_unique / cpa_accept, dep_to_redep = cpa_redep / cpa_accept.
func TestFTDToRedepAndDepToRedepRates(t *testing.T) {
	// 4 FTD clicks: 3 have at least one redep (one of them has TWO redeps),
	// 1 has none.
	h1 := history("c1", deposit(0, event.CpaAccept, 1), deposit(1, event.CpaRedep, 1), deposit(2, event.CpaRedep, 1))
	h2 := history("c2", deposit(0, event.CpaAccept, 1), deposit(1, event.CpaRedep, 1))
	h3 := history("c3", deposit(0, event.CpaAccept, 1), deposit(1, event.CpaRedep, 1))
	h4 := history("c4", deposit(0, event.CpaAccept, 1)) // no redep

	cohorts := ltv.ComputeFTDCohorts([]chstore.ClickHistory{h1, h2, h3, h4}, ltv.PeriodDay, at(200))
	c := cohorts[0]

	if c.AnchorCount != 4 {
		t.Fatalf("AnchorCount = %d, want 4", c.AnchorCount)
	}
	if c.RedepUniqueCount != 3 {
		t.Fatalf("RedepUniqueCount = %d, want 3", c.RedepUniqueCount)
	}
	wantFTDToRedep := 3.0 / 4.0
	if c.FTDToRedepRate != wantFTDToRedep {
		t.Fatalf("FTDToRedepRate = %v, want %v", c.FTDToRedepRate, wantFTDToRedep)
	}
	// 4 redep EVENTS total (c1 has 2, c2 and c3 have 1 each) / 4 accepts.
	wantDepToRedep := 4.0 / 4.0
	if c.DepToRedepRate != wantDepToRedep {
		t.Fatalf("DepToRedepRate = %v, want %v", c.DepToRedepRate, wantDepToRedep)
	}
}

// TestLifetimeDaysAvgOnlyCountsClicksWithRedep: §26.5's lifetime_days is
// "days from FTD to last redeposit" — undefined, not zero, for a click
// that never redeposited.
func TestLifetimeDaysAvgOnlyCountsClicksWithRedep(t *testing.T) {
	withRedep := history("c1", deposit(0, event.CpaAccept, 1), deposit(10, event.CpaRedep, 1))
	noRedep := history("c2", deposit(0, event.CpaAccept, 1))

	cohorts := ltv.ComputeFTDCohorts([]chstore.ClickHistory{withRedep, noRedep}, ltv.PeriodDay, at(200))
	c := cohorts[0]
	if !c.HasLifetimeData {
		t.Fatal("HasLifetimeData = false, want true (one click has a redep)")
	}
	if c.LifetimeDaysAvg != 10 {
		t.Fatalf("LifetimeDaysAvg = %v, want 10 (averaged over only the one click with a redep, not both)", c.LifetimeDaysAvg)
	}

	// All clicks with no redep at all: no lifetime data, not zero.
	cohorts = ltv.ComputeFTDCohorts([]chstore.ClickHistory{noRedep}, ltv.PeriodDay, at(200))
	c = cohorts[0]
	if c.HasLifetimeData {
		t.Fatal("HasLifetimeData = true, want false when no click in the cohort has any redep")
	}
}

// TestRegCohortAnchorsOnHoldNotAccept: the same click's FTD- and Reg-
// anchored windows must differ when registration and first deposit happen
// on different days.
func TestRegCohortAnchorsOnHoldNotAccept(t *testing.T) {
	h := chstore.ClickHistory{
		ClickID: "c1", CampaignID: "camp-1", NetworkID: "net-1", Country: "US",
		Deposits: []chstore.Deposit{
			{EventAt: at(0), Type: event.CpaHold}, // reg on day 0
			deposit(3, event.CpaAccept, 10),       // FTD on day 3 -> reg-anchored offset 3 = d1_7
			deposit(4, event.CpaRedep, 5),         // redep on day 4 -> reg-anchored offset 4 = d1_7
		},
	}

	regCohorts := ltv.ComputeRegCohorts([]chstore.ClickHistory{h}, ltv.PeriodDay, at(200))
	rc := regCohorts[0]
	if rc.Windows[ltv.WindowD0].RevenueUSD != 0 {
		t.Fatalf("reg-anchored d0 revenue = %v, want 0 (both deposits land after day 0 relative to registration)", rc.Windows[ltv.WindowD0].RevenueUSD)
	}
	if rc.Windows[ltv.WindowD1_7].RevenueUSD != 15 {
		t.Fatalf("reg-anchored d1_7 revenue = %v, want 15 (10 + 5, both fall in days 1-7 from registration)", rc.Windows[ltv.WindowD1_7].RevenueUSD)
	}

	ftdCohorts := ltv.ComputeFTDCohorts([]chstore.ClickHistory{h}, ltv.PeriodDay, at(200))
	fc := ftdCohorts[0]
	if fc.Windows[ltv.WindowD0].RevenueUSD != 10 {
		t.Fatalf("ftd-anchored d0 revenue = %v, want 10 (the FTD deposit itself is always day 0 relative to itself)", fc.Windows[ltv.WindowD0].RevenueUSD)
	}
	if fc.Windows[ltv.WindowD1_7].RevenueUSD != 5 {
		t.Fatalf("ftd-anchored d1_7 revenue = %v, want 5 (only the redep, one day after FTD)", fc.Windows[ltv.WindowD1_7].RevenueUSD)
	}
}

// TestRegToFTDRate verifies reg_to_ftd_rate = cpa_accept / cpa_hold: the
// share of a Reg cohort's registrants who reached a first deposit at all.
func TestRegToFTDRate(t *testing.T) {
	converted := chstore.ClickHistory{ClickID: "c1", CampaignID: "camp-1", NetworkID: "net-1", Country: "US", Deposits: []chstore.Deposit{
		{EventAt: at(0), Type: event.CpaHold}, deposit(1, event.CpaAccept, 10),
	}}
	notConverted := chstore.ClickHistory{ClickID: "c2", CampaignID: "camp-1", NetworkID: "net-1", Country: "US", Deposits: []chstore.Deposit{
		{EventAt: at(0), Type: event.CpaHold},
	}}

	cohorts := ltv.ComputeRegCohorts([]chstore.ClickHistory{converted, notConverted}, ltv.PeriodDay, at(200))
	c := cohorts[0]
	if c.AnchorCount != 2 {
		t.Fatalf("AnchorCount = %d, want 2", c.AnchorCount)
	}
	if c.RegToFTDRate != 0.5 {
		t.Fatalf("RegToFTDRate = %v, want 0.5", c.RegToFTDRate)
	}
}

// TestMissingFXRateContributesZeroNotError: a deposit with no FX rate on
// file (HasUSDValue=false) must count toward TotalDeposits but contribute
// exactly 0 revenue — never an error, never fabricated.
func TestMissingFXRateContributesZeroNotError(t *testing.T) {
	h := chstore.ClickHistory{ClickID: "c1", CampaignID: "camp-1", NetworkID: "net-1", Country: "US", Deposits: []chstore.Deposit{
		{EventAt: at(0), Type: event.CpaAccept, USDValue: 0, HasUSDValue: false},
	}}
	cohorts := ltv.ComputeFTDCohorts([]chstore.ClickHistory{h}, ltv.PeriodDay, at(200))
	c := cohorts[0]
	if c.LTVTotalUSD != 0 {
		t.Fatalf("LTVTotalUSD = %v, want 0", c.LTVTotalUSD)
	}
	if c.TotalDeposits != 1 {
		t.Fatalf("TotalDeposits = %d, want 1 (the deposit still happened, just with no known USD value)", c.TotalDeposits)
	}
}

// TestGroupingByDimensions: cohorts with different campaign/network/country
// must never merge into one row.
func TestGroupingByDimensions(t *testing.T) {
	a := chstore.ClickHistory{ClickID: "c1", CampaignID: "camp-A", NetworkID: "net-1", Country: "US", Deposits: []chstore.Deposit{deposit(0, event.CpaAccept, 1)}}
	b := chstore.ClickHistory{ClickID: "c2", CampaignID: "camp-B", NetworkID: "net-1", Country: "US", Deposits: []chstore.Deposit{deposit(0, event.CpaAccept, 1)}}
	c := chstore.ClickHistory{ClickID: "c3", CampaignID: "camp-A", NetworkID: "net-1", Country: "DE", Deposits: []chstore.Deposit{deposit(0, event.CpaAccept, 1)}}

	cohorts := ltv.ComputeFTDCohorts([]chstore.ClickHistory{a, b, c}, ltv.PeriodDay, at(200))
	if len(cohorts) != 3 {
		t.Fatalf("cohorts = %d, want 3 (different campaign or country must not merge)", len(cohorts))
	}
}

// TestPeriodGrouping: day/week/month must bucket the same click
// differently.
func TestPeriodGrouping(t *testing.T) {
	h := history("c1", deposit(0, event.CpaAccept, 1))

	dayCohorts := ltv.ComputeFTDCohorts([]chstore.ClickHistory{h}, ltv.PeriodDay, at(200))
	if dayCohorts[0].Key.Bucket != "2026-01-01" {
		t.Fatalf("day bucket = %q, want 2026-01-01", dayCohorts[0].Key.Bucket)
	}

	monthCohorts := ltv.ComputeFTDCohorts([]chstore.ClickHistory{h}, ltv.PeriodMonth, at(200))
	if monthCohorts[0].Key.Bucket != "2026-01" {
		t.Fatalf("month bucket = %q, want 2026-01", monthCohorts[0].Key.Bucket)
	}

	weekCohorts := ltv.ComputeFTDCohorts([]chstore.ClickHistory{h}, ltv.PeriodWeek, at(200))
	if weekCohorts[0].Key.Bucket != "2026-W01" {
		t.Fatalf("week bucket = %q, want 2026-W01", weekCohorts[0].Key.Bucket)
	}
}

// TestEmptyInputProducesNoCohorts: no histories, no rows — not a panic,
// not a zero-valued cohort.
func TestEmptyInputProducesNoCohorts(t *testing.T) {
	if got := ltv.ComputeFTDCohorts(nil, ltv.PeriodDay, at(0)); len(got) != 0 {
		t.Fatalf("cohorts = %d, want 0", len(got))
	}
	if got := ltv.ComputeRegCohorts(nil, ltv.PeriodDay, at(0)); len(got) != 0 {
		t.Fatalf("cohorts = %d, want 0", len(got))
	}
}
