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
// destination.
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
func (h *Handler) serveLanding(w http.ResponseWriter, r *http.Request, d landingDecision, ev event.Event, params params, pwa pwaStep) {
	trackURL := requestOrigin(r) + landClickPath + h.links.Sign(clickClaims(ev, params, d.landingID, r.Host, pwa))

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

// clickClaims is everything a signed onward link (landing click-through,
// PWA page) carries from the original click: routing + attribution
// context, and which funnel stages (PWA) still lie ahead.
// clickClaims are the claims of a link issued on host (the request's
// Host): it is honoured only there (Claims.Host). pwa is the PWA stage
// still ahead of the visitor.
func clickClaims(ev event.Event, params params, landingID, host string, pwa pwaStep) landinglink.Claims {
	c := landinglink.Claims{
		Host:            landinglink.NormalizeHost(host),
		OrganizationID:  ev.OrganizationID,
		CampaignID:      ev.CampaignID,
		ClickID:         ev.ClickID,
		StreamSetID:     ev.StreamSetID,
		FlowID:          ev.FlowID,
		LandingID:       landingID,
		PwaID:           ev.PwaID,
		PostlandingID:   ev.PostlandingID,
		Destination:     ev.Destination,
		OfferID:         ev.OfferID,
		NetworkID:       ev.NetworkID,
		TrafficSourceID: ev.TrafficSourceID,
		ConfigVersion:   ev.ConfigVersion,
		IssuedAt:        time.Now().Unix(),

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
	pwa.applyTo(&c)
	return c
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
	if err != nil || !claims.OnHost(r.Host) {
		// Another domain's link — even another organization's — is not
		// honoured here (tech audit M4).
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

	click := landClickEvent(claims, attrs, ip, r)
	h.events.Enqueue(click)
	outcome = outcomeLandClick
	if claims.PwaID != "" {
		// Next funnel stage: the PWA install page, on the same token.
		http.Redirect(w, r, pwaURL(requestOrigin(r), chi.URLParam(r, "token")), http.StatusFound)
		return
	}
	if claims.PwaURL != "" {
		// An external PWA or an app link (pwa_external.go).
		h.sendToPwaRedirect(w, r, claims, click)
		return
	}
	h.continueOnward(w, r, claims, func() event.Event { return click })
}

func landClickEvent(c landinglink.Claims, attrs routing.Attributes, ip net.IP, r *http.Request) event.Event {
	ev := c.Event(event.LandClick, time.Now().UTC())
	ev.Country = attrs[routing.FieldCountry]
	ev.Region = attrs[routing.FieldRegion]
	ev.City = attrs[routing.FieldCity]
	ev.Device = attrs[routing.FieldDevice]
	ev.Platform = attrs[routing.FieldPlatform]
	ev.OS = attrs[routing.FieldOS]
	ev.OSVersion = attrs[routing.FieldOSVersion]
	ev.Browser = attrs[routing.FieldBrowser]
	ev.BrowserVersion = attrs[routing.FieldBrowserVersion]
	ev.Language = attrs[routing.FieldLanguage]
	ev.IsBot = attrs[routing.FieldBot] == "1"
	ev.IsProxy = attrs[routing.FieldProxy] == "1"
	ev.ASN = attrs[routing.FieldASN]
	ev.ConnectionType = attrs[routing.FieldConnectionType]
	ev.IP = ip.String()
	ev.UserAgent = r.UserAgent()
	return ev
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
