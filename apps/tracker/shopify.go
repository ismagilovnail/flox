package main

import (
	"context"
	"net/http"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/shopify"
)

// shopify takes a Shopify store's webhooks (audit UNI-4 part 3):
// orders/paid is a purchase or a repeat purchase, refunds/create a refund
// of what was paid back. Signed in X-Shopify-Hmac-SHA256; the store is
// X-Shopify-Shop-Domain.
func (h *ShopWebhookHandler) shopify(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, shopFormat{
		platform:        shopify.Platform,
		topicHeader:     "X-Shopify-Topic",
		signatureHeader: "X-Shopify-Hmac-SHA256",
		sourceHeader:    "X-Shopify-Shop-Domain",
		verify:          shopify.Verify,
		topics:          []string{shopify.TopicOrderPaid, shopify.TopicRefundCreate},
		handle:          h.shopifyWebhook,
	})
}

func (h *ShopWebhookHandler) shopifyWebhook(ctx context.Context, network conversion.Network, topic string, body []byte) (shopAnswer, error) {
	if topic == shopify.TopicRefundCreate {
		rf, err := shopify.ParseRefund(body)
		if err != nil {
			return shopAck("bad_request", ""), nil
		}
		amount, currency, ok := rf.Amount()
		if !ok {
			return shopAck("ignored_no_money", ""), nil
		}
		return h.refund(ctx, network, shopRefund{orderID: rf.OrderID.String(), refundID: rf.ID.String(),
			amount: amount, currency: currency, at: rf.At()})
	}
	o, err := shopify.ParseOrder(body)
	if err != nil {
		return shopAck("bad_request", ""), nil
	}
	if o.Test {
		return shopAck("ignored_test_order", ""), nil
	}
	return h.purchase(ctx, network, shopOrder{id: o.ID.String(), clickID: o.ClickID(),
		total: o.TotalPrice, currency: o.Currency, at: o.At()})
}
