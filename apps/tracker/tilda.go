package main

import (
	"net/http"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/shopconn"
	"github.com/ismagilovnail/flox/apps/internal/tilda"
)

// tilda takes a Tilda site's form submissions (audit UNI-4 part 5): a
// submission with a cart order is a purchase or a repeat purchase, keyed
// by the order id; any other form a lead, keyed by Tilda's tranid. Tilda
// does not sign: the receiver's API key (tilda.KeyName, a POST field or a
// header) is checked instead, and nothing says which site sent it.
func (h *ShopWebhookHandler) tilda(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, shopFormat{
		platform: tilda.Platform,
		verify: func(r *http.Request, body []byte, secret string) bool {
			sub, err := tilda.Parse(r, body)
			return err == nil && tilda.Authorized(r, sub, secret)
		},
		ping: func(r *http.Request, body []byte) bool {
			sub, err := tilda.Parse(r, body)
			return err == nil && tilda.IsTest(sub)
		},
		topics: []string{"form"},
		handle: h.tildaSubmission,
	})
}

func (h *ShopWebhookHandler) tildaSubmission(r *http.Request, network conversion.Network, _ string, body []byte) (shopAnswer, error) {
	ctx := r.Context()
	sub, err := tilda.Parse(r, body)
	if err != nil {
		return shopAck("bad_request", ""), nil
	}
	clickID := sub.ClickID()
	if p := sub.Payment; p != nil {
		return h.purchase(ctx, network, shopOrder{id: p.OrderID, clickID: clickID, total: p.Amount, currency: p.Currency})
	}
	form := map[string][]string{"status": {shopconn.StatusLead}}
	if tran := sub.TranID(); tran != "" {
		form["txn_id"] = []string{tran}
	}
	if clickID != "" {
		form["click_id"] = []string{clickID}
	}
	res, err := h.record(ctx, network, form)
	if err != nil {
		return shopAnswer{}, err
	}
	return shopRecorded(res), nil
}
