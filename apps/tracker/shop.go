package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/clientip"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/ratelimit"
	"github.com/ismagilovnail/flox/apps/internal/shopconn"
)

// maxShopBody bounds a webhook body: an order with many line items is
// tens of kilobytes.
const maxShopBody = 2 << 20

// ShopWebhookHandler takes online stores' order webhooks to own-site
// sources (audit UNI-4, docs/shop-connections.md):
//
//	POST /shopify/{networkId}       shopify.go
//	POST /woocommerce/{networkId}   woocommerce.go
//
// The store signs each with its key; a webhook that is not signed with the
// connected store's key, or says it is from another store, is refused.
// Stores give up on an address that keeps failing (Shopify retries 8 times
// over 4 hours and may drop the subscription; WooCommerce does not retry
// and disables the webhook after 5 failures in a row): everything the
// engine decided — recorded, a duplicate, refused with its reason in the
// postback log — is answered 200; only a fault on FLOX's side is not.
type ShopWebhookHandler struct {
	shops    *shopconn.Store
	networks conversion.NetworkLookup
	service  *conversion.Service
	failures *ratelimit.Failures
	clientIP *clientip.Resolver
	logger   *slog.Logger
}

func (h *ShopWebhookHandler) Register(r chi.Router) {
	r.Post("/shopify/{networkId}", h.shopify)
	r.Post("/woocommerce/{networkId}", h.woocommerce)
}

// shopFormat is what differs between the platforms' webhooks.
type shopFormat struct {
	platform shopconn.Platform
	// Request headers: the topic, the signature, the store it is from.
	topicHeader, signatureHeader, sourceHeader string
	verify                                     func(body []byte, signature, secret string) bool
	// ping, when set: a delivery check answered 200 before the signature.
	ping func(body []byte, signature string) bool
	// topics are the ones acted on, the metric's topic labels.
	topics []string
	// handle reads a verified webhook and records what it says.
	handle func(ctx context.Context, network conversion.Network, topic string, body []byte) (shopAnswer, error)
}

// shopAnswer is what a webhook came to: the response and the metric's
// result label.
type shopAnswer struct {
	res   postbackResponse
	label string
}

// shopAck answers a webhook the engine was not asked about.
func shopAck(label, message string) shopAnswer {
	if message == "" {
		message = label
	}
	return shopAnswer{postbackResponse{Result: "ok", Message: message}, label}
}

func shopRecorded(res conversion.Result) shopAnswer {
	return shopAnswer{postbackResponse{Result: string(res.Kind), Status: string(res.Status), Message: res.Message}, string(res.Kind)}
}

