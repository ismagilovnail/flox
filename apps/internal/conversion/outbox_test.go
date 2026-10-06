package conversion_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	pbstore "github.com/ismagilovnail/flox/apps/internal/postback"
)

// probeWriter wraps the real delivery writer. Inside the transaction it
// checks that the CPA event row is already there — read through the same
// tx, so a dev-stack worker polling event_queue cannot consume it first —
// and it can be told to fail, to prove the whole record rolls back.
type probeWriter struct {
	inner pbstore.PostgresStore

	mu          sync.Mutex
	calls       int
	eventsInTx  int
	failWith    error
	lastRequest conversion.DeliveryRequest
}

func (w *probeWriter) InsertTx(ctx context.Context, tx pgx.Tx, req conversion.DeliveryRequest) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls++
	w.lastRequest = req
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM event_queue WHERE payload->>'clickId' = $1`, req.ClickID,
	).Scan(&w.eventsInTx); err != nil {
		return err
	}
	if w.failWith != nil {
		return w.failWith
	}
	return w.inner.InsertTx(ctx, tx, req)
}

func successEntry(orgID, networkID, clickID string, status event.Type) conversion.Entry {
	ev := event.Event{Type: status, OrganizationID: orgID, ClickID: clickID, NetworkID: networkID, EventAt: time.Now().UTC()}
	return conversion.Entry{
		OrganizationID: orgID, NetworkID: networkID, ClickID: clickID,
		Status: status, Kind: conversion.ResultSuccess,
		Event: &ev,
		Delivery: &conversion.DeliveryRequest{
			OrganizationID: orgID, NetworkID: networkID, ClickID: clickID, Status: status,
			URL: "https://net.example/pb?click_id=" + clickID,
		},
	}
}

// Audit 2026-10-06, D1: a recorded conversion, its CPA event and its
// outgoing postback are one transaction.
func TestRecordWritesOutboxInSameTransaction(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID, false)
	writer := &probeWriter{inner: *pbstore.NewPostgresStore(pool)}
	store := conversion.NewPostgresStore(pool).WithDeliveries(writer)
	clickID := idgen.New()

	rec, err := store.Record(ctx, successEntry(orgID, networkID, clickID, event.CpaAccept))
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if rec.Kind != conversion.ResultSuccess {
		t.Fatalf("kind = %q, want success", rec.Kind)
	}
	if writer.eventsInTx != 1 {
		t.Fatalf("CPA events visible in the record's transaction = %d, want 1", writer.eventsInTx)
	}
	if writer.lastRequest.SourcePostbackID != rec.ID {
		t.Fatalf("delivery source_postback_id = %q, want the recorded row %q", writer.lastRequest.SourcePostbackID, rec.ID)
	}
	var deliveries int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM postback_deliveries WHERE source_postback_id = $1`, rec.ID,
	).Scan(&deliveries); err != nil {
		t.Fatalf("counting deliveries: %v", err)
	}
	if deliveries != 1 {
		t.Fatalf("deliveries for the conversion = %d, want 1", deliveries)
	}

	// A duplicate writes its own 'duplicate' row and no side effects.
	dup, err := store.Record(ctx, successEntry(orgID, networkID, clickID, event.CpaAccept))
	if err != nil {
		t.Fatalf("Record (duplicate): %v", err)
	}
	if dup.Kind != conversion.ResultDuplicate {
		t.Fatalf("second kind = %q, want duplicate", dup.Kind)
	}
	if writer.calls != 1 {
		t.Fatalf("delivery writes = %d, want 1 (a duplicate must not queue another postback)", writer.calls)
	}
}

func TestRecordRollsBackEverythingWhenTheOutboxFails(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID, false)
	writer := &probeWriter{inner: *pbstore.NewPostgresStore(pool), failWith: errors.New("delivery queue unavailable")}
	store := conversion.NewPostgresStore(pool).WithDeliveries(writer)
	clickID := idgen.New()

	if _, err := store.Record(ctx, successEntry(orgID, networkID, clickID, event.CpaAccept)); err == nil {
		t.Fatal("Record succeeded although its delivery could not be queued")
	}

	// Nothing may survive: the network's retry has to find a clean slate,
	// not a "duplicate" of a conversion whose side effects never happened.
	var rows, events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM postbacks WHERE click_id = $1`, clickID).Scan(&rows); err != nil {
		t.Fatalf("counting postbacks: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event_queue WHERE payload->>'clickId' = $1`, clickID).Scan(&events); err != nil {
		t.Fatalf("counting events: %v", err)
	}
	if rows != 0 || events != 0 {
		t.Fatalf("after a failed record: postbacks=%d events=%d, want 0/0", rows, events)
	}

	// The retry succeeds once the queue is back.
	writer.failWith = nil
	rec, err := store.Record(ctx, successEntry(orgID, networkID, clickID, event.CpaAccept))
	if err != nil || rec.Kind != conversion.ResultSuccess {
		t.Fatalf("retry: kind=%q err=%v, want success", rec.Kind, err)
	}
}

func TestRecordRefusesDeliveryWithoutWriter(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID, false)

	_, err := conversion.NewPostgresStore(pool).Record(ctx, successEntry(orgID, networkID, idgen.New(), event.CpaAccept))
	if err == nil {
		t.Fatal("a store without a DeliveryWriter recorded a conversion whose postback would never be sent")
	}
}

// Audit 2026-10-06, D3: a CPA_HOLD replay racing a CPA_ACCEPT for the same
// click used to pass the progression check on both sides and could land
// last. Under the click lock the click always ends on CPA_ACCEPT.
func TestConcurrentHoldAndAcceptNeverEndOnHold(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID, false)
	store := conversion.NewPostgresStore(pool)

	for i := 0; i < 40; i++ {
		clickID := idgen.New()
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, status := range []event.Type{event.CpaAccept, event.CpaHold} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := store.Record(ctx, conversion.Entry{
					OrganizationID: orgID, NetworkID: networkID, ClickID: clickID,
					Status: status, Kind: conversion.ResultSuccess,
				})
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("Record: %v", err)
			}
		}
		last, ok, err := store.LastStatus(ctx, orgID, clickID)
		if err != nil || !ok {
			t.Fatalf("LastStatus: ok=%v err=%v", ok, err)
		}
		if last != event.CpaAccept {
			t.Fatalf("iteration %d: click ended on %s, want CPA_ACCEPT", i, last)
		}
	}
}

// The row is dated like its ClickHouse event (postbacks.event_at), not by
// the moment of the write, so the reconciliation buckets both sides on the
// same day.
func TestRecordStampsTheEventTime(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	networkID := seedNetwork(t, ctx, pool, orgID, false)
	store := conversion.NewPostgresStore(pool).WithDeliveries(&probeWriter{inner: *pbstore.NewPostgresStore(pool)})

	received := time.Date(2026, 9, 10, 23, 59, 59, 998_000_000, time.UTC)
	e := successEntry(orgID, networkID, idgen.New(), event.CpaAccept)
	e.Event.EventAt = received
	rec, err := store.Record(ctx, e)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	var eventAt, createdAt time.Time
	if err := pool.QueryRow(ctx, `SELECT event_at, created_at FROM postbacks WHERE id = $1`, rec.ID).Scan(&eventAt, &createdAt); err != nil {
		t.Fatalf("reading row: %v", err)
	}
	if !eventAt.Equal(received) {
		t.Fatalf("event_at = %s, want the event's %s (created_at is %s)", eventAt.UTC(), received, createdAt.UTC())
	}
}
