package main

import (
	"context"
	_ "embed"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/clientip"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/ratelimit"
)

// floxJS is the script a site without a server puts on its pages
// (static/flox.js): it keeps the click id and reports a lead or a purchase.
//
//go:embed static/flox.js
var floxJS []byte

// BrowserConversionHandler takes conversions from a page's script (audit
// UNI-1 part 2, docs/networks-offers.md "Own site without a server"):
//
//	GET  /flox.js
//	GET/POST /c/{networkId}
//
// A page cannot hold the postback secret, so /c/ has none. What stands in
// for it: the network must have turned it on (browser_conversions); the
// page must be on one of its browser_domains (Origin, else Referer); the
// engine takes only statuses that map to CPA_HOLD or CPA_ACCEPT, each once
// per click (duplicates are never accepted here), and only a click it knows
// or one fresh enough to be on its way (conversion.BrowserFreshClick). A
// refused request counts toward the same per-IP limit as refused
// postbacks.
type BrowserConversionHandler struct {
	networks conversion.NetworkLookup
	service  *conversion.Service
	failures *ratelimit.Failures
	clientIP *clientip.Resolver
	logger   *slog.Logger
}

func (h *BrowserConversionHandler) Register(r chi.Router) {
	r.Get("/flox.js", serveFloxJS)
	r.Get("/c/{networkId}", h.handle)
	r.Post("/c/{networkId}", h.handle)
}

func serveFloxJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(floxJS)
}

// pageHost is the host of the page that made the request: Origin (sent by
// sendBeacon and fetch), else Referer (an image request). "" when neither
// names one.
func pageHost(r *http.Request) string {
	for _, h := range []string{r.Header.Get("Origin"), r.Header.Get("Referer")} {
		if h == "" || h == "null" {
			continue
		}
		u, err := url.Parse(h)
		if err == nil && u.Hostname() != "" {
			return strings.ToLower(u.Hostname())
		}
	}
	return ""
}

// domainAllowed: host is one of domains or a subdomain of one.
func domainAllowed(host string, domains []string) bool {
	for _, d := range domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func (h *BrowserConversionHandler) handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	result := "internal_error"
	defer func() { metrics.BrowserConversionsTotal.WithLabelValues(result).Inc() }()
	refuse := func(code int, label, message string) {
		result = label
		writeJSON(w, code, postbackResponse{Result: "error", Message: message})
	}

	ip := h.clientIP.String(r)
	if h.failures != nil {
		if h.failures.Blocked(ctx, ip) {
			refuse(http.StatusTooManyRequests, "rate_limited", "too many refused conversions, try again later")
			return
		}
		defer func() {
			// Everything but a recorded, repeated or ignored conversion: a
			// wrong site, a source that is off, a made-up click.
			if result != "success" && result != "duplicate" && result != "ignored" && result != "internal_error" {
				h.failures.Record(context.WithoutCancel(ctx), ip)
			}
		}()
	}

	networkID := chi.URLParam(r, "networkId")
	if !idgen.IsValid(networkID) {
		refuse(http.StatusNotFound, "unknown_network", "unknown source")
		return
	}
	network, err := h.networks.ByID(ctx, networkID)
	if errors.Is(err, conversion.ErrNetworkNotFound) || (err == nil && !network.BrowserConversions) {
		refuse(http.StatusNotFound, "unknown_network", "unknown source, or conversions from pages are off for it")
		return
	}
	if err != nil {
		h.logger.Error("looking up network", "error", err, "network_id", networkID)
		refuse(http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	host := pageHost(r)
	if host == "" || !domainAllowed(host, network.BrowserDomains) {
		refuse(http.StatusForbidden, "forbidden_origin", "this site is not among the source's domains")
		return
	}
	// The page may read the answer (its console, fetch): only its own site.
	if origin := r.Header.Get("Origin"); origin != "" && origin != "null" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
	}

	if err := r.ParseForm(); err != nil {
		refuse(http.StatusBadRequest, "bad_request", "could not parse request")
		return
	}
	form := url.Values{}
	for _, k := range []string{"click_id", "status", "revenue", "currency", "txn_id"} {
		if v := r.Form.Get(k); v != "" {
			form.Set(k, v)
		}
	}
	p, err := conversion.ParseParams(form, network, time.Now())
	if err != nil {
		refuse(http.StatusBadRequest, "bad_request", strings.TrimPrefix(err.Error(), "conversion: "))
		return
	}
	p.Browser = true
	p.AcceptDuplicates = false
	p.ExternalClickID = ""

	res, err := h.service.Record(ctx, p)
	if err != nil {
		h.logger.Error("recording a page's conversion", "error", err, "network_id", networkID)
		refuse(http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	result = string(res.Kind)
	if res.Attribution.Outcome == attribution.OutcomeUnknownClick {
		// Fresh enough to be retried, but not yet known: counted like a
		// refusal, so made-up click ids cannot be sent without limit.
		result = "unknown_click"
	}
	writeJSON(w, http.StatusOK, postbackResponse{Result: string(res.Kind), Status: string(res.Status), Message: res.Message})
}
