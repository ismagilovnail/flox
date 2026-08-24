package conversion_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// TestPostgresStoreDedupAndProgression runs the same money-correctness
// rules as conversion_test.go's fakeStore-backed tests, but against a real
// Postgres and the actual migration 00013 constraint — the fake enforces
// the CONTRACT, this proves the SCHEMA actually implements it.
func TestPostgresStoreDedupAndProgression(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID, false)

	store := conversion.NewPostgresStore(pool)
	clickID := idgen.New()

	success := func(status event.Type, eventRef string) conversion.ResultKind {
		_, kind, err := store.Record(ctx, conversion.Entry{
			OrganizationID: orgID, NetworkID: networkID, ClickID: clickID,
			Status: status, EventRef: eventRef, Kind: conversion.ResultSuccess,
		})
		if err != nil {
			t.Fatalf("Record(%s, %q): %v", status, eventRef, err)
		}
		return kind
	}

	if kind := success(event.CpaHold, ""); kind != conversion.ResultSuccess {
		t.Fatalf("HOLD: kind = %q, want success", kind)
	}
	if kind := success(event.CpaAccept, ""); kind != conversion.ResultSuccess {
		t.Fatalf("ACCEPT: kind = %q, want success (click_id-alone dedup would have dropped this)", kind)
	}
	if kind := success(event.CpaRedep, "txn-1"); kind != conversion.ResultSuccess {
		t.Fatalf("first REDEP: kind = %q, want success", kind)
	}
	if kind := success(event.CpaRedep, "txn-2"); kind != conversion.ResultSuccess {
		t.Fatalf("second REDEP: kind = %q, want success ((click_id,status) dedup would have dropped this)", kind)
	}
	if kind := success(event.CpaRedep, "txn-1"); kind != conversion.ResultDuplicate {
		t.Fatalf("replayed REDEP: kind = %q, want duplicate", kind)
	}

	last, ok, err := store.LastStatus(ctx, orgID, clickID)
	if err != nil {
		t.Fatalf("LastStatus: %v", err)
	}
	if !ok || last != event.CpaRedep {
		t.Fatalf("LastStatus = (%q, %v), want (CPA_REDEP, true)", last, ok)
	}
}

// TestPostgresStoreFindSuccessID exercises outgoing replay's one lookup
// (apps/internal/postbacklogs.Service.ReplayOutgoing) against the real
// schema: it must resolve the exact success row for a given event_ref, not
// just any success row for the (network, click, status) triple, and it
// must never see a different tenant's row.
func TestPostgresStoreFindSuccessID(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	otherOrgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID, false)
	store := conversion.NewPostgresStore(pool)
	clickID := idgen.New()

	if _, _, err := store.Record(ctx, conversion.Entry{
		OrganizationID: orgID, NetworkID: networkID, ClickID: clickID,
		Status: event.CpaRedep, EventRef: "txn-1", Kind: conversion.ResultSuccess,
	}); err != nil {
		t.Fatalf("Record txn-1: %v", err)
	}
	id2, _, err := store.Record(ctx, conversion.Entry{
		OrganizationID: orgID, NetworkID: networkID, ClickID: clickID,
		Status: event.CpaRedep, EventRef: "txn-2", Kind: conversion.ResultSuccess,
	})
	if err != nil {
		t.Fatalf("Record txn-2: %v", err)
	}
	if _, _, err := store.Record(ctx, conversion.Entry{
		OrganizationID: orgID, NetworkID: networkID, ClickID: clickID,
		Status: event.Type(""), Kind: conversion.ResultError, Message: "no event mapping",
	}); err != nil {
		t.Fatalf("Record error row: %v", err)
	}

	got, found, err := store.FindSuccessID(ctx, orgID, networkID, clickID, string(event.CpaRedep), "txn-2")
	if err != nil {
		t.Fatalf("FindSuccessID: %v", err)
	}
	if !found || got != id2 {
		t.Fatalf("FindSuccessID(txn-2) = (%q, %v), want (%q, true) — must resolve the txn-2 row, not txn-1's", got, found, id2)
	}

	if _, found, err := store.FindSuccessID(ctx, orgID, networkID, clickID, string(event.CpaRedep), "txn-does-not-exist"); err != nil {
		t.Fatalf("FindSuccessID(unknown event_ref): %v", err)
	} else if found {
		t.Fatal("FindSuccessID(unknown event_ref): want not found")
	}

	if _, found, err := store.FindSuccessID(ctx, orgID, networkID, clickID, string(event.CpaAccept), ""); err != nil {
		t.Fatalf("FindSuccessID(never-recorded status): %v", err)
	} else if found {
		t.Fatal("FindSuccessID(never-recorded status, only an error row exists): want not found")
	}

	if _, found, err := store.FindSuccessID(ctx, otherOrgID, networkID, clickID, string(event.CpaRedep), "txn-2"); err != nil {
		t.Fatalf("FindSuccessID(wrong org): %v", err)
	} else if found {
		t.Fatal("FindSuccessID(wrong org): want not found — tenant isolation")
	}
}

