package main

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/classifier"
	"github.com/ismagilovnail/flox/apps/internal/clientip"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/eventbuf"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/macro"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/offercap"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/routingcache"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
	"github.com/ismagilovnail/flox/apps/internal/storage"
)

// Handler owns the §41 critical path:
//
//	HTTP request → parse → classify → route → record async → redirect
//
// Nothing on this path performs an analytics query or waits on event
// persistence (§41, CLAUDE.md non-negotiable #9).
type Handler struct {
	// store is internal/routingcache in production (clicks served from
	// memory, Postgres off the hot path); tests use routingstore directly.
	store      routingcache.Store
	classifier *classifier.Classifier
	engine     routing.Router
	events     *eventbuf.Writer
	logger     *slog.Logger
	clientIP   *clientip.Resolver
	// sticky signs and checks the sf_{campaignId} cookies (sticky.go).
	sticky *stickySigner

	// links signs internal landings' click links; nil (LANDING_LINK_KEY
	// unset/invalid) means internal landing stages are skipped.
	links *landinglink.Signer
	// assets builds a served landing's <base href>; nil means no object
	// storage configured, pages are served without one.
	assets *storage.Client
	// caps is the background snapshot of offers at their daily cap; nil
	// (no ClickHouse) means no offer is capped.
	caps *offercap.Monitor
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/t/{trackingID}", h.track)
	r.Get(landClickPath+"{token}", h.landClick)
	h.registerPwa(r)
}

// trackOutcome labels §53's tracking_requests_total counter — kept to a
// handful of fixed values (low label cardinality is a hard Prometheus
// requirement; anything per-campaign or per-slug here would be a
// cardinality explosion).
type trackOutcome string

const (
	outcomeRedirected trackOutcome = "redirected"
	outcomeLanding    trackOutcome = "landing"    // an internal landing was served
	outcomeLandClick  trackOutcome = "land_click" // a visitor clicked through a served landing
	outcomePwa        trackOutcome = "pwa"        // a PWA install page was served
	outcomeBlocked    trackOutcome = "blocked"
	outcomeNotFound   trackOutcome = "not_found"
	outcomeError      trackOutcome = "error"
)

