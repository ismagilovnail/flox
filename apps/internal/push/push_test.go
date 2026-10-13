package push_test

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
	"github.com/ismagilovnail/flox/apps/internal/push"
	"github.com/ismagilovnail/flox/apps/internal/testpg"
)

func TestValidEndpoint(t *testing.T) {
	for endpoint, want := range map[string]bool{
		"https://fcm.googleapis.com/fcm/send/abc":                 true,
		"https://updates.push.services.mozilla.com/wpush/v2/abc":  true,
		"https://web.push.apple.com/QGuQ":                         true,
		"https://wns2-by3p.notify.windows.com/w/?token=abc":       true,
		"http://fcm.googleapis.com/fcm/send/abc":                  false, // not https
		"https://fcm.googleapis.com:8443/x":                       false, // port
		"https://evil.example/fcm.googleapis.com":                 false,
		"https://fcm.googleapis.com.evil.example/x":               false,
		"https://169.254.169.254/latest/meta-data":                false,
		"https://user@fcm.googleapis.com/x":                       false,
		"https://fcm.googleapis.com/" + strings.Repeat("a", 2100): false,
	} {
		if got := push.ValidEndpoint(endpoint); got != want {
			t.Errorf("ValidEndpoint(%q) = %v, want %v", endpoint, got, want)
		}
	}
}

func TestValidKeys(t *testing.T) {
	p256 := base64.RawURLEncoding.EncodeToString(make([]byte, 65))
	auth := base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	if !push.ValidKeys(p256, auth) {
		t.Fatal("valid keys rejected")
	}
	if push.ValidKeys(auth, p256) || push.ValidKeys("not base64!", auth) || push.ValidKeys(p256, "") {
		t.Fatal("invalid keys accepted")
	}
}

type fixture struct {
	pool       *pgxpool.Pool
	repo       *push.Repository
	org        string
	pwa, camp  string
	deliveries string
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
	table := testpg.PrivateTable(t, pool, "push_deliveries")
	f := &fixture{pool: pool, repo: push.NewRepositoryOnTable(pool, table), deliveries: table}
	f.org, f.pwa, f.camp = f.seedOrg(t)
	return f
}

// seedOrg creates an organization with a PWA; the campaign id is just a
// value subscriptions carry.
func (f *fixture) seedOrg(t *testing.T) (org, pwa, camp string) {
	t.Helper()
	ctx := context.Background()
	org, pwa, camp = idgen.New(), idgen.New(), idgen.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, org, "Push Org "+org); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testpg.DeleteOrganization(context.Background(), f.pool, org) })
	if _, err := f.pool.Exec(ctx, `INSERT INTO pwas (id, organization_id, name, short_name, theme_color, background_color, icon_url, start_url)
		VALUES ($1, $2, 'App', 'App', '#000000', '#ffffff', 'https://cdn.example/i.png', '/')`, pwa, org); err != nil {
		t.Fatal(err)
	}
	return org, pwa, camp
}

// subscribe stores a subscription; endpoints are on push.invalid, which
// never resolves.
func (f *fixture) subscribe(t *testing.T, org, pwa, camp, country string) (string, string) {
	t.Helper()
	endpoint := "https://push.invalid/" + idgen.New()
	click := idgen.New()
	created, err := f.repo.SaveSubscription(context.Background(), push.NewSubscription{
		OrganizationID: org, PwaID: pwa, CampaignID: camp, ClickID: click, Endpoint: endpoint,
		P256dh: "k", Auth: "a", LinkToken: "tok", Context: []byte(`{}`), Origin: "https://t.example", Country: country,
	})
	if err != nil || !created {
		t.Fatalf("SaveSubscription = %v, %v", created, err)
	}
	return endpoint, click
}

func TestSaveSubscription(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	endpoint, _ := f.subscribe(t, f.org, f.pwa, f.camp, "US")

	// Again (the app re-subscribing): refreshed, not new.
	again, err := f.repo.SaveSubscription(ctx, push.NewSubscription{
		OrganizationID: f.org, PwaID: f.pwa, Endpoint: endpoint, P256dh: "k2", Auth: "a2", LinkToken: "t", Context: []byte(`{}`), Origin: "o",
	})
	if err != nil || again {
		t.Fatalf("re-subscribe = %v, %v, want not new", again, err)
	}

	// Another organization's PWA, or the same endpoint from another
	// organization: nothing stored, nothing taken over.
	otherOrg, otherPwa, _ := f.seedOrg(t)
	for _, s := range []push.NewSubscription{
		{OrganizationID: otherOrg, PwaID: f.pwa, Endpoint: "https://push.invalid/" + idgen.New()},
		{OrganizationID: otherOrg, PwaID: otherPwa, Endpoint: endpoint},
	} {
		s.P256dh, s.Auth, s.LinkToken, s.Context, s.Origin = "k", "a", "t", []byte(`{}`), "o"
		if created, err := f.repo.SaveSubscription(ctx, s); err != nil || created {
			t.Fatalf("cross-tenant SaveSubscription = %v, %v", created, err)
		}
	}
	var org, p256 string
	if err := f.pool.QueryRow(ctx, `SELECT organization_id, p256dh FROM push_subscriptions WHERE endpoint = $1`, endpoint).Scan(&org, &p256); err != nil || org != f.org || p256 != "k2" {
		t.Fatalf("subscription = %s/%s, %v", org, p256, err)
	}
	if list, _, _ := f.repo.ListSubscriptions(ctx, otherOrg, push.SubscriptionFilter{}, pagination.Page{Limit: 10}); len(list) != 0 {
		t.Fatalf("other organization sees %d subscriptions", len(list))
	}
}

