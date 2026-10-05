package main

import (
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/classifier"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/landing"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
)

// landClickPath is the tracker route an internal landing's trackUrl
// points at.
const landClickPath = "/lc/"

// landingDecision is what track() does with a routed flow's landing
// stage. The zero value means "no landing stage applies": redirect to the
// destination exactly as before landings were served.
type landingDecision struct {
	externalURL string                    // redirect here instead of the destination
	page        *routingstore.LandingPage // serve this internal landing
	landingID   string
}

// decideLanding resolves the routed flow's landing stage:
//
//   - no enabled stage, or a paused landing → none (a paused landing is
//     skipped, not a dead end — same "inactive stage falls through"
//     stance routing takes on an inactive offer);
//   - external → redirect to its URL;
//   - internal → serve it, if click links can be signed (LANDING_LINK_KEY
//     configured) and its source loads; otherwise none, logged — a broken
//     landing must not take the redirect down with it.
//
// Archived landings are still served: archiving only hides a landing from
// the flow picker, existing flows keep working (the UI's own archive
// confirmation says so).
func (h *Handler) decideLanding(r *http.Request, orgID string, loaded routingstore.CampaignRouting, flowID string) landingDecision {
	if flowID == "" {
		return landingDecision{}
	}
	stage, ok := loaded.FlowLandings[flowID]
	if !ok || stage.Status == string(landing.StatusPaused) {
		return landingDecision{}
	}
	switch landing.Type(stage.Type) {
	case landing.TypeExternal:
		if stage.URL == "" {
			return landingDecision{}
		}
		return landingDecision{externalURL: stage.URL, landingID: stage.LandingID}
	case landing.TypeInternal:
		if h.links == nil {
			return landingDecision{}
		}
		page, err := h.store.LoadLandingPage(r.Context(), orgID, stage.LandingID)
		if err != nil {
			h.logger.Error("loading landing page, redirecting past it", "error", err, "landing_id", stage.LandingID)
			return landingDecision{}
		}
		return landingDecision{page: &page, landingID: stage.LandingID}
	}
	return landingDecision{}
}

// serveLanding writes an internal landing as the response to the
// tracking request itself — no extra redirect hop — rendered by the same
// landing.Render the editor previews with, its trackUrl a signed click
// link back to this tracker. The caller has already enqueued the
// SOURCE_CLICK; LAND_VIEW is recorded here, on the same click_id.
func (h *Handler) serveLanding(w http.ResponseWriter, r *http.Request, d landingDecision, ev event.Event, params params) {
	claims := landinglink.Claims{
		OrganizationID: ev.OrganizationID,
		CampaignID:     ev.CampaignID,
		ClickID:        ev.ClickID,
		StreamSetID:    ev.StreamSetID,
		FlowID:         ev.FlowID,
		LandingID:      d.landingID,
		Destination:    ev.Destination,
		ConfigVersion:  ev.ConfigVersion,
		IssuedAt:       time.Now().Unix(),

		UTMSource:       params.utmSource,
		UTMMedium:       params.utmMedium,
		UTMCampaign:     params.utmCampaign,
		UTMContent:      params.utmContent,
		UTMTerm:         params.utmTerm,
		Subs:            landinglink.TrimSubs(params.subs),
		ExternalClickID: params.externalClickID,
		FBClickID:       params.fbClickID,
		TTClickID:       params.ttClickID,
		Referrer:        params.referrer,
	}
	trackURL := requestOrigin(r) + landClickPath + h.links.Sign(claims)

	assetBase := ""
	if h.assets != nil && d.page.BundlePrefix != "" {
		assetBase = h.assets.PublicURL(d.page.BundlePrefix + "/")
	}
	page := landing.Render(d.page.Content, assetBase, trackURL)

	view := ev
	view.Type = event.LandView
	h.events.Enqueue(view)

	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	// Every response embeds a per-click signed link: never cacheable,
	// by the browser or anything in between.
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

// landClick answers GET /lc/{token} — a visitor clicking through an
// internal landing. Verifies the signed link, records LAND_CLICK on the
// original click_id, and redirects to the destination the click was
// routed to when the landing was served. No database read on this path:
// everything needed is in the (signed) token; geo/device are re-derived
// from this request, the same visitor seconds later.
func (h *Handler) landClick(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	outcome := outcomeNotFound
	defer func() {
		metrics.TrackingLatencySeconds.Observe(time.Since(start).Seconds())
		metrics.TrackingRequestsTotal.WithLabelValues(string(outcome)).Inc()
	}()

	if h.links == nil {
		http.NotFound(w, r)
		return
	}
	claims, err := h.links.Verify(chi.URLParam(r, "token"), time.Now())
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ip := h.clientIP.NetIP(r)
	attrs, err := h.classifier.Classify(r.Context(), classifier.Input{
		IP:             ip,
		UserAgent:      r.UserAgent(),
		AcceptLanguage: r.Header.Get("Accept-Language"),
	})
	if err != nil {
		h.logger.Error("classifying landing click", "error", err)
		attrs = routing.Attributes{}
	}

	h.events.Enqueue(landClickEvent(claims, attrs, ip, r))
	outcome = outcomeLandClick
	http.Redirect(w, r, claims.Destination, http.StatusFound)
}

func landClickEvent(c landinglink.Claims, attrs routing.Attributes, ip net.IP, r *http.Request) event.Event {
	return event.Event{
		Type:           event.LandClick,
		EventAt:        time.Now().UTC(),
		OrganizationID: c.OrganizationID,
		CampaignID:     c.CampaignID,
		ClickID:        c.ClickID,

		StreamSetID:   c.StreamSetID,
		FlowID:        c.FlowID,
		Destination:   c.Destination,
		ConfigVersion: c.ConfigVersion,
		LandingID:     c.LandingID,

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
		Referrer:       c.Referrer,

		UTMSource:   c.UTMSource,
		UTMMedium:   c.UTMMedium,
		UTMCampaign: c.UTMCampaign,
		UTMContent:  c.UTMContent,
		UTMTerm:     c.UTMTerm,
		Subs:        landinglink.ExpandSubs(c.Subs),

		ExternalClickID: c.ExternalClickID,
		FBClickID:       c.FBClickID,
		TTClickID:       c.TTClickID,
	}
}

// requestOrigin is the scheme + host the visitor reached this tracker on
// — trackUrl must be absolute: a served landing carries a <base href>
// pointing at object storage, so a relative "/lc/..." would resolve
// there. X-Forwarded-Proto is honoured without a trusted-proxy check on
// purpose: the only thing a spoofed value changes is the scheme of the
// spoofer's own click link.
func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