func (h *Handler) track(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	outcome := outcomeError // default: any early return below that doesn't set this explicitly was a genuine error path
	defer func() {
		metrics.TrackingLatencySeconds.Observe(time.Since(start).Seconds())
		metrics.TrackingRequestsTotal.WithLabelValues(string(outcome)).Inc()
	}()

	ctx := r.Context()
	slug := chi.URLParam(r, "trackingID")
	host := hostWithoutPort(r.Host)

	link, err := h.store.ResolveTrackingLink(ctx, host, slug)
	if err != nil {
		if errors.Is(err, routingstore.ErrNotFound) {
			// An unknown tracking link is not a routing decision — no
			// campaign means no configuration, no click to attribute, and
			// nothing to record. 404 and stop.
			outcome = outcomeNotFound
			http.NotFound(w, r)
			return
		}
		h.logger.Error("resolving tracking link", "error", err, "host", host, "slug", slug)
		http.Error(w, "tracking unavailable", http.StatusBadGateway)
		return
	}

	// A paused/draft/archived campaign never reaches the routing engine —
	// this is the "inactive campaigns" case internal/routing deliberately
	// does not model, handled here where the campaign's status actually
	// lives.
	if link.CampaignStatus != "active" {
		outcome = outcomeNotFound
		http.NotFound(w, r)
		return
	}

	loaded, err := h.store.LoadRoutingConfig(ctx, link.OrganizationID, link.CampaignID)
	if err != nil {
		h.logger.Error("loading routing config", "error", err, "campaign_id", link.CampaignID)
		http.Error(w, "tracking unavailable", http.StatusBadGateway)
		return
	}
	cfg := loaded.Config

	params := parseParams(r)
	ip := h.clientIP.NetIP(r)

	attrs, err := h.classifier.Classify(ctx, classifier.Input{
		IP:             ip,
		UserAgent:      r.UserAgent(),
		AcceptLanguage: r.Header.Get("Accept-Language"),
	})
	if err != nil {
		// Classification failing must not take the redirect down with it:
		// route on whatever we have (empty attributes still match
		// "everything else" style stream sets and campaign fallbacks).
		h.logger.Error("classifying request", "error", err)
		attrs = routing.Attributes{}
	}
	params.applyTo(attrs)

	sticky := h.sticky.parse(r, link.CampaignID)

	result, err := h.engine.Resolve(ctx, routing.RequestContext{
		Attributes:   attrs,
		Config:       cfg,
		Sticky:       sticky.routingState(),
		VisitKey:     visitKey(link.CampaignID, ip, r.UserAgent()),
		CappedOffers: h.caps.Capped(),
	})
	if err != nil {
		h.logger.Error("resolving route", "error", err, "campaign_id", link.CampaignID)
		http.Error(w, "tracking unavailable", http.StatusBadGateway)
		return
	}

	clickID := clickIDFor(sticky, result, loaded.StickyFlowKeepClickID)

	ev := buildEvent(link, result, attrs, params, clickID, ip, r)
	// The destination's macros ({click_id}, {sub1}…) are resolved once, here:
	// the recorded event, and the signed landing/PWA links that carry
	// ev.Destination onward, all hold the URL the visitor actually reaches.
	// Before the 2026-10-06 audit (T7) the offer got a literal "{click_id}"
	// and a network could never send back a postback that matches a click.
	macros := clickMacros(ev)
	ev.Destination = macro.ResolveURL(result.Destination, macros)

	if result.Destination == "" {
		// No flow, no stream-set fallback, no campaign fallback. The click
		// is effectively blocked, so it is recorded as SOURCE_FILTER
		// rather than a SOURCE_CLICK that went nowhere — §43 distinguishes
		// the two precisely so blocked traffic stays visible in analytics.
		ev.Type = event.SourceFilter
		ev.FilterReason = result.Reason
		h.events.Enqueue(ev)
		outcome = outcomeBlocked
		http.NotFound(w, r)
		return
	}

	if cfg.StickyFlow && result.StreamSetID != "" && result.FlowID != "" {
		h.sticky.set(w, link.CampaignID, result, clickID, loaded.StickyFlowKeepClickID)
	}

	// The flow's landing stage, if any, sits between this click and the
	// destination (§25's funnel: Landing -> ... -> Destination).
	lnd := h.decideLanding(r, link.OrganizationID, loaded, result.FlowID)
	// Only a landing actually shown is attributed (the `landing` report
	// dimension): a skipped stage — paused, unsignable — leaves it empty.
	ev.LandingID = lnd.landingID
	// The PWA stage is reachable straight from the click, or after an
	// internal landing (its click link leads there). An external landing
	// is a page we can't link onward from, so the PWA behind it is never
	// reached — and never attributed.
	if lnd.externalURL == "" {
		ev.PwaID = h.decidePwa(loaded, result.FlowID)
	}

	h.events.Enqueue(ev)

	if lnd.page != nil {
		outcome = outcomeLanding
		h.serveLanding(w, r, lnd, ev, params)
		return
	}
	if ev.PwaID != "" {
		outcome = outcomeRedirected
		token := h.links.Sign(clickClaims(ev, params, ""))
		http.Redirect(w, r, pwaURL(requestOrigin(r), token), http.StatusFound)
		return
	}

	target := ev.Destination
	if lnd.externalURL != "" {
		target = macro.ResolveURL(lnd.externalURL, macros)
	}
	outcome = outcomeRedirected

	// 302, not 301: a permanently-cached redirect would bypass the tracker
	// entirely on the next click, losing both the event and any future
	// routing change.
	http.Redirect(w, r, target, http.StatusFound)
}

// clickMacros is what a destination or external landing URL is resolved
// against at click time. {status}/{revenue}/{currency} belong to a
// conversion and resolve empty here; {payout}/{source} are not known to
// the resolver and stay literal (internal/macro).
func clickMacros(ev event.Event) macro.Values {
	return macro.Values{
		ClickID:    ev.ClickID,
		CampaignID: ev.CampaignID,
		OfferID:    ev.OfferID,
		Country:    ev.Country,
		Device:     ev.Device,
		Subs:       ev.Subs,
	}
}