func (h *ShopWebhookHandler) serve(w http.ResponseWriter, r *http.Request, f shopFormat) {
	ctx := r.Context()
	topic := r.Header.Get(f.topicHeader)
	result := "error"
	defer func() {
		label := "other"
		if slices.Contains(f.topics, topic) {
			label = topic
		}
		metrics.ShopWebhooksTotal.WithLabelValues(f.platform.Name, label, result).Inc()
	}()
	answer := func(code int, label, message string) {
		result = label
		writeJSON(w, code, postbackResponse{Result: "error", Message: message})
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
	shop, err := h.shops.Connection(ctx, networkID, f.platform.Name)
	switch {
	case errors.Is(err, shopconn.ErrNotConnected):
		answer(http.StatusNotFound, "unknown_network", "unknown source, or no "+f.platform.Title+" store connected to it")
		return
	case errors.Is(err, shopconn.ErrNoKey):
		h.logger.Error("a store webhook arrived but SHOP_WEBHOOK_ENCRYPTION_KEY is not set", "platform", f.platform.Name, "network_id", networkID)
		answer(http.StatusServiceUnavailable, "internal_error", f.platform.Title+" webhooks cannot be checked here")
		return
	case err != nil:
		h.logger.Error("reading a store connection", "error", err, "platform", f.platform.Name, "network_id", networkID)
		answer(http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxShopBody+1))
	if err != nil || len(body) > maxShopBody {
		answer(http.StatusRequestEntityTooLarge, "bad_request", "body too large or unreadable")
		return
	}
	signature := r.Header.Get(f.signatureHeader)
	if f.ping != nil && f.ping(body, signature) {
		result = "ping"
		writeJSON(w, http.StatusOK, postbackResponse{Result: "ok", Message: "ping"})
		return
	}
	if !f.verify(body, signature, shop.Secret) {
		answer(http.StatusUnauthorized, "bad_signature", "the signature does not match the store's key")
		return
	}
	if d := f.platform.NormalizeDomain(r.Header.Get(f.sourceHeader)); d != "" && d != shop.ShopDomain {
		answer(http.StatusUnauthorized, "bad_signature", "the webhook is from another store")
		return
	}

	network, err := h.networks.ByID(ctx, networkID)
	if err != nil || network.OrganizationID != shop.OrganizationID {
		h.logger.Error("looking up the network of a store webhook", "error", err, "platform", f.platform.Name, "network_id", networkID)
		answer(http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	if !slices.Contains(f.topics, topic) {
		result = "ignored"
		writeJSON(w, http.StatusOK, postbackResponse{Result: "ok", Message: "topic not used: " + topic})
		return
	}
	a, err := f.handle(ctx, network, topic, body)
	if err != nil {
		h.logger.Error("recording a store webhook", "error", err, "platform", f.platform.Name, "network_id", networkID, "topic", topic)
		answer(http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	result = a.label
	writeJSON(w, http.StatusOK, a.res)
}

func (h *ShopWebhookHandler) record(ctx context.Context, network conversion.Network, form url.Values) (conversion.Result, error) {
	p, err := conversion.ParseParams(form, network, time.Now())
	if err != nil {
		// What the store sent cannot be read (an amount, a time): sending it
		// again will not help, so it is answered 200 and logged here — never
		// recorded without the part that could not be read.
		h.logger.Warn("a store webhook FLOX cannot read", "error", err, "network_id", network.ID, "order", form.Get("txn_id"))
		return conversion.Result{Kind: conversion.ResultError, Message: strings.TrimPrefix(err.Error(), "conversion: ")}, nil
	}
	p.ExternalClickID = ""
	return h.service.Record(ctx, p)
}

// shopOrder is a paid order as the store gives it.
type shopOrder struct {
	id, clickID         string
	total, currency, at string
}

// purchase records a paid order: a purchase, or a repeat purchase when
// the click already bought. An order already recorded (a replayed
// webhook; WooCommerce sends the order with every update of it) is a
// duplicate without calling the engine — otherwise the replay of a first
// order would become a repeat purchase.
//
// The click is looked up before the order, so a delivery racing another
// of the same order cannot see the other's purchase as the click's and
// miss it as the order's: either it sees the order, or the click had not
// bought and its purchase comes back a duplicate.
func (h *ShopWebhookHandler) purchase(ctx context.Context, network conversion.Network, o shopOrder) (shopAnswer, error) {
	status := shopconn.StatusPurchase
	if o.clickID != "" {
		bought, err := h.shops.ClickBought(ctx, network.OrganizationID, o.clickID)
		if err != nil {
			return shopAnswer{}, err
		}
		if bought {
			status = shopconn.StatusRepeatPurchase
		}
	}
	seen, err := h.shops.OrderRecorded(ctx, network.OrganizationID, network.ID, o.id)
	if err != nil || seen {
		return shopAck(string(conversion.ResultDuplicate), ""), err
	}
	form := url.Values{"status": {status}, "txn_id": {o.id}}
	if o.clickID != "" {
		form.Set("click_id", o.clickID)
	}
	if v := strings.TrimSpace(o.total); v != "" {
		form.Set("revenue", v)
		if o.currency != "" {
			form.Set("currency", o.currency)
		}
	}
	if o.at != "" {
		form.Set("event_time", o.at)
	}
	res, err := h.record(ctx, network, form)
	if err != nil {
		return shopAnswer{}, err
	}
	if res.Kind == conversion.ResultDuplicate && status == shopconn.StatusPurchase && o.clickID != "" {
		// The click's purchase landed between the lookups and the record:
		// the same order racing itself (two deliveries at once) is a
		// duplicate; another order of the click is a repeat purchase.
		if seen, err = h.shops.OrderRecorded(ctx, network.OrganizationID, network.ID, o.id); err != nil || seen {
			return shopRecorded(res), err
		}
		form.Set("status", shopconn.StatusRepeatPurchase)
		if res, err = h.record(ctx, network, form); err != nil {
			return shopAnswer{}, err
		}
	}
	return shopRecorded(res), nil
}

// shopRefund is money paid back on an order.
type shopRefund struct {
	orderID, refundID string
	amount            float64
	currency, at      string
}

// refund records money paid back on an order FLOX recorded: a refund of
// that amount, keyed by the refund's id (two partial refunds are two, a
// replay is a duplicate); an order no FLOX click made is left.
func (h *ShopWebhookHandler) refund(ctx context.Context, network conversion.Network, rf shopRefund) (shopAnswer, error) {
	clickID, found, err := h.shops.OrderClick(ctx, network.OrganizationID, network.ID, rf.orderID)
	if err != nil {
		return shopAnswer{}, err
	}
	if !found {
		return shopAck("ignored_unknown_order", ""), nil
	}
	form := url.Values{
		"status": {shopconn.StatusRefund}, "click_id": {clickID}, "txn_id": {rf.orderID},
		"refund_amount": {strconv.FormatFloat(rf.amount, 'f', -1, 64)}, "refund_id": {rf.refundID},
	}
	if rf.currency != "" {
		form.Set("currency", rf.currency)
	}
	if rf.at != "" {
		form.Set("event_time", rf.at)
	}
	res, err := h.record(ctx, network, form)
	if err != nil {
		return shopAnswer{}, err
	}
	return shopRecorded(res), nil
}
