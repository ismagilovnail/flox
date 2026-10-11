package reattribute_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
	"github.com/ismagilovnail/flox/apps/internal/reattribute"
	"github.com/ismagilovnail/flox/apps/internal/testpg"
)

type harness struct {
	t                *testing.T
	pg               *pgxpool.Pool
	ch               driver.Conn
	store            *chstore.EventStore
	retrier          *reattribute.Retrier
	retries, queue   string
	orgID, networkID string
	campaignID       string
	conversionAt     time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dsn, chURL := os.Getenv("DATABASE_URL"), os.Getenv("CLICKHOUSE_URL")
	if dsn == "" || chURL == "" {
		t.Skip("needs DATABASE_URL and CLICKHOUSE_URL")
	}
	ctx := context.Background()
	pg, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	t.Cleanup(pg.Close) // not defer: the cleanups below run in t.Cleanup and need the pool
	ch, err := chconn.NewConn(ctx, config.ClickHouseConfig{URL: chURL, Database: "flox", User: "flox"})
	if err != nil {
		t.Fatalf("clickhouse: %v", err)
	}
	t.Cleanup(func() { _ = ch.Close() })
	if err := chstore.Migrate(ctx, ch); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	h := &harness{
		t: t, pg: pg, ch: ch, store: chstore.NewEventStore(ch),
		// Private copies: the dev worker runs a retrier and a flusher of its own.
		retries: testpg.PrivateTable(t, pg, "attribution_retries"),
		queue:   testpg.PrivateTable(t, pg, "event_queue"),
		orgID:   idgen.New(), networkID: idgen.New(), campaignID: idgen.New(),
		conversionAt: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC),
	}
	h.retrier = reattribute.New(pg, conversion.NewPostgresStore(pg), chstore.NewClickResolver(ch), h.store,
		slog.New(slog.NewTextHandler(io.Discard, nil))).OnTables(h.retries, h.queue)
	h.exec(`INSERT INTO organizations (id, name) VALUES ($1, 'reattribute test')`, h.orgID)
	t.Cleanup(func() { _, _ = pg.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, h.orgID) })
	h.exec(`INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, 'n')`, h.networkID, h.orgID)
	return h
}

func (h *harness) exec(sql string, args ...any) {
	h.t.Helper()
	if _, err := h.pg.Exec(context.Background(), sql, args...); err != nil {
		h.t.Fatalf("%s: %v", sql, err)
	}
}

// click puts the conversion's click in ClickHouse, an hour before it.
func (h *harness) click(clickID string) {
	h.t.Helper()
	if err := h.store.InsertBatch(chstore.WithBatchID(context.Background(), idgen.New()), []event.Event{{
		ID: idgen.New(), Type: event.SourceClick, EventAt: h.conversionAt.Add(-time.Hour), OrganizationID: h.orgID,
		CampaignID: h.campaignID, ClickID: clickID, Country: "DE",
	}}); err != nil {
		h.t.Fatalf("inserting click: %v", err)
	}
}

// unknownClick records an accept the way conversion.PostgresStore does when
// its click was not found, and returns the conversion as stored.
func (h *harness) unknownClick(clickID string) conversion.StoredConversion {
	h.t.Helper()
	id := idgen.New()
	h.exec(`INSERT INTO postbacks (id, organization_id, network_id, click_id, status, direction, result,
	          usd_value, currency, revenue, attribution_outcome, attribution_method, message, event_at)
	        VALUES ($1, $2, $3, $4, 'CPA_ACCEPT', 'incoming', 'success', 25, 'USD', 25, 'unknown_click', 'none',
	          'click_id not found', $5)`,
		id, h.orgID, h.networkID, clickID, h.conversionAt)
	c, err := conversion.NewPostgresStore(h.pg).StoredSuccess(context.Background(), h.orgID, id)
	if err != nil {
		h.t.Fatalf("StoredSuccess: %v", err)
	}
	return c
}

func (h *harness) due(postbackID string, attempt int) {
	h.t.Helper()
	h.exec(`INSERT INTO `+h.retries+` (postback_id, organization_id, attempt, next_attempt_at)
	        VALUES ($1, $2, $3, now() - interval '1 second')`, postbackID, h.orgID, attempt)
}

