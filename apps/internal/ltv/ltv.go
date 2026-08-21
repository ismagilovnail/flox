// Package ltv is Phase 26.5's cohort/LTV engine (§26.5) — "a primary
// reason teams pay for a tracker in this vertical. Do not skip."
//
// Like internal/routing, this package is pure: given a click's full
// CPA_HOLD/CPA_ACCEPT/CPA_REDEP history (chstore.ClickHistory), it computes
// cohort membership and LTV windows deterministically, with no database,
// no clock of its own (the caller supplies asOf), and no I/O. ClickHouse's
// job (internal/chstore.ClicksByFTDAnchor/ClicksByRegAnchor) is only to
// fetch the raw rows; every number in a Cohort is arithmetic over those
// rows, which is what makes "numbers reconcile against fixtures" (§26.5's
// stated acceptance criterion) something this package's own tests can
// prove directly, the same way internal/routing's conformance fixture
// does for weighted flow selection.
package ltv

import (
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
)

// CohortPeriod is the grouping granularity §26.5 names for both cohort
// types (cohort_day/week/month, reg_cohort_day/week/month).
type CohortPeriod string

const (
	PeriodDay   CohortPeriod = "day"
	PeriodWeek  CohortPeriod = "week"
	PeriodMonth CohortPeriod = "month"
)

// bucket formats t according to period — the cohort grouping key's date
// component. Week uses ISO 8601 week numbering (time.Time.ISOWeek): a
// week's Monday can fall in the calendar month/year before its Thursday,
// and ISO week numbering is what avoids that ambiguity leaking into a
// cohort table's row labels.
func bucket(period CohortPeriod, t time.Time) string {
	switch period {
	case PeriodWeek:
		year, week := t.ISOWeek()
		return fmt2Digit(year) + "-W" + fmt2DigitPad(week)
	case PeriodMonth:
		return t.Format("2006-01")
	default:
		return t.Format("2006-01-02")
	}
}

func fmt2Digit(year int) string {
	return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006")
}

func fmt2DigitPad(week int) string {
	if week < 10 {
		return "0" + itoa(week)
	}
	return itoa(week)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := [4]byte{}
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}

// WindowKey is one of §26.5's four LTV buckets, day-offset-from-anchor
// inclusive on both ends: d0=[0,0], d1_7=[1,7], d8_30=[8,30], d31_90=[31,90].
// A deposit landing on day 91+ relative to its anchor falls outside every
// window and is excluded from LTVTotalUSD — §26.5's own ltv_total formula
// (d0+d1_7+d8_30+d31_90) is capped at 90 days by construction, not an
// oversight here.
type WindowKey string

const (
	WindowD0     WindowKey = "d0"
	WindowD1_7   WindowKey = "d1_7"
	WindowD8_30  WindowKey = "d8_30"
	WindowD31_90 WindowKey = "d31_90"
)

// Windows is every window key, in the order §26.5 lists them.
var Windows = []WindowKey{WindowD0, WindowD1_7, WindowD8_30, WindowD31_90}

type windowBound struct{ minDay, maxDay int }

var windowBounds = map[WindowKey]windowBound{
	WindowD0:     {0, 0},
	WindowD1_7:   {1, 7},
	WindowD8_30:  {8, 30},
	WindowD31_90: {31, 90},
}

func windowForOffset(offsetDays int) (WindowKey, bool) {
	for _, w := range Windows {
		b := windowBounds[w]
		if offsetDays >= b.minDay && offsetDays <= b.maxDay {
			return w, true
		}
	}
	return "", false
}

// WindowResult is one cohort's revenue in one window, and whether that
// window's full time span has actually elapsed yet.
//
// Complete is evaluated against the LATEST anchor date among the cohort's
// own clicks (conservative on purpose): a "day" cohort's members mostly
// share one calendar date, but a "week" or "month" cohort spans several,
// and a window can't be called complete for the whole group until it's
// complete for the youngest member too. RevenueUSD is populated either
// way — §26.5's acceptance criterion is "incomplete windows are visibly
// marked, not shown as zero," which means the partial total still shows,
// just flagged.
type WindowResult struct {
	RevenueUSD float64
	Complete   bool
}