func TestBroadcastAudienceAndOpens(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, clickUS := f.subscribe(t, f.org, f.pwa, f.camp, "US")
	f.subscribe(t, f.org, f.pwa, f.camp, "DE")
	f.subscribe(t, f.org, f.pwa, idgen.New(), "US")
	otherOrg, otherPwa, otherCamp := f.seedOrg(t)
	f.subscribe(t, otherOrg, otherPwa, otherCamp, "US")

	for filters, want := range map[*push.Filters]int{
		{}:                          3,
		{Countries: []string{"US"}}: 2,
		{CampaignIDs: []string{f.camp}, Countries: []string{"US"}}: 1,
		{PwaIDs: []string{otherPwa}}:                               0, // another organization's PWA selects nothing here
	} {
		if n, err := f.repo.CountAudience(ctx, f.org, *filters); err != nil || n != want {
			t.Fatalf("CountAudience(%+v) = %d, %v, want %d", *filters, n, err, want)
		}
	}

	svc := push.NewService(f.repo, true)
	b, err := svc.CreateBroadcast(ctx, f.org, "", push.CreateBroadcastInput{
		Name: "Weekend", Title: "Hi", Target: push.TargetURL, URL: "https://lp.example/?c={click_id}",
		Filters: push.Filters{Countries: []string{"US"}, CampaignIDs: []string{f.camp}},
	})
	if err != nil || b.Total != 1 {
		t.Fatalf("CreateBroadcast = %+v, %v", b, err)
	}
	var deliveryID string
	if err := f.pool.QueryRow(ctx, `SELECT id FROM `+f.deliveries+` WHERE broadcast_id = $1`, b.ID).Scan(&deliveryID); err != nil {
		t.Fatalf("delivery: %v", err)
	}

	// Another click, or another organization, cannot count an open.
	for _, c := range []struct{ org, click string }{{f.org, idgen.New()}, {otherOrg, clickUS}} {
		if _, found, err := f.repo.RecordOpen(ctx, c.org, c.click, deliveryID); err != nil || found {
			t.Fatalf("foreign RecordOpen found=%v err=%v", found, err)
		}
	}
	for i, wantFirst := range []bool{true, false} {
		open, found, err := f.repo.RecordOpen(ctx, f.org, clickUS, deliveryID)
		if err != nil || !found || open.First != wantFirst || open.Target != push.TargetURL || open.URL != "https://lp.example/?c={click_id}" {
			t.Fatalf("RecordOpen #%d = %+v, %v, %v", i, open, found, err)
		}
	}
	if got, _ := svc.GetBroadcast(ctx, f.org, b.ID); got.Opened != 1 {
		t.Fatalf("opened = %d, want 1 (counted once)", got.Opened)
	}
	var nf *apierror.Error
	if _, err := svc.GetBroadcast(ctx, otherOrg, b.ID); !errors.As(err, &nf) || nf.Status != 404 {
		t.Fatalf("other organization reads the broadcast: %v", err)
	}
	if list, total, _ := svc.ListBroadcasts(ctx, otherOrg, pagination.Page{Limit: 10}); total != 0 || len(list) != 0 {
		t.Fatalf("other organization lists %d broadcasts", total)
	}

	sum, err := svc.Summary(ctx, f.org)
	if err != nil || sum.Active != 3 || len(sum.ByCountry) != 2 || sum.ByCountry[0].Key != "US" || sum.ByCountry[0].Count != 2 {
		t.Fatalf("Summary = %+v, %v", sum, err)
	}
}

func TestCreateBroadcastValidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	svc := push.NewService(f.repo, true)
	valid := push.CreateBroadcastInput{Name: "Promo", Title: "Hi", Target: push.TargetDestination}

	var verr *apierror.Error
	if _, err := svc.CreateBroadcast(ctx, f.org, "", valid); !errors.As(err, &verr) || verr.Fields["filters"] == "" {
		t.Fatalf("no subscribers: %v, want a filters error", err)
	}
	f.subscribe(t, f.org, f.pwa, f.camp, "US")
	for name, mutate := range map[string]func(*push.CreateBroadcastInput){
		"title":     func(in *push.CreateBroadcastInput) { in.Title = "" },
		"name":      func(in *push.CreateBroadcastInput) { in.Name = "x" },
		"body":      func(in *push.CreateBroadcastInput) { in.Body = strings.Repeat("b", 241) },
		"iconUrl":   func(in *push.CreateBroadcastInput) { in.IconURL = "http://cdn.example/i.png" },
		"url":       func(in *push.CreateBroadcastInput) { in.Target, in.URL = push.TargetURL, "javascript:alert(1)" },
		"target":    func(in *push.CreateBroadcastInput) { in.Target = "nowhere" },
		"countries": func(in *push.CreateBroadcastInput) { in.Filters.Countries = []string{"usa"} },
		"pwaIds":    func(in *push.CreateBroadcastInput) { in.Filters.PwaIDs = []string{"' OR 1=1"} },
	} {
		in := valid
		mutate(&in)
		if _, err := svc.CreateBroadcast(ctx, f.org, "", in); !errors.As(err, &verr) || verr.Fields[name] == "" {
			t.Fatalf("%s: err = %v, want a %s field error", name, err, name)
		}
	}
	if _, err := push.NewService(f.repo, false).CreateBroadcast(ctx, f.org, "", valid); !errors.As(err, &verr) || verr.Status != 503 {
		t.Fatalf("unconfigured: %v, want 503", err)
	}
	if b, err := svc.CreateBroadcast(ctx, f.org, "", valid); err != nil || b.Total != 1 {
		t.Fatalf("valid broadcast: %+v, %v", b, err)
	}
}
