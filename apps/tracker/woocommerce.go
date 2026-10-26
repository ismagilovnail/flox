package main

import (
	"net/http"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/woocommerce"
)

// woocommerce takes a WooCommerce store's order webhooks (audit UNI-4
// part 4): order.created and order.updated carry the whole order, so one
// update can be the payment, a refund, or nothing FLOX records (a note, a
// shipping status). A paid order is a purchase or a repeat purchase once;
// each refund on it, once. Signed in X-WC-Webhook-Signature; the store is
// X-WC-Webhook-Source (its home_url).
func (h *ShopWebhookHandler) woocommerce(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, shopFormat{
		platform:     woocommerce.Platform,
		topicHeader:  "X-WC-Webhook-Topic",
		sourceHeader: "X-WC-Webhook-Source",
		verify:       headerSignature("X-WC-Webhook-Signature", woocommerce.Verify),
		ping: func(r *http.Request, body []byte) bool {
			return woocommerce.IsPing(body, r.Header.Get("X-WC-Webhook-Signature"))
		},
		topics: []string{woocommerce.TopicOrderCreated, woocommerce.TopicOrderUpdated},
		handle: h.wooOrder,
	})
}

func (h *ShopWebhookHandler) wooOrder(r *http.Request, network conversion.Network, _ string, body []byte) (shopAnswer, error) {
	ctx := r.Context()
	o, err := woocommerce.ParseOrder(body)
	if err != nil {
		return shopAck("bad_request", ""), nil
	}
	if !o.Paid() {
		return shopAck("ignored_unpaid", ""), nil
	}
	orderID := o.ID.String()
	currency := strings.ToUpper(strings.TrimSpace(o.Currency))
	a, err := h.purchase(ctx, network, shopOrder{id: orderID, clickID: o.ClickID(), total: o.Total, currency: currency, at: o.At()})
	if err != nil {
		return shopAnswer{}, err
	}
	// Every refund the order has comes with each update of it: the ones
	// already recorded are skipped here rather than logged as duplicates
	// over and over. The answer is the last refund recorded, else the
	// purchase's.
	for _, rf := range o.PaidBack() {
		seen, err := h.shops.RefundRecorded(ctx, network.OrganizationID, network.ID, orderID, rf.ID)
		if err != nil {
			return shopAnswer{}, err
		}
		if seen {
			continue
		}
		ra, err := h.refund(ctx, network, shopRefund{orderID: orderID, refundID: rf.ID, amount: rf.Amount, currency: currency})
		if err != nil {
			return shopAnswer{}, err
		}
		a = ra
	}
	return a, nil
}
