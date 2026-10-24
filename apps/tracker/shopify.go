package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/clientip"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/ratelimit"
	"github.com/ismagilovnail/flox/apps/internal/shopify"
)

// maxShopifyBody bounds a webhook body: an order with many line items is
// tens of kilobytes.
const maxShopifyBody = 2 << 20

// ShopifyWebhookHandler takes a Shopify store's order webhooks (audit
// UNI-4 part 3, docs/shop-connections.md):
//
//	POST /shopify/{networkId}
//
// The store signs each with its key (X-Shopify-Hmac-SHA256); a webhook
// that is not signed with the connected store's key, or comes from another
// store, is refused. Shopify waits 5 seconds and retries a non-2xx answer 8
// times over 4 hours, then may drop the subscription: everything the
// engine decided — recorded, a duplicate, refused with its reason in the
// postback log — is answered 200; only a fault on FLOX's side is not.
type ShopifyWebhookHandler struct {
	shops    *shopify.Store
	networks conversion.NetworkLookup
	service  *conversion.Service
	failures *ratelimit.Failures
	clientIP *clientip.Resolver
	logger   *slog.Logger
}

func (h *ShopifyWebhookHandler) Register(r chi.Router) {
	r.Post("/shopify/{networkId}", h.handle)
}

func (h *ShopifyWebhookHandler) handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	topic := r.Header.Get("X-Shopify-Topic")
	result := "error"
	defer func() { metrics.ShopWebhooksTotal.WithLabelValues(shopify.Platform, topicLabel(topic), result).Inc() }()
	answer := func(code int, label, message string) {
		result = label
		res := "ok"
		if code >= 300 {
			res = "error"
		}
		writeJSON(w, code, postbackResponse{Result: res, Message: message})
	}

	ip := h.clientIP.String(r)
	if h.failures != nil {
		if h.failures.Blocked(ctx, ip) {
			answer(http.StatusTooManyRequests, "rate_limited", "too many refused webhooks, try again later")
			return
		}
		defer func() {
			// Unsigned, wrongly signed or for an unknown source.
			if result == "unknown_network" || result == "bad_signature" {
				h.failures.Record(context.WithoutCancel(ctx), ip)
			}
		}()
	}

	networkID := chi.URLParam(r, "networkId")
	if !idgen.IsValid(networkID) {
		answer(http.StatusNotFound, "unknown_network", "unknown source")
		return
	}
	shop, err := h.shops.Connection(ctx, networkID)
	switch {
	case errors.Is(err, shopify.ErrNotConnected):
		answer(http.StatusNotFound, "unknown_network", "unknown source, or no Shopify store connected to it")
		return
	case errors.Is(err, shopify.ErrNoKey):
		h.logger.Error("a Shopify webhook arrived but SHOP_WEBHOOK_ENCRYPTION_KEY is not set", "network_id", networkID)
		answer(http.StatusServiceUnavailable, "internal_error", "Shopify webhooks cannot be checked here")
		return
	case err != nil:
		h.logger.Error("reading a Shopify connection", "error", err, "network_id", networkID)
		answer(http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxShopifyBody+1))
	if err != nil || len(body) > maxShopifyBody {
		answer(http.StatusRequestEntityTooLarge, "bad_request", "body too large or unreadable")
		return
	}
	if !shopify.Verify(body, r.Header.Get("X-Shopify-Hmac-SHA256"), shop.Secret) {
		answer(http.StatusUnauthorized, "bad_signature", "the signature does not match the store's key")
		return
	}
	if d := shopify.NormalizeShopDomain(r.Header.Get("X-Shopify-Shop-Domain")); d != "" && d != shop.ShopDomain {
		answer(http.StatusUnauthorized, "bad_signature", "the webhook is from another store")
		return
	}

	network, err := h.networks.ByID(ctx, networkID)
	if err != nil || network.OrganizationID != shop.OrganizationID {
		h.logger.Error("looking up the network of a Shopify webhook", "error", err, "network_id", networkID)
		answer(http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	var form url.Values
	switch topic {
	case shopify.TopicOrderPaid:
		form, result, err = h.orderForm(ctx, network, body)
	case shopify.TopicRefundCreate:
		form, result, err = h.refundForm(ctx, network, body)
	default:
		answer(http.StatusOK, "ignored", "topic not used: "+topic)
		return
	}
	if err != nil {
		h.logger.Error("reading a Shopify webhook", "error", err, "network_id", networkID, "topic", topic)
		answer(http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if form == nil {
		answer(http.StatusOK, result, result)
		return
	}

	res, err := h.record(ctx, network, form)
	if err != nil {
		h.logger.Error("recording a Shopify webhook", "error", err, "network_id", networkID, "topic", topic)
		answer(http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	// Two orders of one click racing: the second purchase is a repeat one.
	if res.Kind == conversion.ResultDuplicate && form.Get("status") == shopify.StatusPurchase && form.Get("click_id") != "" {
		form.Set("status", shopify.StatusRepeatPurchase)
		if res, err = h.record(ctx, network, form); err != nil {
			h.logger.Error("recording a Shopify webhook", "error", err, "network_id", networkID, "topic", topic)
			answer(http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
	}
	result = string(res.Kind)
	writeJSON(w, http.StatusOK, postbackResponse{Result: string(res.Kind), Status: string(res.Status), Message: res.Message})
}

// topicLabel bounds the metric's topic label: the header is the sender's.
func topicLabel(topic string) string {
	switch topic {
	case shopify.TopicOrderPaid, shopify.TopicRefundCreate:
		return topic
	}
	return "other"
}

func (h *ShopifyWebhookHandler) record(ctx context.Context, network conversion.Network, form url.Values) (conversion.Result, error) {
	p, err := conversion.ParseParams(form, network, time.Now())
	if err != nil {
		// What Shopify sent cannot be read (an amount, a time): sending it
		// again will not help, so it is answered 200 and logged here — never
		// recorded without the part that could not be read.
		h.logger.Warn("a Shopify webhook FLOX cannot read", "error", err, "network_id", network.ID, "order", form.Get("txn_id"))
		return conversion.Result{Kind: conversion.ResultError, Message: strings.TrimPrefix(err.Error(), "conversion: ")}, nil
	}
	p.ExternalClickID = ""
	return h.service.Record(ctx, p)
}

// orderForm is a paid order as the own-site statuses: a purchase, or a
// repeat purchase when the click already bought; nil (with the reason)
// for a test order or one already recorded.
func (h *ShopifyWebhookHandler) orderForm(ctx context.Context, network conversion.Network, body []byte) (url.Values, string, error) {
	o, err := shopify.ParseOrder(body)
	if err != nil {
		return nil, "bad_request", nil
	}
	if o.Test {
		return nil, "ignored_test_order", nil
	}
	orderID := o.ID.String()
	seen, err := h.shops.OrderRecorded(ctx, network.OrganizationID, network.ID, orderID)
	if err != nil {
		return nil, "", err
	}
	if seen {
		return nil, string(conversion.ResultDuplicate), nil
	}
	clickID := o.ClickID()
	status := shopify.StatusPurchase
	if clickID != "" {
		bought, err := h.shops.ClickBought(ctx, network.OrganizationID, clickID)
		if err != nil {
			return nil, "", err
		}
		if bought {
			status = shopify.StatusRepeatPurchase
		}
	}
	form := url.Values{"status": {status}, "txn_id": {orderID}}
	if clickID != "" {
		form.Set("click_id", clickID)
	}
	if v := strings.TrimSpace(o.TotalPrice); v != "" {
		form.Set("revenue", v)
		if o.Currency != "" {
			form.Set("currency", o.Currency)
		}
	}
	if at := o.At(); at != "" {
		form.Set("event_time", at)
	}
	return form, "", nil
}

// refundForm is money paid back on an order FLOX recorded: a refund of
// that amount, keyed by the refund's id; nil for a refund that paid
// nothing back or of an order no FLOX click made.
func (h *ShopifyWebhookHandler) refundForm(ctx context.Context, network conversion.Network, body []byte) (url.Values, string, error) {
	rf, err := shopify.ParseRefund(body)
	if err != nil {
		return nil, "bad_request", nil
	}
	amount, currency, ok := rf.Amount()
	if !ok {
		return nil, "ignored_no_money", nil
	}
	orderID := rf.OrderID.String()
	clickID, found, err := h.shops.OrderClick(ctx, network.OrganizationID, network.ID, orderID)
	if err != nil {
		return nil, "", err
	}
	if !found {
		return nil, "ignored_unknown_order", nil
	}
	form := url.Values{
		"status": {shopify.StatusRefund}, "click_id": {clickID}, "txn_id": {orderID},
		"refund_amount": {strconv.FormatFloat(amount, 'f', -1, 64)}, "refund_id": {rf.ID.String()},
	}
	if currency != "" {
		form.Set("currency", currency)
	}
	if at := rf.At(); at != "" {
		form.Set("event_time", at)
	}
	return form, "", nil
}
