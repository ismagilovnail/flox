package pushsend_test

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/push"
	"github.com/ismagilovnail/flox/apps/internal/pushsend"
	"github.com/ismagilovnail/flox/apps/internal/testpg"
)

func TestClassify(t *testing.T) {
	for _, c := range []struct {
		code int
		err  error
		want pushsend.Result
	}{
		{201, nil, pushsend.ResultDelivered},
		{404, nil, pushsend.ResultGone},
		{410, nil, pushsend.ResultGone},
		{429, nil, pushsend.ResultRetry},
		{503, nil, pushsend.ResultRetry},
		{0, errors.New("dial"), pushsend.ResultRetry},
		{400, nil, pushsend.ResultFailed},
		{403, nil, pushsend.ResultFailed},
		{413, nil, pushsend.ResultFailed},
	} {
		if got, _ := pushsend.Classify(c.code, c.err); got != c.want {
			t.Errorf("Classify(%d, %v) = %s, want %s", c.code, c.err, got, c.want)
		}
	}
}

// subscriberKeys is a browser's subscription key pair.
func subscriberKeys(t *testing.T) (p256dh, auth string) {
	t.Helper()
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a := make([]byte, 16)
	_, _ = rand.Read(a)
	return base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(a)
}

// TestWebPushSends: the request a push service gets is encrypted
// (aes128gcm) and carries the VAPID authorization and the TTL.
func TestWebPushSends(t *testing.T) {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	var got *http.Request
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	p256, auth := subscriberKeys(t)
	sender := &pushsend.WebPush{Client: srv.Client(), PublicKey: pub, PrivateKey: priv, Subject: "mailto:ops@flox.example", TTL: 3600}
	code, err := sender.Send(context.Background(), pushsend.Subscriber{Endpoint: srv.URL + "/push/abc", P256dh: p256, Auth: auth}, []byte(`{"title":"Hi"}`))
	if err != nil || code != http.StatusCreated {
		t.Fatalf("Send = %d, %v", code, err)
	}
	if got.Header.Get("Content-Encoding") != "aes128gcm" || got.Header.Get("TTL") != "3600" ||
		!strings.HasPrefix(got.Header.Get("Authorization"), "vapid t=") || !strings.Contains(got.Header.Get("Authorization"), "k="+pub) {
		t.Fatalf("headers = %v", got.Header)
	}
	if strings.Contains(string(body), "Hi") {
		t.Fatal("payload sent in the clear")
	}
}

type fakeSender struct {
	mu    sync.Mutex
	codes map[string]int // by endpoint; missing = 201
	sent  []pushsend.Payload
}

func (s *fakeSender) Send(_ context.Context, to pushsend.Subscriber, payload []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var p pushsend.Payload
	_ = json.Unmarshal(payload, &p)
	s.sent = append(s.sent, p)
	if c, ok := s.codes[to.Endpoint]; ok {
		return c, nil
	}
	return http.StatusCreated, nil
}

type fakeEvents struct {
	mu     sync.Mutex
	events []event.Event
}

func (e *fakeEvents) InsertBatch(_ context.Context, evs []event.Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, evs...)
	return nil
}