func (h *harness) makeDue(postbackID string) {
	h.t.Helper()
	h.exec(`UPDATE `+h.retries+` SET next_attempt_at = now() - interval '1 second' WHERE postback_id = $1`, postbackID)
}

func (h *harness) run() {
	h.t.Helper()
	if _, err := h.retrier.RunOnce(context.Background(), 100); err != nil {
		h.t.Fatalf("RunOnce: %v", err)
	}
}

// retry reads the retry row; ok is false once it is gone.
func (h *harness) retry(postbackID string) (attempt int, inSeconds float64, attributed, ok bool) {
	h.t.Helper()
	err := h.pg.QueryRow(context.Background(), `
		SELECT attempt, extract(epoch FROM next_attempt_at - now())::float8, attributed_at IS NOT NULL
		FROM `+h.retries+` WHERE postback_id = $1`, postbackID).Scan(&attempt, &inSeconds, &attributed)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return 0, 0, false, false
		}
		h.t.Fatalf("reading retry: %v", err)
	}
	return attempt, inSeconds, attributed, true
}

func (h *harness) stored(id string) conversion.StoredConversion {
	h.t.Helper()
	c, err := conversion.NewPostgresStore(h.pg).StoredSuccess(context.Background(), h.orgID, id)
	if err != nil {
		h.t.Fatalf("StoredSuccess: %v", err)
	}
	return c
}

func (h *harness) copies(c conversion.StoredConversion) chstore.StoredEvent {
	h.t.Helper()
	got, err := h.store.ConversionEventCopies(context.Background(), h.orgID, c.ID, c.EventAt)
	if err != nil {
		h.t.Fatalf("ConversionEventCopies: %v", err)
	}
	return got
}

// unattributedInClickHouse is the copy the flush wrote before the click
// turned up.
func (h *harness) unattributedInClickHouse(c conversion.StoredConversion) {
	h.t.Helper()
	if err := h.store.InsertBatch(chstore.WithBatchID(context.Background(), idgen.New()),
		[]event.Event{conversion.EventFor(c, nil)}); err != nil {
		h.t.Fatalf("inserting unattributed copy: %v", err)
	}
}

func (h *harness) wantAttributed(id, clickID string) {
	h.t.Helper()
	got := h.stored(id)
	if got.AttributionOutcome != string(attribution.OutcomeAttributed) || got.AttributedClickID != clickID ||
		got.AttributionMethod != string(attribution.MethodClickID) || got.TimeToConversionMS == nil || *got.TimeToConversionMS != time.Hour.Milliseconds() {
		h.t.Fatalf("postbacks row = %+v, want attributed to %s by click_id, 1h to conversion", got, clickID)
	}
	var message string
	if err := h.pg.QueryRow(context.Background(), `SELECT message FROM postbacks WHERE id = $1`, id).Scan(&message); err != nil {
		h.t.Fatal(err)
	}
	if !strings.Contains(message, "attributed on retry") {
		h.t.Errorf("message = %q, want the retry noted", message)
	}
}

// Still waiting in event_queue: the queued payload is rewritten and nothing
// is left for ClickHouse.
func TestClickTurnsUpWhileEventIsQueued(t *testing.T) {
	h := newHarness(t)
	clickID := idgen.New()
	c := h.unknownClick(clickID)
	payload, _ := json.Marshal(conversion.EventFor(c, nil))
	h.exec(`INSERT INTO `+h.queue+` (id, organization_id, type, payload) VALUES ($1, $2, 'CPA_ACCEPT', $3)`, c.ID, h.orgID, payload)
	h.due(c.ID, 0)
	h.click(clickID)

	h.run()

	h.wantAttributed(c.ID, clickID)
	var queued event.Event
	var raw []byte
	if err := h.pg.QueryRow(context.Background(), `SELECT payload FROM `+h.queue+` WHERE id = $1`, c.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &queued); err != nil {
		t.Fatal(err)
	}
	if queued.CampaignID != h.campaignID || queued.Country != "DE" || queued.AttributionOutcome != "attributed" || queued.ID != c.ID {
		t.Errorf("queued event = %+v, want the click's campaign and country, same id", queued)
	}
	if _, _, _, ok := h.retry(c.ID); ok {
		t.Error("retry row left behind")
	}
	if got := h.copies(c); got.Copies != 0 {
		t.Errorf("ClickHouse copies = %d, want none: the flush writes it", got.Copies)
	}
}

