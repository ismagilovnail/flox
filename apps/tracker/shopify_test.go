package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/clientip"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/ratelimit"
	"github.com/ismagilovnail/flox/apps/internal/shopconn"
	"github.com/ismagilovnail/flox/apps/internal/shopify"
)

// knownClicks attributes a conversion to a click it was given.
type knownClicks map[string]bool

func (k knownClicks) AttributeConversion(_ context.Context, c attribution.Conversion) (attribution.Attribution, error) {
	if !k[c.ClickID] {
		return attribution.Attribution{Outcome: attribution.OutcomeUnknownClick}, nil
	}
	return attribution.Attribution{Outcome: attribution.OutcomeAttributed, Method: attribution.MethodClickID,
		Click: attribution.Click{ClickID: c.ClickID, OrganizationID: c.OrganizationID}}, nil
}

func shopifyPool(t *testing.T) *pgxpool.Pool {
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
	return pool
}

// Audit UNI-4 part 3: a connected Shopify store's webhooks, on the real
// Postgres and conversion engine. Only a webhook signed with the store's
// key, from that store, is taken; a paid order is a purchase, the click's
// next order a repeat purchase; a replay records nothing; a refund
// reverses what was paid back of that order; test orders, unknown orders
// and other topics are acknowledged and left.
func TestShopifyWebhooks(t *testing.T) {
	f := newShopFixture(t)
	pool, ctx, orgID, networkID, partnerID, click, other := f.pool, f.ctx, f.orgID, f.networkID, f.partnerID, f.click, f.other
	settings := shopconn.NewService(f.store, shopify.Platform, "https://t.example")
	if _, err := settings.Put(ctx, orgID, partnerID, shopconn.Input{ShopDomain: "my-shop.myshopify.com", Secret: "k"}); err == nil {
		t.Fatal("connected a store to a partner network")
	}
	if _, err := settings.Put(ctx, orgID, networkID, shopconn.Input{ShopDomain: "my-shop.example.com", Secret: "k"}); err == nil {
		t.Fatal("took a domain that is not myshopify.com")
	}
	view, err := settings.Put(ctx, orgID, networkID, shopconn.Input{ShopDomain: "https://My-Shop.myshopify.com/", Secret: "shpss_signing_key"})
	if err != nil {
		t.Fatal(err)
	}
	if !view.Connected || view.ShopDomain != "my-shop.myshopify.com" || view.SecretPreview != "_key" || view.WebhookURL != "https://t.example/shopify/"+networkID {
		t.Fatalf("settings = %+v", view)
	}
	if _, err := settings.Get(ctx, idgen.New(), networkID); err == nil {
		t.Fatal("another organization read the store connection")
	}

	send := func(network, topic string, payload any, secret, shop string) (int, postbackResponse) {
		t.Helper()
		body, _ := json.Marshal(payload)
		return f.send("/shopify/"+network, body, map[string]string{
			"X-Shopify-Topic": topic, "X-Shopify-Shop-Domain": shop, "X-Shopify-Hmac-Sha256": shopify.Sign(body, secret)})
	}
	const secret, shop = "shpss_signing_key", "my-shop.myshopify.com"
	at := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	order := func(id int, extra map[string]any) map[string]any {
		o := map[string]any{"id": id, "name": fmt.Sprintf("#%d", id), "currency": "RUB", "total_price": "1990.00",
			"processed_at": at, "test": false, "note_attributes": []any{}}
		for k, v := range extra {
			o[k] = v
		}
		return o
	}
	withAttr := map[string]any{"note_attributes": []map[string]any{{"name": "gift", "value": "no"}, {"name": "flox_click_id", "value": click}}}
	rows := f.rows

	// Refused before anything is read.
	if code, _ := send(networkID, shopify.TopicOrderPaid, order(1, withAttr), "wrong-key", shop); code != http.StatusUnauthorized {
		t.Fatalf("a wrong signature: %d", code)
	}
	if code, _ := send(networkID, shopify.TopicOrderPaid, order(1, withAttr), secret, "other.myshopify.com"); code != http.StatusUnauthorized {
		t.Fatalf("another store: %d", code)
	}
	if code, _ := send(partnerID, shopify.TopicOrderPaid, order(1, withAttr), secret, shop); code != http.StatusNotFound {
		t.Fatalf("a source without a store: %d", code)
	}

	// The first order is a purchase; its replay records nothing.
	if code, res := send(networkID, shopify.TopicOrderPaid, order(1001, withAttr), secret, shop); code != 200 || res.Result != "success" {
		t.Fatalf("first order: %d %+v", code, res)
	}
	if code, res := send(networkID, shopify.TopicOrderPaid, order(1001, withAttr), secret, shop); code != 200 || res.Result != "ok" {
		t.Fatalf("replayed order: %d %+v", code, res)
	}
	if n, rev := rows("CPA_ACCEPT"); n != 1 || rev != 1990 {
		t.Fatalf("purchases: %d, %v", n, rev)
	}
	// The click's next order, found by the landing page's click_id, is a
	// repeat purchase.
	landing := map[string]any{"landing_site": "/collections/all?utm_source=x&click_id=" + click, "total_price": "990.00"}
	if code, res := send(networkID, shopify.TopicOrderPaid, order(1002, landing), secret, shop); code != 200 || res.Result != "success" {
		t.Fatalf("second order: %d %+v", code, res)
	}
	if n, rev := rows("CPA_REDEP"); n != 1 || rev != 990 {
		t.Fatalf("repeat purchases: %d, %v", n, rev)
	}
	var txn string
	if err := pool.QueryRow(ctx, `SELECT network_txn_id FROM postbacks WHERE organization_id = $1 AND status = 'CPA_REDEP'`, orgID).Scan(&txn); err != nil || txn != "1002" {
		t.Fatalf("repeat purchase txn %q (%v)", txn, err)
	}

	// Acknowledged and left: a test order, another topic, an order without
	// a click (the engine's refusal, in the postback log).
	if code, res := send(networkID, shopify.TopicOrderPaid, order(1003, map[string]any{"test": true, "landing_site": "/?click_id=" + other}), secret, shop); code != 200 || res.Message != "ignored_test_order" {
		t.Fatalf("test order: %d %+v", code, res)
	}
	if code, _ := send(networkID, "products/update", map[string]any{"id": 1}, secret, shop); code != 200 {
		t.Fatalf("another topic: %d", code)
	}
	if code, res := send(networkID, shopify.TopicOrderPaid, order(1004, nil), secret, shop); code != 200 || res.Result == "success" {
		t.Fatalf("order without a click: %d %+v", code, res)
	}

	// A refund of the repeat purchase: what was paid back, not more; its
	// replay is a duplicate; a refund of an order FLOX did not record is
	// left.
	refund := map[string]any{"id": 77, "order_id": 1002, "processed_at": at, "transactions": []map[string]any{
		{"amount": "500.00", "kind": "refund", "status": "success", "currency": "RUB"},
		{"amount": "490.00", "kind": "refund", "status": "failure", "currency": "RUB"},
	}}
	if code, res := send(networkID, shopify.TopicRefundCreate, refund, secret, shop); code != 200 || res.Result != "success" {
		t.Fatalf("refund: %d %+v", code, res)
	}
	var reversed float64
	var kind string
	if err := pool.QueryRow(ctx, `SELECT reversal_revenue::float8, refund_kind FROM postbacks
		WHERE organization_id = $1 AND status = 'CPA_DECLINE' AND result = 'success'`, orgID).Scan(&reversed, &kind); err != nil || reversed != 500 || kind != "repeat" {
		t.Fatalf("refund recorded as %v %q (%v), want 500 of the repeat purchase", reversed, kind, err)
	}
	if code, res := send(networkID, shopify.TopicRefundCreate, refund, secret, shop); code != 200 || res.Result != "duplicate" {
		t.Fatalf("replayed refund: %d %+v", code, res)
	}
	unknown := map[string]any{"id": 78, "order_id": 9999, "transactions": []map[string]any{{"amount": "5", "kind": "refund", "status": "success"}}}
	if code, res := send(networkID, shopify.TopicRefundCreate, unknown, secret, shop); code != 200 || res.Message != "ignored_unknown_order" {
		t.Fatalf("refund of an unknown order: %d %+v", code, res)
	}
}

