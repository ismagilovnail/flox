package fx_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/fx"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func mustPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connecting to database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// XTS/XXX are ISO 4217 codes reserved for testing / "no currency": no
// provider publishes them, so the dev worker's real sync cannot interfere.
const testCurrency = "XTS"

func TestUpsertAndRepairUSD(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	store := fx.NewStore(pool)

	orgID := idgen.New()
	mustExec(t, pool, `INSERT INTO organizations (id, name) VALUES ($1, 'fx test')`, orgID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM fx_rates WHERE currency = $1`, testCurrency)
	})
	networkID, sourceID, campaignID := idgen.New(), idgen.New(), idgen.New()
	mustExec(t, pool, `INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, 'n')`, networkID, orgID)
	mustExec(t, pool, `INSERT INTO traffic_sources (id, organization_id, name, type) VALUES ($1, $2, 's', 'Facebook')`, sourceID, orgID)
	mustExec(t, pool, `INSERT INTO campaigns (id, organization_id, traffic_source_id, name, fallback_url) VALUES ($1, $2, $3, 'c', 'https://example.com')`,
		campaignID, orgID, sourceID)

	// A Friday rate; events on the following Monday must use it.
	friday := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	if err := store.Upsert(ctx, []fx.Rate{{Currency: testCurrency, Date: friday, RateToUSD: 2}}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	// Re-publishing a date replaces the rate.
	if err := store.Upsert(ctx, []fx.Rate{{Currency: testCurrency, Date: friday, RateToUSD: 2.5}}); err != nil {
		t.Fatalf("Upsert (again): %v", err)
	}

	seedPostback := func(currency string, at time.Time) string {
		id := idgen.New()
		mustExec(t, pool, `
			INSERT INTO postbacks (id, organization_id, network_id, click_id, status, direction, result, revenue, currency, created_at, event_at)
			VALUES ($1, $2, $3, $4, 'CPA_ACCEPT', 'incoming', 'success', 10, $5, $6, $6)`,
			id, orgID, networkID, idgen.New(), currency, at)
		return id
	}
	monday := friday.AddDate(0, 0, 3).Add(9 * time.Hour)
	inWindow := seedPostback(testCurrency, monday)
	usdLower := seedPostback("usd", monday)
	tooOld := seedPostback(testCurrency, friday.AddDate(0, 0, fx.MaxStaleDays).Add(time.Hour))
	beforeRate := seedPostback(testCurrency, friday.AddDate(0, 0, -1))
	unknown := seedPostback("XXX", monday)

	costID := idgen.New()
	mustExec(t, pool, `
		INSERT INTO cost_entries (id, organization_id, campaign_id, traffic_source_id, entry_date, amount, currency)
		VALUES ($1, $2, $3, $4, $5, 40, $6)`,
		costID, orgID, campaignID, sourceID, monday.Format(time.DateOnly), testCurrency)

	if oldest, ok, err := store.OldestMissingDate(ctx); err != nil || !ok || oldest.After(friday.AddDate(0, 0, -1)) {
		t.Fatalf("OldestMissingDate = %v %v %v, want on or before %s", oldest, ok, err, friday.AddDate(0, 0, -1).Format(time.DateOnly))
	}

	if _, err := store.RepairUSD(ctx); err != nil {
		t.Fatalf("RepairUSD: %v", err)
	}

	usd := func(table, id string) *float64 {
		var v *float64
		col := "usd_value"
		if table == "cost_entries" {
			col = "amount_usd"
		}
		if err := pool.QueryRow(ctx, `SELECT `+col+`::float8 FROM `+table+` WHERE id = $1`, id).Scan(&v); err != nil {
			t.Fatalf("reading %s %s: %v", table, id, err)
		}
		return v
	}
	for name, tc := range map[string]struct {
		table, id string
		want      *float64
	}{
		"weekend uses Friday's rate": {"postbacks", inWindow, ptr(25)},
		"lower-case usd is 1:1":      {"postbacks", usdLower, ptr(10)},
		"rate older than 7 days":     {"postbacks", tooOld, nil},
		"event before any rate":      {"postbacks", beforeRate, nil},
		"currency nobody publishes":  {"postbacks", unknown, nil},
		"cost in a foreign currency": {"cost_entries", costID, ptr(100)},
	} {
		got := usd(tc.table, tc.id)
		switch {
		case tc.want == nil && got != nil:
			t.Errorf("%s: usd = %v, want NULL (no rate is never zero)", name, *got)
		case tc.want != nil && (got == nil || *got != *tc.want):
			t.Errorf("%s: usd = %v, want %v", name, got, *tc.want)
		}
	}
}

func ptr(v float64) *float64 { return &v }

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// The rate is the one on the conversion's event date (event_at, the
// network's event_time), as on the live path — not the day the postback
// arrived. A decline that reversed an accept with no USD value gets it once
// the accept does. Only recorded incoming conversions are buckets to
// rewrite in ClickHouse.
func TestRepairUSDByEventTimeWithReversalsAndBuckets(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	store := fx.NewStore(pool)

	orgID, networkID := idgen.New(), idgen.New()
	mustExec(t, pool, `INSERT INTO organizations (id, name) VALUES ($1, 'fx event time test')`, orgID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM fx_rates WHERE currency = $1`, testCurrency)
	})
	mustExec(t, pool, `INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, 'n')`, networkID, orgID)

	// A rate only on the event date, 20 days before the postback arrived.
	eventDay := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	arrived := eventDay.AddDate(0, 0, 20).Add(10 * time.Hour)
	if err := store.Upsert(ctx, []fx.Rate{{Currency: testCurrency, Date: eventDay, RateToUSD: 3}}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	clickID := idgen.New()
	accept, decline, dup := idgen.New(), idgen.New(), idgen.New()
	mustExec(t, pool, `
		INSERT INTO postbacks (id, organization_id, network_id, click_id, status, direction, result, revenue, currency, created_at, event_at)
		VALUES ($1, $2, $3, $4, 'CPA_ACCEPT', 'incoming', 'success', 10, $5, $6, $7)`,
		accept, orgID, networkID, clickID, testCurrency, arrived, eventDay.Add(9*time.Hour))
	mustExec(t, pool, `
		INSERT INTO postbacks (id, organization_id, network_id, click_id, status, direction, result, created_at, event_at,
		                       reversal_revenue, reversal_currency)
		VALUES ($1, $2, $3, $4, 'CPA_DECLINE', 'incoming', 'success', $5, $5, 10, $6)`,
		decline, orgID, networkID, clickID, arrived.Add(time.Hour), testCurrency)
	mustExec(t, pool, `
		INSERT INTO postbacks (id, organization_id, network_id, click_id, status, direction, result, revenue, currency, created_at, event_at)
		VALUES ($1, $2, $3, $4, 'CPA_ACCEPT', 'incoming', 'duplicate', 10, $5, $6, $7)`,
		dup, orgID, networkID, clickID, testCurrency, arrived, eventDay.Add(9*time.Hour))

	repaired, err := store.RepairUSD(ctx)
	if err != nil {
		t.Fatalf("RepairUSD: %v", err)
	}

	var acceptUSD, reversalUSD *float64
	if err := pool.QueryRow(ctx, `SELECT usd_value::float8 FROM postbacks WHERE id = $1`, accept).Scan(&acceptUSD); err != nil {
		t.Fatal(err)
	}
	if acceptUSD == nil || *acceptUSD != 30 {
		t.Fatalf("accept usd = %v, want 30 (the event date's rate, not the arrival's)", acceptUSD)
	}
	if err := pool.QueryRow(ctx, `SELECT reversal_usd_value::float8 FROM postbacks WHERE id = $1`, decline).Scan(&reversalUSD); err != nil {
		t.Fatal(err)
	}
	if reversalUSD == nil || *reversalUSD != 30 {
		t.Fatalf("decline reversal usd = %v, want 30 (the accept's value, so they net to zero)", reversalUSD)
	}

	var mine []fx.Bucket
	for _, b := range repaired.Buckets {
		if b.OrganizationID == orgID {
			mine = append(mine, b)
		}
	}
	want := []fx.Bucket{
		{OrganizationID: orgID, Day: "2026-09-01", Status: "CPA_ACCEPT"},
		{OrganizationID: orgID, Day: arrived.Format(time.DateOnly), Status: "CPA_DECLINE"},
	}
	if len(mine) != len(want) || mine[0] != want[0] || mine[1] != want[1] {
		t.Fatalf("buckets = %+v, want %+v (one per recorded conversion, by event day; not the duplicate)", mine, want)
	}
}