// Already flushed unattributed: the copy is replaced and the daily bucket
// it fed under no campaign rebuilt.
func TestClickTurnsUpAfterEventReachedClickHouse(t *testing.T) {
	h := newHarness(t)
	clickID := idgen.New()
	c := h.unknownClick(clickID)
	h.unattributedInClickHouse(c)
	h.due(c.ID, 1)
	h.click(clickID)

	h.run()

	h.wantAttributed(c.ID, clickID)
	if _, _, _, ok := h.retry(c.ID); ok {
		t.Error("retry row left behind")
	}
	if got := h.copies(c); got.Copies != 1 || got.CampaignID != h.campaignID || got.USD != 25 {
		t.Errorf("ClickHouse = %+v, want one copy under the campaign with its 25 USD", got)
	}
	daily := func(campaignID string) (n uint64) {
		t.Helper()
		if err := h.ch.QueryRow(context.Background(), `
			SELECT sum(event_count) FROM conversion_events_daily_campaign
			WHERE organization_id = ? AND campaign_id = ? AND type = 'CPA_ACCEPT'`, h.orgID, campaignID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if old, now := daily(""), daily(h.campaignID); old != 0 || now != 1 {
		t.Errorf("daily buckets: no campaign %d, campaign %d — want 0 and 1", old, now)
	}
}

// Being flushed right now: Postgres is attributed, the ClickHouse rewrite
// waits for the flush, and a later run — after a restart too — does only
// that.
func TestEventInFlightIsRewrittenAfterTheFlush(t *testing.T) {
	h := newHarness(t)
	clickID := idgen.New()
	c := h.unknownClick(clickID)
	payload, _ := json.Marshal(conversion.EventFor(c, nil))
	h.exec(`INSERT INTO `+h.queue+` (id, organization_id, type, payload, status, claimed_at, batch_id)
	        VALUES ($1, $2, 'CPA_ACCEPT', $3, 'processing', now(), 'b1')`, c.ID, h.orgID, payload)
	h.due(c.ID, 0)
	h.click(clickID)

	h.run()

	h.wantAttributed(c.ID, clickID)
	attempt, in, attributed, ok := h.retry(c.ID)
	if !ok || !attributed || attempt != 0 || in < 20 || in > 40 {
		t.Fatalf("retry = attempt %d in %.0fs attributed %v present %v, want attempt 0 again in ~30s, attributed", attempt, in, attributed, ok)
	}

	// The flush lands the unattributed copy and deletes the queue row.
	h.unattributedInClickHouse(c)
	h.exec(`DELETE FROM `+h.queue+` WHERE id = $1`, c.ID)
	h.makeDue(c.ID)
	h.run()

	if _, _, _, ok := h.retry(c.ID); ok {
		t.Error("retry row left behind")
	}
	if got := h.copies(c); got.Copies != 1 || got.CampaignID != h.campaignID {
		t.Errorf("ClickHouse = %+v, want one copy under the campaign", got)
	}
}

// A dead-lettered event never reaches ClickHouse on its own; the
// reconciliation re-emits it from the postbacks row, attributed by now.
func TestDeadLetteredEventIsLeftToTheReconciliation(t *testing.T) {
	h := newHarness(t)
	clickID := idgen.New()
	c := h.unknownClick(clickID)
	payload, _ := json.Marshal(conversion.EventFor(c, nil))
	h.exec(`INSERT INTO `+h.queue+` (id, organization_id, type, payload, status, dead_at, batch_id)
	        VALUES ($1, $2, 'CPA_ACCEPT', $3, 'dead', now(), 'b1')`, c.ID, h.orgID, payload)
	h.due(c.ID, 0)
	h.click(clickID)

	h.run()

	h.wantAttributed(c.ID, clickID)
	if _, _, _, ok := h.retry(c.ID); ok {
		t.Error("retry row left behind")
	}
	if got := h.copies(c); got.Copies != 0 {
		t.Errorf("ClickHouse copies = %d, want none", got.Copies)
	}
}

// No click: each miss moves to the next delay; after the last one
// unknown_click is final and says so.
func TestClickNeverTurnsUp(t *testing.T) {
	h := newHarness(t)
	c := h.unknownClick(idgen.New())
	h.due(c.ID, 0)

	h.run()

	attempt, in, attributed, ok := h.retry(c.ID)
	want := conversion.AttributionRetryDelays[1].Seconds()
	if !ok || attributed || attempt != 1 || in < want-10 || in > want+10 {
		t.Fatalf("retry = attempt %d in %.0fs attributed %v present %v, want attempt 1 in ~%.0fs", attempt, in, attributed, ok, want)
	}

	last := len(conversion.AttributionRetryDelays) - 1
	h.exec(`UPDATE `+h.retries+` SET attempt = $2 WHERE postback_id = $1`, c.ID, last)
	h.makeDue(c.ID)
	h.run()

	if _, _, _, ok := h.retry(c.ID); ok {
		t.Error("retry row left after the last miss")
	}
	got := h.stored(c.ID)
	if got.AttributionOutcome != string(attribution.OutcomeUnknownClick) {
		t.Errorf("outcome = %s, want unknown_click to stay", got.AttributionOutcome)
	}
	var message string
	if err := h.pg.QueryRow(context.Background(), `SELECT message FROM postbacks WHERE id = $1`, c.ID).Scan(&message); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "still no click after 5 retries") {
		t.Errorf("message = %q, want the give-up noted", message)
	}
}

// A retry row of another organization's postback is worked under that
// organization: the click is looked up in its own clicks only.
func TestRetryLooksUpOnlyItsOwnOrganizationsClicks(t *testing.T) {
	h := newHarness(t)
	other := newHarness(t)
	clickID := idgen.New()
	c := h.unknownClick(clickID)
	h.due(c.ID, 0)
	other.click(clickID) // same click id, another tenant

	h.run()

	if got := h.stored(c.ID); got.AttributionOutcome != string(attribution.OutcomeUnknownClick) || got.AttributedClickID != "" {
		t.Fatalf("postbacks row = %+v, attributed to another organization's click", got)
	}
	if attempt, _, _, ok := h.retry(c.ID); !ok || attempt != 1 {
		t.Errorf("retry = attempt %d present %v, want a miss (attempt 1)", attempt, ok)
	}
}

// pixelProbe records the pixel events queued in the attribution's
// transaction.
type pixelProbe struct{ got []pixel.Event }

func (p *pixelProbe) InsertTx(_ context.Context, _ pgx.Tx, ev pixel.Event) error {
	p.got = append(p.got, ev)
	return nil
}

// A conversion recorded without its click had no stream set, so its pixels
// got nothing; attributed on retry, it is queued for them with the click's
// stream set and the conversion's own id.
func TestAttributedOnRetryQueuesPixelEvent(t *testing.T) {
	h := newHarness(t)
	probe := &pixelProbe{}
	h.retrier.WithPixels(probe)
	clickID := idgen.New()
	c := h.unknownClick(clickID)
	h.due(c.ID, 0)
	streamSet := idgen.New()
	if err := h.store.InsertBatch(chstore.WithBatchID(context.Background(), idgen.New()), []event.Event{{
		ID: idgen.New(), Type: event.SourceClick, EventAt: h.conversionAt.Add(-time.Hour), OrganizationID: h.orgID,
		CampaignID: h.campaignID, StreamSetID: streamSet, ClickID: clickID, IP: "203.0.113.5", UserAgent: "UA", FBClickID: "fb-r",
	}}); err != nil {
		t.Fatal(err)
	}

	h.run()

	h.wantAttributed(c.ID, clickID)
	if len(probe.got) != 1 {
		t.Fatalf("pixel events = %d, want 1", len(probe.got))
	}
	ev := probe.got[0]
	if ev.SourceEventID != c.ID || ev.StreamSetID != streamSet || ev.Type != event.CpaAccept || ev.IP != "203.0.113.5" || ev.FBClickID != "fb-r" {
		t.Fatalf("pixel event = %+v", ev)
	}
}