// shopFixture is an organization with an own-site source (the own-site
// mapping), a partner network, two clicks the attribution knows, and the
// store webhook handler on the real Postgres and conversion engine.
type shopFixture struct {
	t                           *testing.T
	pool                        *pgxpool.Pool
	ctx                         context.Context
	orgID, networkID, partnerID string
	click, other                string
	clicks                      knownClicks // add to it to make more clicks known
	store                       *shopconn.Store
	router                      chi.Router
}

func newShopFixture(t *testing.T) *shopFixture {
	t.Helper()
	pool := shopifyPool(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	orgID, networkID, partnerID := idgen.New(), idgen.New(), idgen.New()
	exec(`INSERT INTO organizations (id, name) VALUES ($1, $2)`, orgID, "Org "+orgID)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID) })
	exec(`INSERT INTO networks (id, organization_id, name, template_key, currency) VALUES ($1, $3, 'Shop', 'own_site', 'RUB'), ($2, $3, 'Partner', '', 'USD')`,
		networkID, partnerID, orgID)
	for raw, flox := range map[string]string{"purchase": "CPA_ACCEPT", "repeat_purchase": "CPA_REDEP", "refund": "CPA_DECLINE"} {
		exec(`INSERT INTO event_mappings (id, organization_id, network_id, network_status, flox_status) VALUES ($1, $2, $3, $4, $5)`,
			idgen.New(), orgID, networkID, raw, flox)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	store := shopconn.NewStore(pool, key)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver, _ := clientip.New(nil, logger)
	click, other := idgen.New(), idgen.New()
	clicks := knownClicks{click: true, other: true}
	h := &ShopWebhookHandler{
		shops:    store,
		networks: conversion.NewPostgresNetworkLookup(pool),
		service: conversion.NewService(conversion.NewPostgresMapper(pool), conversion.NewPostgresStore(pool), usdOnly{},
			clicks, noAttempts{}),
		failures: ratelimit.New(nil, logger).Failures("shop-test", 100, time.Minute),
		clientIP: resolver,
		logger:   logger,
	}
	r := chi.NewRouter()
	h.Register(r)
	return &shopFixture{t: t, pool: pool, ctx: ctx, orgID: orgID, networkID: networkID, partnerID: partnerID,
		click: click, other: other, clicks: clicks, store: store, router: r}
}

// send posts body to path with the headers.
func (f *shopFixture) send(path string, body []byte, headers map[string]string) (int, postbackResponse) {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
	req.RemoteAddr = "10.0.0.1:1"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var res postbackResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	return rec.Code, res
}

// rows is the organization's recorded postbacks of a status: how many,
// what revenue.
func (f *shopFixture) rows(status string) (n int, revenue float64) {
	f.t.Helper()
	_ = f.pool.QueryRow(f.ctx, `SELECT count(*), coalesce(sum(revenue), 0)::float8 FROM postbacks
		WHERE organization_id = $1 AND result = 'success' AND status = $2`, f.orgID, status).Scan(&n, &revenue)
	return n, revenue
}