// TestDeliverer sends a broadcast to three subscribers: one delivered, one
// gone (subscription ended, NOTIFICATION_UNSUBSCRIBE on its click, its
// other pending delivery dropped), one refused; counters on the broadcast.
func TestDeliverer(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	table := testpg.PrivateTable(t, pool, "push_deliveries")
	org, pwa := idgen.New(), idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, org, "Pushsend "+org); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testpg.DeleteOrganization(context.Background(), pool, org) })
	if _, err := pool.Exec(ctx, `INSERT INTO pwas (id, organization_id, name, short_name, theme_color, background_color, icon_url, start_url)
		VALUES ($1, $2, 'App', 'App', '#000000', '#ffffff', 'https://cdn.example/app.png', '/')`, pwa, org); err != nil {
		t.Fatal(err)
	}

	repo := push.NewRepositoryOnTable(pool, table)
	endpoints := map[string]string{}
	clicks := map[string]string{}
	for _, name := range []string{"ok", "gone", "bad"} {
		endpoints[name] = "https://push.invalid/" + idgen.New()
		clicks[name] = idgen.New()
		claims, _ := json.Marshal(landinglink.Claims{OrganizationID: org, ClickID: clicks[name], CampaignID: "camp-1", PwaID: pwa})
		if _, err := repo.SaveSubscription(ctx, push.NewSubscription{
			OrganizationID: org, PwaID: pwa, CampaignID: "camp-1", ClickID: clicks[name], Endpoint: endpoints[name],
			P256dh: "k", Auth: "a", LinkToken: "tok-" + name, Context: claims, Origin: "https://t.example", Country: "US",
		}); err != nil {
			t.Fatal(err)
		}
	}
	svc := push.NewService(repo, true)
	b, err := svc.CreateBroadcast(ctx, org, "", push.CreateBroadcastInput{Name: "Promo", Title: "Hi", Body: "There", Target: push.TargetDestination})
	if err != nil || b.Total != 3 {
		t.Fatalf("broadcast: %+v, %v", b, err)
	}
	second, err := svc.CreateBroadcast(ctx, org, "", push.CreateBroadcastInput{Name: "Promo 2", Title: "Again", Target: push.TargetDestination})
	if err != nil {
		t.Fatal(err)
	}
	// Only the first broadcast is due now.
	if _, err := pool.Exec(ctx, `UPDATE `+table+` SET next_attempt_at = now() + interval '1 hour' WHERE broadcast_id = $1`, second.ID); err != nil {
		t.Fatal(err)
	}

	sender := &fakeSender{codes: map[string]int{endpoints["gone"]: 410, endpoints["bad"]: 400}}
	events := &fakeEvents{}
	d := pushsend.NewDeliverer(pushsend.NewPostgresStoreOnTable(pool, table), sender, events, slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithClock(func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) })
	if n, err := d.RunOnce(ctx, 10); err != nil || n != 3 {
		t.Fatalf("RunOnce = %d, %v", n, err)
	}

	got, _ := svc.GetBroadcast(ctx, org, b.ID)
	if got.Delivered != 1 || got.Failed != 2 || got.Unsubscribed != 1 {
		t.Fatalf("counters = delivered %d failed %d unsubscribed %d", got.Delivered, got.Failed, got.Unsubscribed)
	}
	for _, p := range sender.sent {
		if p.Title != "Hi" || p.Body != "There" || p.Icon != "https://cdn.example/app.png" ||
			!strings.HasPrefix(p.URL, "https://t.example/pwa/tok-") || !strings.Contains(p.URL, "/push/click/") {
			t.Fatalf("payload = %+v", p)
		}
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM push_subscriptions WHERE endpoint = $1`, endpoints["gone"]).Scan(&status); err != nil || status != push.StatusUnsubscribed {
		t.Fatalf("gone subscription = %q, %v", status, err)
	}
	var dropped string
	if err := pool.QueryRow(ctx, `SELECT d.delivery_status FROM `+table+` d JOIN push_subscriptions s ON s.id = d.subscription_id
		WHERE d.broadcast_id = $1 AND s.endpoint = $2`, second.ID, endpoints["gone"]).Scan(&dropped); err != nil || dropped != "dead" {
		t.Fatalf("ended subscription's pending delivery = %q, %v", dropped, err)
	}
	if len(events.events) != 1 || events.events[0].Type != event.NotificationUnsubscribe ||
		events.events[0].ClickID != clicks["gone"] || events.events[0].PwaID != pwa || events.events[0].Country != "US" {
		t.Fatalf("events = %+v", events.events)
	}
	// Ended subscriptions get nothing more.
	if n, _ := svc.CountAudience(ctx, org, push.Filters{}); n != 2 {
		t.Fatalf("audience after unsubscribe = %d, want 2", n)
	}
}

// TestDelivererRetriesThenFails: a push service that keeps failing is
// retried until MaxAttempts, then the delivery fails.
func TestDelivererRetriesThenFails(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	table := testpg.PrivateTable(t, pool, "push_deliveries")
	org, pwa := idgen.New(), idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, org, "Pushsend "+org); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testpg.DeleteOrganization(context.Background(), pool, org) })
	if _, err := pool.Exec(ctx, `INSERT INTO pwas (id, organization_id, name, short_name, theme_color, background_color, icon_url, start_url)
		VALUES ($1, $2, 'App', 'App', '#000000', '#ffffff', '', '/')`, pwa, org); err != nil {
		t.Fatal(err)
	}
	repo := push.NewRepositoryOnTable(pool, table)
	endpoint := "https://push.invalid/" + idgen.New()
	if _, err := repo.SaveSubscription(ctx, push.NewSubscription{OrganizationID: org, PwaID: pwa, Endpoint: endpoint,
		P256dh: "k", Auth: "a", LinkToken: "t", Context: []byte(`{}`), Origin: "o"}); err != nil {
		t.Fatal(err)
	}
	b, err := push.NewService(repo, true).CreateBroadcast(ctx, org, "", push.CreateBroadcastInput{Name: "Promo", Title: "Hi", Target: push.TargetDestination})
	if err != nil {
		t.Fatal(err)
	}
	d := pushsend.NewDeliverer(pushsend.NewPostgresStoreOnTable(pool, table), &fakeSender{codes: map[string]int{endpoint: 503}}, &fakeEvents{},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	for i := 0; i < pushsend.MaxAttempts; i++ {
		if _, err := pool.Exec(ctx, `UPDATE `+table+` SET next_attempt_at = now() WHERE broadcast_id = $1`, b.ID); err != nil {
			t.Fatal(err)
		}
		if n, err := d.RunOnce(ctx, 10); err != nil || n != 1 {
			t.Fatalf("attempt %d: RunOnce = %d, %v", i+1, n, err)
		}
	}
	var status string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT delivery_status, attempt_count FROM `+table+` WHERE broadcast_id = $1`, b.ID).Scan(&status, &attempts); err != nil ||
		status != "dead" || attempts != pushsend.MaxAttempts {
		t.Fatalf("after %d attempts: %s/%d, %v", pushsend.MaxAttempts, status, attempts, err)
	}
	got, _ := push.NewService(repo, true).GetBroadcast(ctx, org, b.ID)
	if got.Failed != 1 || got.Delivered != 0 {
		t.Fatalf("counters = %+v", got)
	}
}
