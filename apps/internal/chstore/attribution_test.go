package chstore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func TestClickResolverByClickID(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	resolver := chstore.NewClickResolver(conn)
	ctx := context.Background()

	orgID := idgen.New()
	clickID := idgen.New()
	campaignID := idgen.New()
	occurredAt := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)

	err := store.InsertBatch(ctx, []event.Event{{
		Type: event.SourceClick, EventAt: occurredAt, OrganizationID: orgID, CampaignID: campaignID,
		ClickID: clickID, Country: "US", Device: "mobile", ExternalClickID: "fb.123",
		UTMSource: "facebook", Subs: event.Subs{"a", "b"},
	}})
	if err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	click, err := resolver.ByClickID(ctx, orgID, clickID)
	if err != nil {
		t.Fatalf("ByClickID: %v", err)
	}
	if click.ClickID != clickID || click.CampaignID != campaignID || click.Country != "US" || click.Device != "mobile" {
		t.Fatalf("click = %+v, want matching the inserted row", click)
	}
	if click.ExternalClickID != "fb.123" || click.UTMSource != "facebook" || click.Subs[0] != "a" || click.Subs[1] != "b" {
		t.Fatalf("click pass-through fields wrong: %+v", click)
	}
	if !click.OccurredAt.Equal(occurredAt) {
		t.Fatalf("OccurredAt = %v, want %v", click.OccurredAt, occurredAt)
	}
}

func TestClickResolverByClickIDNotFound(t *testing.T) {
	conn := mustConn(t)
	resolver := chstore.NewClickResolver(conn)

	_, err := resolver.ByClickID(context.Background(), idgen.New(), idgen.New())
	if !errors.Is(err, attribution.ErrClickNotFound) {
		t.Fatalf("err = %v, want ErrClickNotFound", err)
	}
}

// TestClickResolverExcludesFilteredClicks: a SOURCE_FILTER row never
// reached a destination, so it must never resolve as an attributable click
// — see attribution.go's comment on clickColumns' type filter.
func TestClickResolverExcludesFilteredClicks(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	resolver := chstore.NewClickResolver(conn)
	ctx := context.Background()

	orgID := idgen.New()
	clickID := idgen.New()
	if err := store.InsertBatch(ctx, []event.Event{{
		Type: event.SourceFilter, EventAt: time.Now().UTC(), OrganizationID: orgID, ClickID: clickID, FilterReason: "bot",
	}}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	_, err := resolver.ByClickID(ctx, orgID, clickID)
	if !errors.Is(err, attribution.ErrClickNotFound) {
		t.Fatalf("err = %v, want ErrClickNotFound for a filtered click", err)
	}
}

// TestClickResolverByClickIDPicksEarliest: stickyFlowKeepClickId reuses one
// click_id across a returning visitor's whole journey (§39-STICKY) — the
// resolver must resolve to the ORIGINAL click, not the most recent one.
func TestClickResolverByClickIDPicksEarliest(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	resolver := chstore.NewClickResolver(conn)
	ctx := context.Background()

	orgID := idgen.New()
	clickID := idgen.New()
	first := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	second := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)

	err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: second, OrganizationID: orgID, ClickID: clickID, CampaignID: "second-visit-campaign"},
		{Type: event.SourceClick, EventAt: first, OrganizationID: orgID, ClickID: clickID, CampaignID: "first-visit-campaign"},
	})
	if err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	click, err := resolver.ByClickID(ctx, orgID, clickID)
	if err != nil {
		t.Fatalf("ByClickID: %v", err)
	}
	if click.CampaignID != "first-visit-campaign" {
		t.Fatalf("resolved campaign = %q, want the earliest visit's campaign", click.CampaignID)
	}
}

func TestClickResolverByExternalClickID(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	resolver := chstore.NewClickResolver(conn)
	ctx := context.Background()

	orgID := idgen.New()
	externalID := "fbclid." + idgen.New()

	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: time.Now().UTC(), OrganizationID: orgID, ClickID: idgen.New(), ExternalClickID: externalID},
	}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	clicks, err := resolver.ByExternalClickID(ctx, orgID, externalID)
	if err != nil {
		t.Fatalf("ByExternalClickID: %v", err)
	}
	if len(clicks) != 1 {
		t.Fatalf("clicks = %d, want exactly 1", len(clicks))
	}

	// A second click sharing the same external_click_id makes it ambiguous
	// at the attribution layer — this resolver's job is only to report
	// every match, not to pick one.
	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: time.Now().UTC(), OrganizationID: orgID, ClickID: idgen.New(), ExternalClickID: externalID},
	}); err != nil {
		t.Fatalf("InsertBatch (second): %v", err)
	}
	clicks, err = resolver.ByExternalClickID(ctx, orgID, externalID)
	if err != nil {
		t.Fatalf("ByExternalClickID (second): %v", err)
	}
	if len(clicks) != 2 {
		t.Fatalf("clicks = %d, want exactly 2", len(clicks))
	}
}

func TestClickResolverByExternalClickIDNoMatch(t *testing.T) {
	conn := mustConn(t)
	resolver := chstore.NewClickResolver(conn)

	clicks, err := resolver.ByExternalClickID(context.Background(), idgen.New(), "nonexistent")
	if err != nil {
		t.Fatalf("ByExternalClickID: %v", err)
	}
	if len(clicks) != 0 {
		t.Fatalf("clicks = %d, want 0", len(clicks))
	}
}

func TestClickResolverTenantIsolation(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	resolver := chstore.NewClickResolver(conn)
	ctx := context.Background()

	orgA := idgen.New()
	orgB := idgen.New()
	clickID := idgen.New()

	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: time.Now().UTC(), OrganizationID: orgA, ClickID: clickID},
	}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	_, err := resolver.ByClickID(ctx, orgB, clickID)
	if !errors.Is(err, attribution.ErrClickNotFound) {
		t.Fatalf("org B resolving org A's click: err = %v, want ErrClickNotFound", err)
	}
}
