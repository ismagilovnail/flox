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
			INSERT INTO postbacks (id, organization_id, network_id, click_id, status, direction, result, revenue, currency, created_at)
			VALUES ($1, $2, $3, $4, 'CPA_ACCEPT', 'incoming', 'success', 10, $5, $6)`,
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
