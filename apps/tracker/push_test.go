package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/push"
	"github.com/ismagilovnail/flox/apps/internal/streamset"
	"github.com/ismagilovnail/flox/apps/internal/testpg"
)

const testVAPIDKey = "BPushTestPublicKey"

func withPush(repo *push.Repository, key string) func(*Handler) {
	return func(h *Handler) {
		h.push = repo
		h.vapidPublicKey = key
	}
}

func postJSON(h http.Handler, host, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "http://"+host+path, bytes.NewBufferString(body))
	req.Host = host
	req.RemoteAddr = "198.51.100.7:12345"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func subscriptionJSON(endpoint string) string {
	p256 := base64.RawURLEncoding.EncodeToString(append([]byte{4}, make([]byte, 64)...))
	auth := base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	return `{"endpoint":"` + endpoint + `","keys":{"p256dh":"` + p256 + `","auth":"` + auth + `"}}`
}

// pushFixture: a flow with an internal PWA whose push switch is on, and a
// push repository over a private push_deliveries.
func pushFixture(t *testing.T, pool *pgxpool.Pool) (landingFixture, string, *push.Repository, string) {
	t.Helper()
	var pwaID string
	fx := seedFlowFixture(t, pool, nil, withPwa(t, pool, &pwaID, streamset.PwaTypeInternal, false))
	if _, err := pool.Exec(context.Background(), `UPDATE pwas SET push_enabled = true WHERE id = $1`, pwaID); err != nil {
		t.Fatal(err)
	}
	table := testpg.PrivateTable(t, pool, "push_deliveries")
	return fx, pwaID, push.NewRepositoryOnTable(pool, table), table
}

// TestPushLaunchAndSubscribe: the installed app's launch page asks for
// notifications; request/decline/subscribe record their events on the
// install click, the subscription is stored once, and the app continues.
func TestPushLaunchAndSubscribe(t *testing.T) {
	pool := testPool(t)
	fx, pwaID, repo, _ := pushFixture(t, pool)
	h, sink, closeEvents := newLandingHandler(t, pool, true, withPush(repo, testVAPIDKey))

	base := strings.TrimPrefix(get(h, fx.host, "/t/go").Header().Get("Location"), "http://"+fx.host)
	launch := get(h, fx.host, base+"open")
	if launch.Code != http.StatusOK || !strings.Contains(launch.Body.String(), `"`+testVAPIDKey+`"`) || !strings.Contains(launch.Body.String(), `push/subscribe`) {
		t.Fatalf("launch page = %d:\n%s", launch.Code, launch.Body)
	}
	assertRedirect(t, get(h, fx.host, base+"go"), landingTestOffer)

	if rec := post(h, fx.host, base+"push/request"); rec.Code != http.StatusNoContent {
		t.Fatalf("request = %d", rec.Code)
	}
	if rec := post(h, fx.host, base+"push/decline"); rec.Code != http.StatusNoContent {
		t.Fatalf("decline = %d", rec.Code)
	}
	endpoint := "https://fcm.googleapis.com/fcm/send/flox-test-" + idgen.New()
	for i := 0; i < 2; i++ {
		if rec := postJSON(h, fx.host, base+"push/subscribe", subscriptionJSON(endpoint)); rec.Code != http.StatusNoContent {
			t.Fatalf("subscribe #%d = %d", i+1, rec.Code)
		}
	}
	for name, body := range map[string]string{
		"unknown push service": subscriptionJSON("https://evil.example/push"),
		"plain http":           subscriptionJSON("http://fcm.googleapis.com/fcm/send/x"),
		"bad keys":             `{"endpoint":"https://fcm.googleapis.com/fcm/send/x","keys":{"p256dh":"x","auth":"y"}}`,
		"not json":             `nope`,
	} {
		if rec := postJSON(h, fx.host, base+"push/subscribe", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: subscribe = %d, want 400", name, rec.Code)
		}
	}

	closeEvents()
	got := eventsByType(sink)
	src := got[event.SourceClick][0]
	for typ, n := range map[event.Type]int{
		event.PwaOpen: 1, event.NotificationRequest: 1, event.NotificationDecline: 1, event.NotificationSubscribe: 1,
	} {
		if len(got[typ]) != n {
			t.Fatalf("%s × %d, want %d", typ, len(got[typ]), n)
		}
		if e := got[typ][0]; e.ClickID != src.ClickID || e.PwaID != pwaID {
			t.Fatalf("%s on click %q pwa %q", typ, e.ClickID, e.PwaID)
		}
	}
	var org, click, origin string
	if err := pool.QueryRow(context.Background(), `SELECT organization_id, click_id, origin FROM push_subscriptions WHERE endpoint = $1`, endpoint).
		Scan(&org, &click, &origin); err != nil || org != fx.orgID || click != src.ClickID || origin != "http://"+fx.host {
		t.Fatalf("stored subscription = %s/%s/%s, %v", org, click, origin, err)
	}
}