// visitKey builds the value internal/routing hashes to pick a flow (§38).
//
// It cannot be the click_id: that is minted *after* Resolve returns, because
// whether to reuse the sticky one depends on the routing result. So the visit
// is fingerprinted from what is already known at request time. Client IP and
// user agent are the pair that stays constant across a retry, a prefetch or a
// duplicate delivery of the same visit — which is exactly the sameness the
// draw has to reproduce.
//
// The campaign id is in the key so that one visitor is bucketed independently
// per campaign; without it, someone whose hash sits low would land in the
// first arm of every split on the platform.
//
// This is not a fraud control and does not need to be: two genuinely different
// visitors sharing an IP and a user agent land on the same flow, which costs
// nothing but a slightly lumpier split, and the ±2% the fixture allows absorbs
// it at any real traffic volume.
func visitKey(campaignID string, ip net.IP, userAgent string) string {
	return campaignID + "|" + ip.String() + "|" + userAgent
}

// clickIDFor mints a new click_id, or reuses the one carried in the sticky
// cookie when the campaign has stickyFlowKeepClickId enabled and the
// sticky assignment was actually applied — keeping a returning visitor's
// whole journey on one attribution chain (§42, §43).
func clickIDFor(sticky *stickyCookie, result routing.RouteResult, keepClickID bool) string {
	if keepClickID && result.StickyApplied && sticky != nil && sticky.ClickID != "" {
		return sticky.ClickID
	}
	return idgen.New()
}

func buildEvent(
	link routingstore.TrackingLink,
	result routing.RouteResult,
	attrs routing.Attributes,
	params params,
	clickID string,
	ip net.IP,
	r *http.Request,
) event.Event {
	return event.Event{
		Type:            event.SourceClick,
		EventAt:         time.Now().UTC(),
		OrganizationID:  link.OrganizationID,
		CampaignID:      link.CampaignID,
		ClickID:         clickID,
		TrafficSourceID: link.TrafficSourceID,

		StreamSetID:   result.StreamSetID,
		FlowID:        result.FlowID,
		Destination:   result.Destination,
		OfferID:       result.OfferID,
		StickyApplied: result.StickyApplied,
		ConfigVersion: result.ConfigVersion,

		Country:        attrs[routing.FieldCountry],
		Region:         attrs[routing.FieldRegion],
		City:           attrs[routing.FieldCity],
		Device:         attrs[routing.FieldDevice],
		Platform:       attrs[routing.FieldPlatform],
		OS:             attrs[routing.FieldOS],
		OSVersion:      attrs[routing.FieldOSVersion],
		Browser:        attrs[routing.FieldBrowser],
		BrowserVersion: attrs[routing.FieldBrowserVersion],
		Language:       attrs[routing.FieldLanguage],
		IsBot:          attrs[routing.FieldBot] == "1",
		IsProxy:        attrs[routing.FieldProxy] == "1",
		ASN:            attrs[routing.FieldASN],
		ConnectionType: attrs[routing.FieldConnectionType],
		IP:             ip.String(),
		UserAgent:      r.UserAgent(),
		Referrer:       params.referrer,

		UTMSource:   params.utmSource,
		UTMMedium:   params.utmMedium,
		UTMCampaign: params.utmCampaign,
		UTMContent:  params.utmContent,
		UTMTerm:     params.utmTerm,
		Subs:        params.subs,

		ExternalClickID: params.externalClickID,
		FBClickID:       params.fbClickID,
		TTClickID:       params.ttClickID,
	}
}

func hostWithoutPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

// A per-request clientIP helper used to live here, trusting X-Forwarded-
// For's first entry unconditionally — the same client-controlled value
// as apps/internal/ratelimit's own former ClientIP (security audit
// finding #2). Both are replaced by *clientip.Resolver, which only
// honours the header from a configured trusted proxy; see Handler.clientIP.
