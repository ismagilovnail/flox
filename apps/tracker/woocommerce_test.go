package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/shopconn"
	"github.com/ismagilovnail/flox/apps/internal/shopify"
	"github.com/ismagilovnail/flox/apps/internal/woocommerce"
)

// Audit UNI-4 part 4: a connected WooCommerce store's webhooks, on the
// real Postgres and conversion engine. The activation ping is answered;
// only a webhook signed with the store's secret, from that store, is
// taken. The order comes with every update of it: an unpaid one records
// nothing, the paid one a purchase once, the click's next order a repeat
// purchase; each refund on it is recorded once, for what it paid back,
// and not logged again on the next update.
func TestWooCommerceWebhooks(t *testing.T) {
	f := newShopFixture(t)
	ctx, orgID, networkID, partnerID, click := f.ctx, f.orgID, f.networkID, f.partnerID, f.click
	settings := shopconn.NewService(f.store, woocommerce.Platform, "https://t.example")
	if _, err := settings.Put(ctx, orgID, partnerID, shopconn.Input{ShopDomain: "shop.example.com", Secret: "k"}); err == nil {
		t.Fatal("connected a store to a partner network")
	}
	if _, err := settings.Put(ctx, orgID, networkID, shopconn.Input{ShopDomain: "localhost", Secret: "k"}); err == nil {
		t.Fatal("took a site address without a domain")
	}
	view, err := settings.Put(ctx, orgID, networkID, shopconn.Input{ShopDomain: "https://Shop.Example.com/store/", Secret: "wc_secret_key"})
	if err != nil {
		t.Fatal(err)
	}
	if !view.Connected || view.ShopDomain != "shop.example.com/store" || view.SecretPreview != "_key" || view.WebhookURL != "https://t.example/woocommerce/"+networkID {
		t.Fatalf("settings = %+v", view)
	}
	if _, err := settings.Get(ctx, idgen.New(), networkID); err == nil {
		t.Fatal("another organization read the store connection")
	}
	// The source's Shopify settings are its own row.
	if v, err := shopconn.NewService(f.store, shopify.Platform, "").Get(ctx, orgID, networkID); err != nil || v.Connected {
		t.Fatalf("the WooCommerce store shows as a Shopify one: %+v %v", v, err)
	}

	const secret, source = "wc_secret_key", "https://shop.example.com/store/"
	send := func(network, topic string, payload any, secret, source string) (int, postbackResponse) {
		t.Helper()
		body, _ := json.Marshal(payload)
		return f.send("/woocommerce/"+network, body, map[string]string{
			"X-WC-Webhook-Topic": topic, "X-WC-Webhook-Source": source, "X-WC-Webhook-Signature": woocommerce.Sign(body, secret)})
	}
	paidAt := time.Now().Add(-time.Minute).UTC().Format("2006-01-02T15:04:05")
	order := func(id int, status string, paid bool, meta []map[string]any, refunds []map[string]any) map[string]any {
		o := map[string]any{"id": id, "status": status, "currency": "rub", "total": "1990.00",
			"date_paid_gmt": nil, "meta_data": meta, "refunds": refunds}
		if paid {
			o["date_paid_gmt"] = paidAt
		}
		return o
	}
	withMeta := []map[string]any{{"id": 1, "key": "flox_click_id", "value": click}}

	// The ping WooCommerce sends on activating the webhook.
	if code, res := f.send("/woocommerce/"+networkID, []byte("webhook_id=7"), nil); code != http.StatusOK || res.Message != "ping" {
		t.Fatalf("ping: %d %+v", code, res)
	}
	// Refused before anything is read.
	if code, _ := send(networkID, woocommerce.TopicOrderUpdated, order(1, "processing", true, withMeta, nil), "wrong", source); code != http.StatusUnauthorized {
		t.Fatalf("a wrong signature: %d", code)
	}
	if code, _ := send(networkID, woocommerce.TopicOrderUpdated, order(1, "processing", true, withMeta, nil), secret, "https://other.example.com/"); code != http.StatusUnauthorized {
		t.Fatalf("another store: %d", code)
	}
	if code, _ := send(partnerID, woocommerce.TopicOrderUpdated, order(1, "processing", true, withMeta, nil), secret, source); code != http.StatusNotFound {
		t.Fatalf("a source without a store: %d", code)
	}
	// A Shopify-signed delivery to the WooCommerce URL is not WooCommerce's.
	if code, _ := f.send("/shopify/"+networkID, []byte(`{}`), map[string]string{"X-Shopify-Topic": "orders/paid"}); code != http.StatusNotFound {
		t.Fatalf("the source's Shopify URL answered %d with no Shopify store", code)
	}

	// Created unpaid: nothing. Paid: a purchase. Updated again (shipped):
	// nothing more.
	if code, res := send(networkID, woocommerce.TopicOrderCreated, order(501, "pending", false, withMeta, nil), secret, source); code != 200 || res.Message != "ignored_unpaid" {
		t.Fatalf("unpaid order: %d %+v", code, res)
	}
	if code, res := send(networkID, woocommerce.TopicOrderUpdated, order(501, "processing", true, withMeta, nil), secret, source); code != 200 || res.Result != "success" {
		t.Fatalf("paid order: %d %+v", code, res)
	}
	if code, res := send(networkID, woocommerce.TopicOrderUpdated, order(501, "completed", true, withMeta, nil), secret, source); code != 200 || res.Result != "ok" || res.Message != "duplicate" {
		t.Fatalf("order updated again: %d %+v", code, res)
	}
	if n, rev := f.rows("CPA_ACCEPT"); n != 1 || rev != 1990 {
		t.Fatalf("purchases: %d, %v", n, rev)
	}
	// The click's next order, found by the visit's entry page (order
	// attribution), is a repeat purchase.
	entry := []map[string]any{{"key": "_wc_order_attribution_session_entry", "value": "https://shop.example.com/store/?click_id=" + click}}
	if code, res := send(networkID, woocommerce.TopicOrderCreated, order(502, "processing", true, entry, nil), secret, source); code != 200 || res.Result != "success" {
		t.Fatalf("second order: %d %+v", code, res)
	}
	if n, _ := f.rows("CPA_REDEP"); n != 1 {
		t.Fatalf("repeat purchases: %d", n)
	}

	// A partial refund of the first order arrives as an update with it;
	// the next update (a second refund) records only the new one; a replay
	// records nothing and logs no duplicate.
	r1 := []map[string]any{{"id": 90, "reason": "", "total": "-490.00"}}
	if code, res := send(networkID, woocommerce.TopicOrderUpdated, order(501, "completed", true, withMeta, r1), secret, source); code != 200 || res.Result != "success" {
		t.Fatalf("refund: %d %+v", code, res)
	}
	r2 := append([]map[string]any{{"id": 91, "reason": "", "total": "-500.00"}}, r1...)
	if code, res := send(networkID, woocommerce.TopicOrderUpdated, order(501, "completed", true, withMeta, r2), secret, source); code != 200 || res.Result != "success" {
		t.Fatalf("second refund: %d %+v", code, res)
	}
	if code, res := send(networkID, woocommerce.TopicOrderUpdated, order(501, "completed", true, withMeta, r2), secret, source); code != 200 || res.Message != "duplicate" {
		t.Fatalf("replayed refunds: %d %+v", code, res)
	}
	var reversed float64
	var refunds, logged int
	if err := f.pool.QueryRow(ctx, `SELECT count(*), coalesce(sum(reversal_revenue), 0)::float8 FROM postbacks
		WHERE organization_id = $1 AND status = 'CPA_DECLINE' AND result = 'success'`, orgID).Scan(&refunds, &reversed); err != nil || refunds != 2 || reversed != 990 {
		t.Fatalf("refunds %d reversing %v (%v), want 2 reversing 990", refunds, reversed, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM postbacks WHERE organization_id = $1 AND status = 'CPA_DECLINE'`, orgID).Scan(&logged); err != nil || logged != 2 {
		t.Fatalf("%d refund rows logged (%v), want 2: replays were sent to the engine", logged, err)
	}
	// Other topics are acknowledged and left.
	if code, res := send(networkID, "product.updated", map[string]any{"id": 1}, secret, source); code != 200 || res.Result != "ok" {
		t.Fatalf("another topic: %d %+v", code, res)
	}
}

// Two deliveries of one first order at once (WooCommerce sends
// order.created and order.updated for a checkout paid in the same request;
// either can be retried by hand): the one that loses the race comes back a
// duplicate purchase, and must not then be recorded as a repeat purchase
// of the same order.
func TestShopOrderRacingItself(t *testing.T) {
	f := newShopFixture(t)
	settings := shopconn.NewService(f.store, woocommerce.Platform, "")
	if _, err := settings.Put(f.ctx, f.orgID, f.networkID, shopconn.Input{ShopDomain: "shop.example.com", Secret: "k"}); err != nil {
		t.Fatal(err)
	}
	paidAt := time.Now().Add(-time.Minute).UTC().Format("2006-01-02T15:04:05")
	for i := range 20 {
		click := idgen.New()
		f.clicks[click] = true
		body, _ := json.Marshal(map[string]any{"id": 700 + i, "status": "processing", "currency": "RUB", "total": "100",
			"date_paid_gmt": paidAt, "meta_data": []map[string]any{{"key": "flox_click_id", "value": click}}})
		headers := map[string]string{"X-WC-Webhook-Topic": woocommerce.TopicOrderUpdated, "X-WC-Webhook-Signature": woocommerce.Sign(body, "k")}
		var wg sync.WaitGroup
		for range 2 {
			wg.Go(func() {
				if code, res := f.send("/woocommerce/"+f.networkID, body, headers); code != http.StatusOK {
					t.Errorf("order %d: %d %+v", 700+i, code, res)
				}
			})
		}
		wg.Wait()
	}
	if n, _ := f.rows("CPA_ACCEPT"); n != 20 {
		t.Fatalf("%d purchases, want 20", n)
	}
	if n, _ := f.rows("CPA_REDEP"); n != 0 {
		t.Fatalf("%d repeat purchases of orders racing themselves", n)
	}
}
