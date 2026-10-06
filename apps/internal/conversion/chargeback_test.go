package conversion_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// eventProbe captures, inside the record's own transaction, the CPA event
// the outbox queued — the dev-stack worker cannot take it first.
type eventProbe struct {
	mu   sync.Mutex
	last event.Event
}

func (p *eventProbe) InsertTx(ctx context.Context, tx pgx.Tx, req conversion.DeliveryRequest) error {
	var payload []byte
	if err := tx.QueryRow(ctx, `
		SELECT payload FROM event_queue WHERE payload->>'clickId' = $1 AND type = $2
		ORDER BY created_at DESC, id DESC LIMIT 1`, req.ClickID, string(req.Status)).Scan(&payload); err != nil {
		return err
	}
	// A fresh value: zero fields are omitted from the payload and must not
	// inherit the previous event's.
	var ev event.Event
	if err := json.Unmarshal(payload, &ev); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.last = ev
	return nil
}

// Audit 2026-10-06, T4/AN3 — §45 "chargeback (ACCEPT then DECLINE → recorded,
// revenue reversed)" and "chargeback undone (DECLINE then ACCEPT →
// recorded)".
func TestChargebackReversesTheAcceptOnly(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID, false)
	probe := &eventProbe{}
	store := conversion.NewPostgresStore(pool).WithDeliveries(probe)

	record := func(clickID string, status event.Type, eventRef string, revenue float64) conversion.Recorded {
		t.Helper()
		ev := event.Event{Type: status, OrganizationID: orgID, ClickID: clickID, NetworkID: networkID, EventAt: time.Now().UTC()}
		if status != event.CpaDecline && status != event.CpaTrash {
			ev.Revenue, ev.Currency, ev.USDValue, ev.HasUSDValue = revenue, "EUR", revenue*1.1, true
		}
		usd := revenue * 1.1
		rec, err := store.Record(ctx, conversion.Entry{
			OrganizationID: orgID, NetworkID: networkID, ClickID: clickID, Status: status, EventRef: eventRef,
			Revenue: &revenue, Currency: "EUR", USDValue: &usd, Kind: conversion.ResultSuccess,
			Event:    &ev,
			Delivery: &conversion.DeliveryRequest{OrganizationID: orgID, NetworkID: networkID, ClickID: clickID, Status: status, URL: "https://x.example"},
		})
		if err != nil {
			t.Fatalf("Record(%s): %v", status, err)
		}
		return rec
	}
	reversal := func(id string) (revenue, usd *float64, currency string) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT reversal_revenue::float8, reversal_usd_value::float8, reversal_currency FROM postbacks WHERE id = $1`, id).
			Scan(&revenue, &usd, &currency); err != nil {
			t.Fatalf("reading reversal: %v", err)
		}
		return revenue, usd, currency
	}

	// ACCEPT 25 EUR, a 10 EUR redeposit, then the chargeback.
	click := idgen.New()
	record(click, event.CpaAccept, "", 25)
	record(click, event.CpaRedep, "txn-1", 10)
	decline := record(click, event.CpaDecline, "", 0)
	if decline.Kind != conversion.ResultSuccess {
		t.Fatalf("decline: %s, want success", decline.Kind)
	}
	if rev, usd, cur := reversal(decline.ID); rev == nil || *rev != 25 || usd == nil || *usd < 27.49 || *usd > 27.51 || cur != "EUR" {
		t.Fatalf("decline reversal = %v %v %q, want the accept's 25 EUR / 27.5 USD — not the redeposit", rev, usd, cur)
	}
	if ev := probe.last; ev.Revenue != -25 || ev.USDValue > -27.49 || ev.USDValue < -27.51 || !ev.HasUSDValue || ev.Currency != "EUR" {
		t.Fatalf("decline event = revenue %v usd %v (has %v) %q, want -25 / -27.5 EUR", ev.Revenue, ev.USDValue, ev.HasUSDValue, ev.Currency)
	}

	// A re-sent decline is a duplicate; a TRASH after it reverses nothing.
	if again := record(click, event.CpaDecline, "", 0); again.Kind != conversion.ResultDuplicate {
		t.Fatalf("second decline: %s, want duplicate (no second reversal)", again.Kind)
	}
	trash := record(click, event.CpaTrash, "", 0)
	if rev, _, _ := reversal(trash.ID); rev != nil {
		t.Fatalf("trash after a decline reversed %v, want nothing", *rev)
	}
	if ev := probe.last; ev.Revenue != 0 || ev.HasUSDValue {
		t.Fatalf("trash event revenue = %v (has usd %v), want 0", ev.Revenue, ev.HasUSDValue)
	}

	// Declined first, approved later: the accept counts, nothing reversed.
	other := idgen.New()
	first := record(other, event.CpaDecline, "", 0)
	if rev, _, _ := reversal(first.ID); rev != nil {
		t.Fatalf("a decline with no accept before it reversed %v", *rev)
	}
	if acc := record(other, event.CpaAccept, "", 25); acc.Kind != conversion.ResultSuccess || probe.last.Revenue != 25 {
		t.Fatalf("accept after a decline: %s, event revenue %v, want success with +25", acc.Kind, probe.last.Revenue)
	}
}

// The revenue a network sends WITH a decline never reaches analytics: reports
// now sum declines, and only the reversal belongs there.
func TestDeclineEventIgnoresTheNetworksRevenue(t *testing.T) {
	svc, mapper, _, events, _ := newHarness()
	mapper.set("net-1", "chargeback", event.CpaDecline)
	p := postback("net-1", "click-1", "chargeback")
	v := 40.0
	p.Revenue, p.Currency = &v, "EUR"
	if _, err := svc.Record(ctx(), p); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if ev := events.last(); ev.Revenue != 0 || ev.HasUSDValue || ev.Currency != "" {
		t.Fatalf("decline event = revenue %v usd %v %q, want none of the network's 40 EUR", ev.Revenue, ev.HasUSDValue, ev.Currency)
	}
}
