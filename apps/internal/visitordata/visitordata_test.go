package visitordata_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/visitordata"
)

func connect(t *testing.T) (*pgxpool.Pool, driver.Conn) {
	t.Helper()
	if os.Getenv("DATABASE_URL") == "" || os.Getenv("CLICKHOUSE_URL") == "" {
		t.Skip("needs DATABASE_URL and CLICKHOUSE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	db := os.Getenv("CLICKHOUSE_DATABASE")
	if db == "" {
		db = "flox"
	}
	user := os.Getenv("CLICKHOUSE_USER")
	if user == "" {
		user = "flox"
	}
	ch, err := chconn.NewConn(ctx, config.ClickHouseConfig{URL: os.Getenv("CLICKHOUSE_URL"), Database: db, User: user, Password: os.Getenv("CLICKHOUSE_PASSWORD")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ch.Close() })
	return pool, ch
}

// Audit 2026-10-15, GDPR-1: a visitor's data request. Found by IP (every
// click from it) or by click id, scoped to the organization; erasure
// deletes the visitor's clicks, visits, push subscription, Telegram user
// and pixel sends, and keeps the organization's conversions.
func TestFindAndEraseAVisitor(t *testing.T) {
	pool, ch := connect(t)
	ctx := context.Background()
	orgA, orgB := idgen.New(), idgen.New()
	for _, org := range []string{orgA, orgB} {
		if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, 'visitor data test')`, org); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = ANY($1)`, []string{orgA, orgB})
		wait := clickhouse.Context(context.Background(), clickhouse.WithSettings(clickhouse.Settings{"mutations_sync": 2}))
		for _, table := range []string{"click_events", "tracking_events", "conversion_events", "click_lookup", "ltv_events"} {
			_ = ch.Exec(wait, "DELETE FROM "+table+" WHERE organization_id IN ?", []string{orgA, orgB})
		}
	})

	const ip, otherIP = "198.51.100.7", "198.51.100.8"
	first, second, stranger, otherOrg := idgen.New(), idgen.New(), idgen.New(), idgen.New()
	at := time.Now().UTC().Add(-time.Hour)
	click := func(org, id, from string) event.Event {
		return event.Event{Type: event.SourceClick, EventAt: at, OrganizationID: org, CampaignID: "c", ClickID: id,
			IP: from, UserAgent: "UA", Referrer: "https://ref.example/?email=x@example.com", Country: "DE"}
	}
	if err := chstore.NewEventStore(ch).InsertBatch(ctx, []event.Event{
		click(orgA, first, ip), click(orgA, second, ip), click(orgA, stranger, otherIP), click(orgB, otherOrg, ip),
		{Type: event.LandView, EventAt: at, OrganizationID: orgA, CampaignID: "c", ClickID: first},
		{ID: idgen.New(), Type: event.CpaAccept, EventAt: at, OrganizationID: orgA, CampaignID: "c", ClickID: first, Revenue: 25, Currency: "USD"},
	}); err != nil {
		t.Fatal(err)
	}
	pwaID := idgen.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO pwas (id, organization_id, name, short_name, theme_color, background_color, icon_url, start_url)
	      VALUES ($1, $2, 'p', 'p', '#000000', '#ffffff', 'https://cdn.example/i.png', '/')`, pwaID, orgA)
	exec(`INSERT INTO push_subscriptions (id, organization_id, pwa_id, click_id, endpoint, p256dh, auth, link_token, origin)
	      VALUES ($1, $2, $3, $4, 'https://fcm.googleapis.com/fcm/send/secret-token', 'k', 'a', $5, 'https://t.example')`, idgen.New(), orgA, pwaID, first, idgen.New())

	svc := visitordata.NewService(pool, ch)
	var apiErr *apierror.Error
	if _, err := svc.Find(ctx, orgA, "", ""); !errors.As(err, &apiErr) || apiErr.Status != 422 {
		t.Fatalf("Find with nothing = %v, want 422", err)
	}
	if _, err := svc.Find(ctx, orgA, "", "not-an-ip"); !errors.As(err, &apiErr) || apiErr.Status != 422 {
		t.Fatalf("Find with a bad IP = %v, want 422", err)
	}

	rep, err := svc.Find(ctx, orgA, "", ip)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.ClickIDs) != 2 || len(rep.Clicks) != 2 || rep.Truncated {
		t.Fatalf("by IP: click ids %v, %d clicks; want this organization's two from the IP", rep.ClickIDs, len(rep.Clicks))
	}
	for _, id := range rep.ClickIDs {
		if id == otherOrg || id == stranger {
			t.Fatalf("by IP found %s: another organization's or another IP's click", id)
		}
	}
	if len(rep.Events) != 1 || len(rep.Conversions) != 1 || len(rep.PushSubscriptions) != 1 ||
		rep.PushSubscriptions[0].EndpointHost != "fcm.googleapis.com" || rep.Clicks[0].Referrer == "" {
		t.Fatalf("report = %+v", rep)
	}
	if rep, err := svc.Find(ctx, orgB, first, ""); err != nil || len(rep.Clicks) != 0 {
		t.Fatalf("another organization finds the click: %+v, %v", rep, err)
	}

	out, err := svc.Erase(ctx, orgA, rep.ClickIDs)
	if err != nil {
		t.Fatalf("Erase: %v", err)
	}
	if out.PushSubscriptions != 1 {
		t.Fatalf("erased %+v, want the push subscription", out)
	}
	count := func(table, org, id string) uint64 {
		t.Helper()
		var n uint64
		if err := ch.QueryRow(ctx, "SELECT count() FROM "+table+" WHERE organization_id = ? AND click_id = ?", org, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, table := range []string{"click_events", "tracking_events", "click_lookup"} {
		if n := count(table, orgA, first) + count(table, orgA, second); n != 0 {
			t.Errorf("%s keeps %d rows of the erased visitor", table, n)
		}
	}
	if count("click_events", orgA, stranger) != 1 || count("click_events", orgB, otherOrg) != 1 {
		t.Error("erasure took another visitor's or another organization's click")
	}
	// Another organization naming this one's click id erases nothing.
	if _, err := svc.Erase(ctx, orgB, []string{stranger}); err != nil {
		t.Fatal(err)
	}
	if count("click_events", orgA, stranger) != 1 {
		t.Error("another organization erased this organization's click")
	}
	if count("conversion_events", orgA, first) != 1 {
		t.Error("the organization's conversion was erased; it is kept")
	}
	if rep, err := svc.Find(ctx, orgA, "", ip); err != nil || len(rep.ClickIDs) != 0 {
		t.Fatalf("after erasure the IP still finds %+v, %v", rep, err)
	}
	if _, err := svc.Erase(ctx, orgA, nil); !errors.As(err, &apiErr) || apiErr.Status != 422 {
		t.Fatalf("Erase of nothing = %v, want 422", err)
	}
}
