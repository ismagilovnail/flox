package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"

	"github.com/ismagilovnail/flox/apps/internal/clientip"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/ratelimit"
)

// networksByID is a NetworkLookup over a few networks.
type networksByID map[string]conversion.Network

func (n networksByID) ByID(_ context.Context, id string) (conversion.Network, error) {
	if net, ok := n[id]; ok {
		return net, nil
	}
	return conversion.Network{}, conversion.ErrNetworkNotFound
}

// Audit UNI-1 part 2: a page's script reports a conversion at /c/ with no
// secret — only for a source that turned it on, only from its domains,
// never as a duplicate, and refusals count toward a per-IP limit.
func TestBrowserConversions(t *testing.T) {
	on, off := idgen.New(), idgen.New()
	org := idgen.New()
	store := &captureStore{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver, err := clientip.New(nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	h := &BrowserConversionHandler{
		networks: networksByID{
			on: {ID: on, OrganizationID: org, AcceptDuplicates: true, PostbackSecretHash: hashFor("s3cret"),
				BrowserConversions: true, BrowserDomains: []string{"shop.example"}},
			off: {ID: off, OrganizationID: org, PostbackSecretHash: hashFor("s3cret"), BrowserDomains: []string{"shop.example"}},
		},
		service:  conversion.NewService(allSales{}, store, usdOnly{}, noClicks{}, noAttempts{}),
		failures: ratelimit.New(nil, logger).Failures("browser-test", 3, time.Minute),
		clientIP: resolver,
		logger:   logger,
	}
	r := chi.NewRouter()
	h.Register(r)
	send := func(ip, network, query string, header map[string]string) (*httptest.ResponseRecorder, postbackResponse) {
		req := httptest.NewRequest(http.MethodPost, "/c/"+network+"?"+query, nil)
		req.RemoteAddr = ip + ":1234"
		for k, v := range header {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		var body postbackResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec, body
	}
	fromShop := map[string]string{"Origin": "https://www.shop.example"}

	// A fresh click from the source's own site (a subdomain), by Origin
	// and by Referer; the secret and an external click id are not taken.
	rec, body := send("10.0.0.1", on, "click_id="+idgen.New()+"&status=purchase&revenue=1990&currency=RUB&txn_id=A-1&secret=s3cret&external_click_id=fb.1", fromShop)
	if rec.Code != http.StatusOK || body.Result != "success" || rec.Header().Get("Access-Control-Allow-Origin") != "https://www.shop.example" {
		t.Fatalf("from the shop: %d %+v, ACAO %q", rec.Code, body, rec.Header().Get("Access-Control-Allow-Origin"))
	}
	got := store.entries[len(store.entries)-1]
	if got.AcceptDuplicates || got.ExternalClickID != "" || got.NetworkTxnID != "A-1" {
		t.Fatalf("recorded %+v, want no duplicates, no external click id, the order number", got)
	}
	// The note goes where the postback log reads it (the attempt log) and
	// into the answer.
	if !strings.Contains(body.Message, "reported by the page's script") {
		t.Fatalf("message %q, want it marked as the page's", body.Message)
	}
	if rec, body := send("10.0.0.1", on, "click_id="+idgen.New()+"&status=lead", map[string]string{"Referer": "https://shop.example/thanks?x=1"}); rec.Code != http.StatusOK {
		t.Fatalf("by Referer: %d %+v", rec.Code, body)
	}

	// Refused: another site, no site at all, a source that is off, an old
	// made-up click.
	recorded := len(store.entries)
	ip := 0
	old := ulid.MustNew(ulid.Timestamp(time.Now().Add(-time.Hour)), ulid.DefaultEntropy()).String()
	for name, tc := range map[string]struct {
		network, query string
		header         map[string]string
		code           int
	}{
		"another site": {on, "click_id=" + idgen.New() + "&status=purchase", map[string]string{"Origin": "https://shop.example.evil.test"}, http.StatusForbidden},
		"no site":      {on, "click_id=" + idgen.New() + "&status=purchase", nil, http.StatusForbidden},
		"source off":   {off, "click_id=" + idgen.New() + "&status=purchase", fromShop, http.StatusNotFound},
		"old click":    {on, "click_id=" + old + "&status=purchase", fromShop, http.StatusOK},
	} {
		// Each from its own IP: the limit is tested below.
		ip++
		rec, body := send(fmt.Sprintf("10.0.1.%d", ip), tc.network, tc.query, tc.header)
		if rec.Code != tc.code || body.Result != "error" {
			t.Errorf("%s: %d %+v, want %d and an error", name, rec.Code, body, tc.code)
		}
	}
	if n := len(store.entries) - recorded; n != 1 {
		t.Errorf("refused requests recorded %d entries, want only the old click's refusal in the log", n)
	}

	// Three refusals from one IP, then 429 before any lookup.
	for i := 0; i < 3; i++ {
		send("10.0.0.9", on, "click_id="+idgen.New()+"&status=purchase", map[string]string{"Origin": "https://evil.test"})
	}
	if rec, _ := send("10.0.0.9", on, "click_id="+idgen.New()+"&status=purchase", fromShop); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("after 3 refusals = %d, want 429", rec.Code)
	}
}

func TestFloxJSIsServed(t *testing.T) {
	r := chi.NewRouter()
	(&BrowserConversionHandler{}).Register(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/flox.js", nil))
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/javascript") ||
		!strings.Contains(rec.Body.String(), "flox_click_id") || rec.Header().Get("Cache-Control") == "" {
		t.Fatalf("/flox.js: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}
