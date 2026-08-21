package conversion_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func mustRedis(t *testing.T) *redis.Client {
	t.Helper()
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL not set, skipping integration test")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("parsing REDIS_URL: %v", err)
	}
	client := redis.NewClient(opts)
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("pinging redis: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestRedisStoreReadsThroughToPostgresOnMiss proves RedisStore is correct
// even starting from an empty cache: the first LastStatus call for a click
// falls through to Postgres and gets the right answer.
func TestRedisStoreReadsThroughToPostgresOnMiss(t *testing.T) {
	pool := mustPool(t)
	rdb := mustRedis(t)
	ctx := context.Background()

	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID, false)
	clickID := idgen.New()

	pg := conversion.NewPostgresStore(pool)
	store := conversion.NewRedisStore(pg, rdb, quietLogger())

	if _, ok, err := store.LastStatus(ctx, orgID, clickID); err != nil || ok {
		t.Fatalf("LastStatus for an unknown click: ok=%v err=%v, want false nil", ok, err)
	}

	if _, _, err := store.Record(ctx, conversion.Entry{
		OrganizationID: orgID, NetworkID: networkID, ClickID: clickID,
		Status: event.CpaHold, Kind: conversion.ResultSuccess,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Populated by Record — must not need a Postgres round trip to answer
	// correctly.
	status, ok, err := store.LastStatus(ctx, orgID, clickID)
	if err != nil || !ok || status != event.CpaHold {
		t.Fatalf("LastStatus after Record: status=%q ok=%v err=%v, want CPA_HOLD true nil", status, ok, err)
	}

	// A fresh RedisStore over the SAME Postgres pool, cache cold: proves
	// the read-through path (not just the write-through one) is correct.
	cold := conversion.NewRedisStore(conversion.NewPostgresStore(pool), rdb, quietLogger())
	// Bypass the now-warm cache key by using a second click that only
	// Postgres knows about.
	clickID2 := idgen.New()
	if _, _, err := pg.Record(ctx, conversion.Entry{
		OrganizationID: orgID, NetworkID: networkID, ClickID: clickID2,
		Status: event.CpaAccept, Kind: conversion.ResultSuccess,
	}); err != nil {
		t.Fatalf("seeding via bare PostgresStore: %v", err)
	}
	status2, ok2, err2 := cold.LastStatus(ctx, orgID, clickID2)
	if err2 != nil || !ok2 || status2 != event.CpaAccept {
		t.Fatalf("cold cache read-through: status=%q ok=%v err=%v, want CPA_ACCEPT true nil", status2, ok2, err2)
	}
}

// TestRedisStoreFallsThroughWhenRedisUnavailable is §45's explicit rule:
// "REDIS UNAVAILABLE: fall through and record the event." A closed client
// stands in for an outage — every call must fail fast, and RedisStore must
// still answer correctly from Postgres alone.
func TestRedisStoreFallsThroughWhenRedisUnavailable(t *testing.T) {
	pool := mustPool(t)
	rdb := mustRedis(t)
	ctx := context.Background()

	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID, false)
	clickID := idgen.New()

	pg := conversion.NewPostgresStore(pool)
	if _, _, err := pg.Record(ctx, conversion.Entry{
		OrganizationID: orgID, NetworkID: networkID, ClickID: clickID,
		Status: event.CpaHold, Kind: conversion.ResultSuccess,
	}); err != nil {
		t.Fatalf("seeding via bare PostgresStore: %v", err)
	}

	_ = rdb.Close() // simulate Redis being down for the rest of this test
	broken := conversion.NewRedisStore(pg, rdb, quietLogger())

	status, ok, err := broken.LastStatus(ctx, orgID, clickID)
	if err != nil || !ok || status != event.CpaHold {
		t.Fatalf("LastStatus with Redis down: status=%q ok=%v err=%v, want CPA_HOLD true nil (fall through to Postgres)", status, ok, err)
	}

	// Recording must also still succeed — the cache-repopulate step after
	// a successful write is best-effort and must not fail the request.
	clickID2 := idgen.New()
	id, kind, err := broken.Record(ctx, conversion.Entry{
		OrganizationID: orgID, NetworkID: networkID, ClickID: clickID2,
		Status: event.CpaHold, Kind: conversion.ResultSuccess,
	})
	if err != nil || kind != conversion.ResultSuccess || id == "" {
		t.Fatalf("Record with Redis down: id=%q kind=%q err=%v, want a real id, success, nil", id, kind, err)
	}
}