// TestPushNotAsked: no launch page without a VAPID key, or for a PWA with
// push off.
func TestPushNotAsked(t *testing.T) {
	pool := testPool(t)
	fx, pwaID, repo, _ := pushFixture(t, pool)

	h, _, closeEvents := newLandingHandler(t, pool, true, withPush(repo, ""))
	base := strings.TrimPrefix(get(h, fx.host, "/t/go").Header().Get("Location"), "http://"+fx.host)
	assertRedirect(t, get(h, fx.host, base+"open"), landingTestOffer)
	closeEvents()

	if _, err := pool.Exec(context.Background(), `UPDATE pwas SET push_enabled = false WHERE id = $1`, pwaID); err != nil {
		t.Fatal(err)
	}
	h2, _, closeEvents2 := newLandingHandler(t, pool, true, withPush(repo, testVAPIDKey))
	base = strings.TrimPrefix(get(h2, fx.host, "/t/go").Header().Get("Location"), "http://"+fx.host)
	assertRedirect(t, get(h2, fx.host, base+"open"), landingTestOffer)
	closeEvents2()
}

// TestPushClick: a notification's click counts the open once, records
// NOTIFICATION_CLICK and leads to the broadcast's URL with the click's
// macros; a delivery that is not this click's still leads on.
func TestPushClick(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	fx, _, repo, table := pushFixture(t, pool)
	h, sink, closeEvents := newLandingHandler(t, pool, true, withPush(repo, testVAPIDKey))

	base := strings.TrimPrefix(get(h, fx.host, "/t/go?sub1=s%201").Header().Get("Location"), "http://"+fx.host)
	if rec := postJSON(h, fx.host, base+"push/subscribe", subscriptionJSON("https://fcm.googleapis.com/fcm/send/flox-test-"+idgen.New())); rec.Code != http.StatusNoContent {
		t.Fatalf("subscribe = %d", rec.Code)
	}
	b, err := push.NewService(repo, true).CreateBroadcast(ctx, fx.orgID, "", push.CreateBroadcastInput{
		Name: "Promo", Title: "Hi", Target: push.TargetURL, URL: "https://lp.example/go?cid={click_id}&s={sub1}",
	})
	if err != nil {
		t.Fatal(err)
	}
	var delivery string
	if err := pool.QueryRow(ctx, `SELECT id FROM `+table+` WHERE broadcast_id = $1`, b.ID).Scan(&delivery); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		rec := get(h, fx.host, base+"push/click/"+delivery)
		u, _ := url.Parse(rec.Header().Get("Location"))
		if rec.Code != http.StatusFound || u.Host != "lp.example" || u.Query().Get("cid") == "" || u.Query().Get("s") != "s 1" {
			t.Fatalf("click #%d = %d → %q", i+1, rec.Code, rec.Header().Get("Location"))
		}
	}
	assertRedirect(t, get(h, fx.host, base+"push/click/"+idgen.New()), landingTestOffer)

	got, _ := push.NewService(repo, true).GetBroadcast(ctx, fx.orgID, b.ID)
	if got.Opened != 1 {
		t.Fatalf("opened = %d, want 1", got.Opened)
	}
	closeEvents()
	if n := len(eventsByType(sink)[event.NotificationClick]); n != 3 {
		t.Fatalf("NOTIFICATION_CLICK × %d, want 3 (every click)", n)
	}
}