func TestPostgresStoreAcceptDuplicatesBypassesDedupNotProgression(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID, true) // accept_duplicates
	store := conversion.NewPostgresStore(pool)
	clickID := idgen.New()

	record := func(status event.Type) (string, conversion.ResultKind) {
		id, kind, err := store.Record(ctx, conversion.Entry{
			OrganizationID: orgID, NetworkID: networkID, ClickID: clickID,
			Status: status, AcceptDuplicates: true, Kind: conversion.ResultSuccess,
		})
		if err != nil {
			t.Fatalf("Record(%s): %v", status, err)
		}
		return id, kind
	}

	id1, kind1 := record(event.CpaHold)
	id2, kind2 := record(event.CpaHold)
	if kind1 != conversion.ResultSuccess || kind2 != conversion.ResultSuccess {
		t.Fatalf("accept_duplicates network: kinds = %q, %q, want success, success", kind1, kind2)
	}
	if id1 == id2 {
		t.Fatal("two distinct rows expected for an accept_duplicates network")
	}
}

func TestPostgresStoreTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	networkA := seedNetwork(t, ctx, pool, orgA, false)
	clickID := idgen.New()

	store := conversion.NewPostgresStore(pool)
	if _, _, err := store.Record(ctx, conversion.Entry{
		OrganizationID: orgA, NetworkID: networkA, ClickID: clickID,
		Status: event.CpaHold, Kind: conversion.ResultSuccess,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	_, ok, err := store.LastStatus(ctx, orgB, clickID)
	if err != nil {
		t.Fatalf("LastStatus: %v", err)
	}
	if ok {
		t.Fatal("org B saw org A's click status, want none")
	}
}

func TestPostgresNetworkLookup(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID, true)

	lookup := conversion.NewPostgresNetworkLookup(pool)
	n, err := lookup.ByID(ctx, networkID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if n.OrganizationID != orgID || !n.AcceptDuplicates {
		t.Fatalf("Network = %+v, want org %q with accept_duplicates=true", n, orgID)
	}

	_, err = lookup.ByID(ctx, idgen.New())
	if !errors.Is(err, conversion.ErrNetworkNotFound) {
		t.Fatalf("unknown network id: err = %v, want ErrNetworkNotFound", err)
	}
}

func TestPostgresMapperCaseInsensitive(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID, false)
	seedEventMapping(t, ctx, pool, orgID, networkID, "Sale", event.CpaAccept)

	mapper := conversion.NewPostgresMapper(pool)
	status, err := mapper.MapStatus(ctx, orgID, networkID, "sale")
	if err != nil {
		t.Fatalf("MapStatus (lowercase lookup of 'Sale' mapping): %v", err)
	}
	if status != event.CpaAccept {
		t.Fatalf("status = %q, want CPA_ACCEPT", status)
	}

	if _, err := mapper.MapStatus(ctx, orgID, networkID, "unknown"); !errors.Is(err, conversion.ErrUnmapped) {
		t.Fatalf("unmapped status: err = %v, want ErrUnmapped", err)
	}
}

func TestPostgresFX(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	fx := conversion.NewPostgresFX(pool)
	at := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)

	if usd, ok, err := fx.ToUSD(ctx, "USD", 42.5, at); err != nil || !ok || usd != 42.5 {
		t.Fatalf("USD passthrough: usd=%v ok=%v err=%v", usd, ok, err)
	}

	seedFXRate(t, ctx, pool, "EUR", at, 1.1)
	usd, ok, err := fx.ToUSD(ctx, "EUR", 100, at)
	if err != nil || !ok || usd < 109.99 || usd > 110.01 {
		t.Fatalf("EUR on rate date: usd=%v ok=%v err=%v, want ~110 true nil", usd, ok, err)
	}

	// No rate on file for the day before: never invent a rate.
	dayBefore := at.AddDate(0, 0, -1)
	if usd, ok, err := fx.ToUSD(ctx, "EUR", 100, dayBefore); err != nil || ok {
		t.Fatalf("no rate for that date: usd=%v ok=%v err=%v, want ok=false err=nil", usd, ok, err)
	}
}

func seedNetwork(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID string, acceptDuplicates bool) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO networks (id, organization_id, name, accept_duplicates) VALUES ($1, $2, $3, $4)`,
		id, orgID, "Test Network", acceptDuplicates,
	)
	if err != nil {
		t.Fatalf("seeding network: %v", err)
	}
	return id
}

func seedEventMapping(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID, networkID, networkStatus string, floxStatus event.Type) {
	t.Helper()
	_, err := pool.Exec(ctx,
		`INSERT INTO event_mappings (id, organization_id, network_id, network_status, flox_status) VALUES ($1, $2, $3, $4, $5)`,
		idgen.New(), orgID, networkID, networkStatus, string(floxStatus),
	)
	if err != nil {
		t.Fatalf("seeding event mapping: %v", err)
	}
}

func seedFXRate(t *testing.T, ctx context.Context, pool *pgxpool.Pool, currency string, at time.Time, rate float64) {
	t.Helper()
	// fx_rates is not tenant-scoped (it's a shared market fact, per
	// migrations/README's "Conventions" note), so tests clean up their own
	// rows rather than relying on an organization cascade.
	date := at.UTC().Format("2006-01-02")
	_, err := pool.Exec(ctx,
		`INSERT INTO fx_rates (currency, rate_date, rate_to_usd) VALUES ($1, $2, $3)
		 ON CONFLICT (currency, rate_date) DO UPDATE SET rate_to_usd = EXCLUDED.rate_to_usd`,
		currency, date, rate,
	)
	if err != nil {
		t.Fatalf("seeding fx rate: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM fx_rates WHERE currency = $1 AND rate_date = $2`, currency, date)
	})
}

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

func seedOrg(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, id, "Test Org "+id)
	if err != nil {
		t.Fatalf("seeding organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, id)
	})
	return id
}
