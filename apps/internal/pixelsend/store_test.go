package pixelsend_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
	"github.com/ismagilovnail/flox/apps/internal/pixelsend"
	"github.com/ismagilovnail/flox/apps/internal/testpg"
)

var testKey = []byte("0123456789abcdef0123456789abcdef")

// fixture is one organization with a campaign, a stream set and pixels
// attached to it.
type fixture struct {
	pool      *pgxpool.Pool
	org       string
	streamSet string
	repo      *pixel.Repository
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()
	f := &fixture{pool: pool, org: idgen.New(), streamSet: idgen.New(), repo: pixel.NewRepository(pool, testKey)}
	src, camp := idgen.New(), idgen.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO organizations (id, name) VALUES ($1, $2)`, []any{f.org, "Pixel Org " + f.org}},
		{`INSERT INTO traffic_sources (id, organization_id, name, type) VALUES ($1, $2, 'Src', 'Facebook')`, []any{src, f.org}},
		{`INSERT INTO campaigns (id, organization_id, traffic_source_id, name, fallback_url) VALUES ($1, $2, $3, 'Camp', 'https://example.com/fb')`, []any{camp, f.org, src}},
		{`INSERT INTO stream_sets (id, organization_id, campaign_id, name) VALUES ($1, $2, $3, 'Set')`, []any{f.streamSet, f.org, camp}},
	} {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	t.Cleanup(func() { _ = testpg.DeleteOrganization(context.Background(), pool, f.org) })
	return f
}

// pixel creates a pixel, attached to the stream set when attach.
func (f *fixture) pixel(t *testing.T, in pixel.CreateInput, attach bool) pixel.Pixel {
	t.Helper()
	ctx := context.Background()
	p, err := pixel.NewService(f.repo).Create(ctx, f.org, in)
	if err != nil {
		t.Fatalf("creating pixel: %v", err)
	}
	if attach {
		if _, err := f.pool.Exec(ctx, `INSERT INTO stream_set_pixels (organization_id, stream_set_id, pixel_id) VALUES ($1, $2, $3)`,
			f.org, f.streamSet, p.ID); err != nil {
			t.Fatalf("attaching pixel: %v", err)
		}
	}
	return p
}

func (f *fixture) event(t event.Type) pixel.Event {
	ev := sampleEvent()
	ev.OrganizationID, ev.StreamSetID, ev.SourceEventID, ev.Type = f.org, f.streamSet, idgen.New(), t
	return ev
}

func TestEnqueueGoesToMatchingPixelsOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	store := pixelsend.NewPostgresStoreOnTable(f.pool, testpg.PrivateTable(t, f.pool, "pixel_deliveries"))

	fb := f.pixel(t, pixel.CreateInput{Name: "FB", Provider: pixel.ProviderFacebook, PixelID: "1", AccessToken: "t", Events: []string{"CPA_ACCEPT"}}, true)
	f.pixel(t, pixel.CreateInput{Name: "Other event", Provider: pixel.ProviderTikTok, PixelID: "2", Events: []string{"CPA_HOLD"}}, true)
	f.pixel(t, pixel.CreateInput{Name: "Not attached", Provider: pixel.ProviderFacebook, PixelID: "3", Events: []string{"CPA_ACCEPT"}}, false)
	paused := f.pixel(t, pixel.CreateInput{Name: "Paused", Provider: pixel.ProviderFacebook, PixelID: "4", Events: []string{"CPA_ACCEPT"}}, true)
	if _, err := pixel.NewService(f.repo).Pause(ctx, f.org, paused.ID); err != nil {
		t.Fatal(err)
	}
	tw := idgen.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO pixels (id, organization_id, name, provider, pixel_id, events) VALUES ($1, $2, 'TW', 'twitter', 'x', '{CPA_ACCEPT}');
		`, tw, f.org); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO stream_set_pixels (organization_id, stream_set_id, pixel_id) VALUES ($1, $2, $3)`, f.org, f.streamSet, tw); err != nil {
		t.Fatal(err)
	}

	ev := f.event(event.CpaAccept)
	wanted, err := store.Wanted(ctx, f.org, f.streamSet, event.CpaAccept)
	if err != nil || !wanted {
		t.Fatalf("Wanted = %v, %v; want true", wanted, err)
	}
	n, err := store.Enqueue(ctx, ev)
	if err != nil || n != 1 {
		t.Fatalf("Enqueue = %d, %v; want 1 delivery (only the attached, active facebook pixel)", n, err)
	}
	if n, _ := store.Enqueue(ctx, ev); n != 0 {
		t.Fatalf("the same event again queued %d, want 0", n)
	}

	claimed, err := store.ClaimDue(ctx, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("ClaimDue = %v, %v", claimed, err)
	}
	d := claimed[0]
	if d.PixelID != fb.ID || d.Event.SourceEventID != ev.SourceEventID || d.Event.IP != ev.IP || *d.Event.USDValue != *ev.USDValue || d.AttemptCount != 1 {
		t.Fatalf("claimed = %+v, want the queued event for the facebook pixel", d)
	}

	if wanted, _ := store.Wanted(ctx, f.org, f.streamSet, event.PwaInstall); wanted {
		t.Fatal("Wanted(PWA_INSTALL) = true, no pixel reports it")
	}
	other := idgen.New()
	if wanted, _ := store.Wanted(ctx, other, f.streamSet, event.CpaAccept); wanted {
		t.Fatal("another organization's event found this organization's pixels")
	}
	if n, _ := store.Enqueue(ctx, pixel.Event{OrganizationID: other, StreamSetID: f.streamSet, SourceEventID: idgen.New(), Type: event.CpaAccept}); n != 0 {
		t.Fatalf("another organization's event queued %d deliveries to this organization's pixels", n)
	}
}

// fakeSender answers with the next outcome and records what it got.
type fakeSender struct {
	outcomes []pixelsend.Outcome
	got      []pixel.Credentials
}

func (s *fakeSender) Send(_ context.Context, c pixel.Credentials, _ pixel.Event) pixelsend.Outcome {
	s.got = append(s.got, c)
	out := s.outcomes[0]
	s.outcomes = s.outcomes[1:]
	return out
}

func deliveryRow(t *testing.T, f *fixture, table, pixelID string) (status string, attempts int, code *int, message string, next time.Time) {
	t.Helper()
	if err := f.pool.QueryRow(context.Background(),
		`SELECT delivery_status, attempt_count, response_status_code, message, next_attempt_at FROM `+table+` WHERE pixel_id = $1`, pixelID,
	).Scan(&status, &attempts, &code, &message, &next); err != nil {
		t.Fatal(err)
	}
	return
}

func TestDelivererOutcomes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	table := testpg.PrivateTable(t, f.pool, "pixel_deliveries")
	store := pixelsend.NewPostgresStoreOnTable(f.pool, table)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

	ok := f.pixel(t, pixel.CreateInput{Name: "OK", Provider: pixel.ProviderFacebook, PixelID: "1", AccessToken: "tok-ok", Events: []string{"CPA_HOLD"}}, true)
	sender := &fakeSender{outcomes: []pixelsend.Outcome{{StatusCode: 200}}}
	d := pixelsend.NewDeliverer(store, f.repo, map[pixel.Provider]pixelsend.Sender{pixel.ProviderFacebook: sender}, logger).WithClock(func() time.Time { return now })
	if _, err := store.Enqueue(ctx, f.event(event.CpaHold)); err != nil {
		t.Fatal(err)
	}
	if n, err := d.RunOnce(ctx, 10); err != nil || n != 1 {
		t.Fatalf("RunOnce = %d, %v", n, err)
	}
	if status, _, code, _, _ := deliveryRow(t, f, table, ok.ID); status != "success" || code == nil || *code != 200 {
		t.Fatalf("delivery = %s/%v, want success 200", status, code)
	}
	if sender.got[0].AccessToken != "tok-ok" {
		t.Fatalf("sender got token %q, want the decrypted one", sender.got[0].AccessToken)
	}

	// Retryable: retried on the postback schedule. Permanent: dead at once.
	retry := f.pixel(t, pixel.CreateInput{Name: "Retry", Provider: pixel.ProviderTikTok, PixelID: "2", Events: []string{"CPA_REDEP"}}, true)
	dead := f.pixel(t, pixel.CreateInput{Name: "Dead", Provider: pixel.ProviderSnapchat, PixelID: "3", Events: []string{"CPA_REDEP"}}, true)
	d = pixelsend.NewDeliverer(store, f.repo, map[pixel.Provider]pixelsend.Sender{
		pixel.ProviderTikTok:   &fakeSender{outcomes: []pixelsend.Outcome{{StatusCode: 503, Err: errString("platform responded 503"), Retry: true}}},
		pixel.ProviderSnapchat: &fakeSender{outcomes: []pixelsend.Outcome{{StatusCode: 401, Err: errString("platform responded 401")}}},
	}, logger).WithClock(func() time.Time { return now })
	if _, err := store.Enqueue(ctx, f.event(event.CpaRedep)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.RunOnce(ctx, 10); err != nil {
		t.Fatal(err)
	}
	status, attempts, _, msg, next := deliveryRow(t, f, table, retry.ID)
	if status != "retrying" || attempts != 1 || msg != "platform responded 503" || !next.Equal(now.Add(30*time.Second)) {
		t.Fatalf("retryable = %s/%d/%q/%v, want retrying in 30s", status, attempts, msg, next)
	}
	if status, _, code, msg, _ := deliveryRow(t, f, table, dead.ID); status != "dead" || *code != 401 || msg != "platform responded 401" {
		t.Fatalf("permanent = %s/%v/%q, want dead", status, code, msg)
	}

	// Paused after queuing: not sent.
	later := f.pixel(t, pixel.CreateInput{Name: "Later paused", Provider: pixel.ProviderFacebook, PixelID: "5", Events: []string{"PWA_INSTALL"}}, true)
	if _, err := store.Enqueue(ctx, f.event(event.PwaInstall)); err != nil {
		t.Fatal(err)
	}
	if _, err := pixel.NewService(f.repo).Pause(ctx, f.org, later.ID); err != nil {
		t.Fatal(err)
	}
	unused := &fakeSender{}
	d = pixelsend.NewDeliverer(store, f.repo, map[pixel.Provider]pixelsend.Sender{pixel.ProviderFacebook: unused}, logger)
	if _, err := d.RunOnce(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if status, _, _, msg, _ := deliveryRow(t, f, table, later.ID); status != "dead" || msg != "pixel is paused: not sent" || len(unused.got) != 0 {
		t.Fatalf("paused = %s/%q (sent %d), want dead without sending", status, msg, len(unused.got))
	}
}

type errString string

func (e errString) Error() string { return string(e) }

// fakeInserter records batches written after the trigger.
type fakeInserter struct{ batches [][]event.Event }

func (f *fakeInserter) InsertBatch(_ context.Context, events []event.Event) error {
	f.batches = append(f.batches, events)
	return nil
}

func TestTriggerSinkQueuesPwaInstall(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	store := pixelsend.NewPostgresStoreOnTable(f.pool, testpg.PrivateTable(t, f.pool, "pixel_deliveries"))
	px := f.pixel(t, pixel.CreateInput{Name: "FB", Provider: pixel.ProviderFacebook, PixelID: "1", Events: []string{"PWA_INSTALL"}}, true)

	clicks := attribution.NewMemoryResolver()
	clickAt := time.Date(2026, 10, 11, 10, 0, 0, 0, time.UTC)
	clicks.Record(attribution.Click{ClickID: "click-9", OrganizationID: f.org, OccurredAt: clickAt, FBClickID: "fb-9"})
	next := &fakeInserter{}
	sink := &pixelsend.TriggerSink{Next: next, Queue: store, Clicks: clicks}

	install := event.Event{ID: idgen.New(), Type: event.PwaInstall, OrganizationID: f.org, StreamSetID: f.streamSet,
		ClickID: "click-9", EventAt: clickAt.Add(time.Hour), IP: "198.51.100.4", UserAgent: "UA"}
	view := event.Event{ID: idgen.New(), Type: event.PwaView, OrganizationID: f.org, StreamSetID: f.streamSet, ClickID: "click-9"}
	batch := []event.Event{view, install}
	if err := sink.InsertBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	if err := sink.InsertBatch(ctx, batch); err != nil { // a retried batch
		t.Fatal(err)
	}
	if len(next.batches) != 2 || len(next.batches[0]) != 2 {
		t.Fatalf("written batches = %v, want both batches passed on whole", next.batches)
	}
	claimed, err := store.ClaimDue(ctx, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claimed = %v, %v; want one delivery for the install", claimed, err)
	}
	ev := claimed[0].Event
	if claimed[0].PixelID != px.ID || ev.SourceEventID != install.ID || ev.FBClickID != "fb-9" || !ev.ClickAt.Equal(clickAt) || ev.IP != "198.51.100.4" {
		t.Fatalf("queued event = %+v, want the install with its click's fbclid and time", ev)
	}
}

// An iOS install (IOS_INSTALL) reaches the pixels set for PWA_INSTALL, as
// PWA_INSTALL — the platform's mapped event (CompleteRegistration) and the
// {status} macro are the configured event's.
func TestTriggerSinkQueuesIosInstallAsPwaInstall(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	store := pixelsend.NewPostgresStoreOnTable(f.pool, testpg.PrivateTable(t, f.pool, "pixel_deliveries"))
	px := f.pixel(t, pixel.CreateInput{Name: "FB", Provider: pixel.ProviderFacebook, PixelID: "1", Events: []string{"PWA_INSTALL"}}, true)
	sink := &pixelsend.TriggerSink{Next: &fakeInserter{}, Queue: store}

	install := event.Event{ID: idgen.New(), Type: event.IosInstall, OrganizationID: f.org, StreamSetID: f.streamSet,
		ClickID: "click-ios", EventAt: time.Now(), IP: "198.51.100.5", UserAgent: "iPhone"}
	if err := sink.InsertBatch(ctx, []event.Event{install}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimDue(ctx, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claimed = %v, %v; want one delivery for the iOS install", claimed, err)
	}
	ev := claimed[0].Event
	if claimed[0].PixelID != px.ID || ev.SourceEventID != install.ID || ev.Type != event.PwaInstall || ev.Macros.Status != "PWA_INSTALL" {
		t.Fatalf("queued event = %+v, want the iOS install as PWA_INSTALL", ev)
	}
}

// TG_START and TG_JOIN come from Telegram's servers (the bot's webhook):
// the platform gets the visitor's IP and user agent from the click, each
// event goes only to the pixels set for it, and is reported under its own
// platform name.
func TestTriggerSinkQueuesTelegramEventsWithTheClicksVisitor(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	store := pixelsend.NewPostgresStoreOnTable(f.pool, testpg.PrivateTable(t, f.pool, "pixel_deliveries"))
	join := f.pixel(t, pixel.CreateInput{Name: "FB join", Provider: pixel.ProviderFacebook, PixelID: "1", Events: []string{"TG_JOIN"}}, true)

	clicks := attribution.NewMemoryResolver()
	clickAt := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	clicks.Record(attribution.Click{ClickID: "click-tg", OrganizationID: f.org, OccurredAt: clickAt,
		IP: "203.0.113.7", UserAgent: "Instagram 300", FBClickID: "fb-tg"})
	sink := &pixelsend.TriggerSink{Next: &fakeInserter{}, Queue: store, Clicks: clicks}

	start := event.Event{ID: idgen.New(), Type: event.TgStart, OrganizationID: f.org, StreamSetID: f.streamSet,
		ClickID: "click-tg", EventAt: clickAt.Add(time.Minute)}
	joined := event.Event{ID: idgen.New(), Type: event.TgJoin, OrganizationID: f.org, StreamSetID: f.streamSet,
		ClickID: "click-tg", EventAt: clickAt.Add(2 * time.Minute)}
	if err := sink.InsertBatch(ctx, []event.Event{start, joined}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimDue(ctx, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claimed = %v, %v; want one delivery: the join (no pixel reports TG_START)", claimed, err)
	}
	ev := claimed[0].Event
	if claimed[0].PixelID != join.ID || ev.SourceEventID != joined.ID || ev.Type != event.TgJoin ||
		ev.IP != "203.0.113.7" || ev.UserAgent != "Instagram 300" || ev.FBClickID != "fb-tg" || !ev.ClickAt.Equal(clickAt) {
		t.Fatalf("queued event = %+v, want the join with its click's visitor", ev)
	}
	if name := pixel.EventName(pixel.ProviderFacebook, nil, string(ev.Type)); name != "Subscribe" {
		t.Fatalf("facebook name for TG_JOIN = %q, want Subscribe", name)
	}
}

// A push subscription comes from the visitor's own installed app: its IP
// and user agent are kept, not replaced by the click's; the platform click
// id still comes from the click.
func TestTriggerSinkQueuesPushSubscribeWithItsOwnVisitor(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	store := pixelsend.NewPostgresStoreOnTable(f.pool, testpg.PrivateTable(t, f.pool, "pixel_deliveries"))
	px := f.pixel(t, pixel.CreateInput{Name: "TT push", Provider: pixel.ProviderTikTok, PixelID: "1", Events: []string{"NOTIFICATION_SUBSCRIBE"}}, true)

	clicks := attribution.NewMemoryResolver()
	clicks.Record(attribution.Click{ClickID: "click-push", OrganizationID: f.org, OccurredAt: time.Now().Add(-time.Hour),
		IP: "203.0.113.8", UserAgent: "TikTok webview", TTClickID: "tt-push"})
	sink := &pixelsend.TriggerSink{Next: &fakeInserter{}, Queue: store, Clicks: clicks}

	sub := event.Event{ID: idgen.New(), Type: event.NotificationSubscribe, OrganizationID: f.org, StreamSetID: f.streamSet,
		ClickID: "click-push", EventAt: time.Now(), IP: "198.51.100.9", UserAgent: "Chrome PWA"}
	if err := sink.InsertBatch(ctx, []event.Event{sub}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimDue(ctx, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claimed = %v, %v; want one delivery for the subscription", claimed, err)
	}
	ev := claimed[0].Event
	if claimed[0].PixelID != px.ID || ev.Type != event.NotificationSubscribe ||
		ev.IP != "198.51.100.9" || ev.UserAgent != "Chrome PWA" || ev.TTClickID != "tt-push" {
		t.Fatalf("queued event = %+v, want the subscription with its own visitor and the click's ttclid", ev)
	}
	if name := pixel.EventName(pixel.ProviderTikTok, nil, string(ev.Type)); name != "Subscribe" {
		t.Fatalf("tiktok name for NOTIFICATION_SUBSCRIBE = %q, want Subscribe", name)
	}
}