// CohortKey identifies one row of a cohort table: a period bucket plus
// §26.5's filterable dimensions actually available in the event pipeline
// today (campaign, network — the "cpa/network" filter — and country).
// "source" and "offer" are named by §26.5 too but apps/internal/event.Event
// carries neither traffic_source_id nor offer_id yet — the same
// pre-existing gap apps/internal/macro's {source} token and Phase 26's
// click_events sort key both already document. Grouping by them is future
// work once that propagation exists, not silently approximated here.
type CohortKey struct {
	Period     CohortPeriod
	Bucket     string
	CampaignID string
	NetworkID  string
	Country    string
}

// base holds every field FTD and Reg cohorts compute identically — the
// windows, the lifetime/redep/deposit aggregates — so ComputeFTDCohorts
// and ComputeRegCohorts share one accumulation loop and differ only in
// which deposit type anchors each click and which rate metric they attach.
type base struct {
	Key                    CohortKey
	AnchorCount            int
	Windows                map[WindowKey]WindowResult
	LTVTotalUSD            float64
	LTVPerAnchorUSD        float64
	LifetimeDaysAvg        float64
	HasLifetimeData        bool
	RedepUniqueCount       int
	TotalDeposits          int
	TotalDepositRevenueUSD float64
}

// FTDCohort is one row of the FTD cohort table (§26.5), anchored on each
// click's CPA_ACCEPT.
type FTDCohort struct {
	base
	// FTDToRedepRate = redep_unique / cpa_accept (== AnchorCount here).
	FTDToRedepRate float64
	// DepToRedepRate = cpa_redep (event count) / cpa_accept.
	DepToRedepRate float64
}

// RegCohort is one row of the Reg cohort table (§26.5), anchored on each
// click's CPA_HOLD.
type RegCohort struct {
	base
	// RegToFTDRate = cpa_accept / cpa_hold (== AnchorCount here) — the
	// share of this cohort's registrants who reached a first deposit at
	// all, regardless of when.
	RegToFTDRate float64
}

type accumulator struct {
	key             CohortKey
	anchorCount     int
	windowRevenue   map[WindowKey]float64
	maxAnchorDate   time.Time
	lifetimeDaysSum float64
	lifetimeCount   int
	redepUnique     int
	depositCount    int
	depositRevenue  float64
	redepEventCount int
	ftdCount        int // Reg cohorts only: clicks that also reached CPA_ACCEPT
}

func newAccumulator(key CohortKey) *accumulator {
	return &accumulator{key: key, windowRevenue: map[WindowKey]float64{}}
}

func (a *accumulator) addClick(anchorDate time.Time, deposits []chstore.Deposit) {
	a.anchorCount++
	if anchorDate.After(a.maxAnchorDate) {
		a.maxAnchorDate = anchorDate
	}

	hasRedep := false
	hasFTD := false
	var lastRedep time.Time
	for _, d := range deposits {
		switch d.Type {
		case event.CpaAccept:
			hasFTD = true
		case event.CpaRedep:
			hasRedep = true
			a.redepEventCount++
			if d.EventAt.After(lastRedep) {
				lastRedep = d.EventAt
			}
		default:
			continue // CPA_HOLD never contributes to deposit sums/windows
		}

		usd := 0.0
		if d.HasUSDValue {
			usd = d.USDValue
		}
		a.depositCount++
		a.depositRevenue += usd

		offset := daysBetween(anchorDate, d.EventAt)
		if offset < 0 {
			continue // a deposit can't precede its own anchor; defensive, not expected
		}
		if w, ok := windowForOffset(offset); ok {
			a.windowRevenue[w] += usd
		}
	}
	if hasFTD {
		a.ftdCount++
	}
	if hasRedep {
		a.redepUnique++
		a.lifetimeDaysSum += float64(daysBetween(anchorDate, lastRedep))
		a.lifetimeCount++
	}
}

func (a *accumulator) toBase(asOf time.Time) base {
	windows := make(map[WindowKey]WindowResult, len(Windows))
	var total float64
	for _, w := range Windows {
		b := windowBounds[w]
		complete := daysBetween(a.maxAnchorDate, asOf) > b.maxDay
		rev := a.windowRevenue[w]
		windows[w] = WindowResult{RevenueUSD: rev, Complete: complete}
		total += rev
	}

	b := base{
		Key:                    a.key,
		AnchorCount:            a.anchorCount,
		Windows:                windows,
		LTVTotalUSD:            total,
		RedepUniqueCount:       a.redepUnique,
		TotalDeposits:          a.depositCount,
		TotalDepositRevenueUSD: a.depositRevenue,
	}
	if a.anchorCount > 0 {
		b.LTVPerAnchorUSD = total / float64(a.anchorCount)
	}
	if a.lifetimeCount > 0 {
		b.HasLifetimeData = true
		b.LifetimeDaysAvg = a.lifetimeDaysSum / float64(a.lifetimeCount)
	}
	return b
}

