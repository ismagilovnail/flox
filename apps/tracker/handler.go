package main

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/classifier"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/eventbuf"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
)

// Handler owns the §41 critical path:
//
//	HTTP request → parse → classify → route → record async → redirect
//
// Nothing on this path performs an analytics query or waits on event
// persistence (§41, CLAUDE.md non-negotiable #9).
type Handler struct {
	store      *routingstore.Store
	classifier *classifier.Classifier
	engine     routing.Router
	events     *eventbuf.Writer
	logger     *slog.Logger
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/t/{trackingID}", h.track)
}

func (h *Handler) track(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "trackingID")
	host := hostWithoutPort(r.Host)

	link, err := h.store.ResolveTrackingLink(ctx, host, slug)
	if err != nil {
		if errors.Is(err, routingstore.ErrNotFound) {
			// An unknown tracking link is not a routing decision — no
			// campaign means no configuration, no click to attribute, and
			// nothing to record. 404 and stop.
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

	attrs, err := h.classifier.Classify(ctx, classifier.Input{
		IP:             clientIP(r),
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

	sticky := parseStickyCookie(r, link.CampaignID)

	result, err := h.engine.Resolve(ctx, routing.RequestContext{
		Attributes: attrs,
		Config:     cfg,
		Sticky:     sticky.routingState(),
	})
	if err != nil {
		h.logger.Error("resolving route", "error", err, "campaign_id", link.CampaignID)
		http.Error(w, "tracking unavailable", http.StatusBadGateway)
		return
	}

	clickID := clickIDFor(sticky, result, loaded.StickyFlowKeepClickID)

	ev := buildEvent(link, result, attrs, params, clickID, r)

	if result.Destination == "" {
		// No flow, no stream-set fallback, no campaign fallback. The click
		// is effectively blocked, so it is recorded as SOURCE_FILTER
		// rather than a SOURCE_CLICK that went nowhere — §43 distinguishes
		// the two precisely so blocked traffic stays visible in analytics.
		ev.Type = event.SourceFilter
		ev.FilterReason = result.Reason
		h.events.Enqueue(ev)
		http.NotFound(w, r)
		return
	}

	if cfg.StickyFlow && result.StreamSetID != "" && result.FlowID != "" {
		setStickyCookie(w, link.CampaignID, result, clickID, loaded.StickyFlowKeepClickID)
	}

	h.events.Enqueue(ev)

	// 302, not 301: a permanently-cached redirect would bypass the tracker
	// entirely on the next click, losing both the event and any future
	// routing change.
	http.Redirect(w, r, result.Destination, http.StatusFound)
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
	r *http.Request,
) event.Event {
	return event.Event{
		Type:           event.SourceClick,
		EventAt:        time.Now().UTC(),
		OrganizationID: link.OrganizationID,
		CampaignID:     link.CampaignID,
		ClickID:        clickID,

		StreamSetID:   result.StreamSetID,
		FlowID:        result.FlowID,
		Destination:   result.Destination,
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
		IP:             clientIP(r).String(),
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

// clientIP prefers X-Forwarded-For's first entry, since the tracker runs
// behind a load balancer/CDN in every real deployment.
func clientIP(r *http.Request) net.IP {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
		if ip := net.ParseIP(first); ip != nil {
			return ip
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip
		}
	}
	return net.ParseIP(r.RemoteAddr)
}