// daysBetween is whole calendar days from a to b, in a's own location
// (both inputs are expected UTC — chstore always returns UTC timestamps).
// Truncating to the date before subtracting is what makes "day 0" mean
// "the same calendar day as the anchor," not "within 24 exact hours" —
// matching §26.5's "day 0/day 1-7" language.
func daysBetween(a, b time.Time) int {
	ad := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	bd := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(bd.Sub(ad).Hours() / 24)
}

// ComputeFTDCohorts groups histories (each already guaranteed to have a
// CPA_ACCEPT — chstore.ClicksByFTDAnchor's contract) by (period bucket of
// its CPA_ACCEPT date, campaign, network, country) and computes every
// §26.5 metric for each group. asOf is the reference "now" for window
// completeness — pass time.Now().UTC() in production, a fixed instant in
// tests.
func ComputeFTDCohorts(histories []chstore.ClickHistory, period CohortPeriod, asOf time.Time) []FTDCohort {
	accs := map[CohortKey]*accumulator{}
	var order []CohortKey

	for _, h := range histories {
		anchorDate, ok := firstOf(h.Deposits, event.CpaAccept)
		if !ok {
			continue // defensive: ClicksByFTDAnchor's contract guarantees this shouldn't happen
		}
		key := CohortKey{Period: period, Bucket: bucket(period, anchorDate), CampaignID: h.CampaignID, NetworkID: h.NetworkID, Country: h.Country}
		acc, exists := accs[key]
		if !exists {
			acc = newAccumulator(key)
			accs[key] = acc
			order = append(order, key)
		}
		acc.addClick(anchorDate, h.Deposits)
	}

	out := make([]FTDCohort, 0, len(order))
	for _, key := range order {
		acc := accs[key]
		b := acc.toBase(asOf)
		c := FTDCohort{base: b}
		if acc.anchorCount > 0 {
			c.FTDToRedepRate = float64(acc.redepUnique) / float64(acc.anchorCount)
			c.DepToRedepRate = float64(acc.redepEventCount) / float64(acc.anchorCount)
		}
		out = append(out, c)
	}
	return out
}

// ComputeRegCohorts is ComputeFTDCohorts' Reg-cohort counterpart, anchored
// on each history's CPA_HOLD instead (chstore.ClicksByRegAnchor's
// contract).
func ComputeRegCohorts(histories []chstore.ClickHistory, period CohortPeriod, asOf time.Time) []RegCohort {
	accs := map[CohortKey]*accumulator{}
	var order []CohortKey

	for _, h := range histories {
		anchorDate, ok := firstOf(h.Deposits, event.CpaHold)
		if !ok {
			continue
		}
		key := CohortKey{Period: period, Bucket: bucket(period, anchorDate), CampaignID: h.CampaignID, NetworkID: h.NetworkID, Country: h.Country}
		acc, exists := accs[key]
		if !exists {
			acc = newAccumulator(key)
			accs[key] = acc
			order = append(order, key)
		}
		acc.addClick(anchorDate, h.Deposits)
	}

	out := make([]RegCohort, 0, len(order))
	for _, key := range order {
		acc := accs[key]
		b := acc.toBase(asOf)
		c := RegCohort{base: b}
		if acc.anchorCount > 0 {
			c.RegToFTDRate = float64(acc.ftdCount) / float64(acc.anchorCount)
		}
		out = append(out, c)
	}
	return out
}

// firstOf returns the (only, per the dedup-key invariant documented on
// chstore.ClicksByFTDAnchor) deposit of typ in deposits.
func firstOf(deposits []chstore.Deposit, typ event.Type) (time.Time, bool) {
	for _, d := range deposits {
		if d.Type == typ {
			return d.EventAt, true
		}
	}
	return time.Time{}, false
}
