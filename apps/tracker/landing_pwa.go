package main

import (
	"encoding/json"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/landing"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
)

// A landing shown as an app (flows.landing_as_pwa, docs/pwa-serving.md
// "Landing as PWA"): the internal landing is served from /lpwa/<token>/
// — its own path, so the page is inside the scope of its web app manifest
// and service worker — with the manifest named and pictured after the
// landing (title, logo). Installing it records PWA_INSTALL; the installed
// app's start_url records PWA_OPEN (and IOS_INSTALL on an iPhone's first
// launch) and continues to the funnel's next step after the landing. Its
// PWA_* events carry the landing and no PWA id (they are the landing's
// installs, not the next stage's PWA's); LAND_VIEW is attributed like any
// landing's, the click's PWA stage included. In-app WebViews are bounced
// to the external browser, as for a PWA page (§28/§73).
const landingPwaPrefix = "/lpwa/"

func (h *Handler) registerLandingPwa(r chi.Router) {
	r.Get(landingPwaPrefix+"{token}", h.landingPwaRedirectSlash)
	r.Get(landingPwaPrefix+"{token}/", h.landingPwaPage)
	r.Get(landingPwaPrefix+"{token}/manifest.webmanifest", h.landingPwaManifest)
	r.Get(landingPwaPrefix+"{token}/sw.js", h.pwaServiceWorker)
	r.Post(landingPwaPrefix+"{token}/installed", h.landingPwaInstalled)
	r.Get(landingPwaPrefix+"{token}/open", h.landingPwaOpen)
}

func (h *Handler) landingPwaRedirectSlash(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, landingPwaPrefix+chi.URLParam(r, "token")+"/", http.StatusMovedPermanently)
}

// landingPwaClaims verifies the token: a landing's, on this host. anyAge
// for the routes an installed app keeps calling (manifest, start_url).
func (h *Handler) landingPwaClaims(r *http.Request, anyAge bool) (landinglink.Claims, bool) {
	if h.links == nil {
		return landinglink.Claims{}, false
	}
	verify := h.links.Verify
	if anyAge {
		verify = h.links.VerifyAnyAge
	}
	c, err := verify(chi.URLParam(r, "token"), time.Now())
	if err != nil || c.LandingID == "" || !c.OnHost(r.Host) {
		return landinglink.Claims{}, false
	}
	return c, true
}

// landingPwaBase is the app's absolute base URL: the landing carries a
// <base href> to object storage, so nothing it links may be relative.
func landingPwaBase(r *http.Request) string {
	return requestOrigin(r) + landingPwaPrefix + chi.URLParam(r, "token") + "/"
}

// landingEvent is a PWA_* event t of the landing app on the click, with
// this request's geo/device: the landing's id, not the PWA stage's.
func (h *Handler) landingEvent(r *http.Request, t event.Type, c landinglink.Claims) event.Event {
	ev := h.claimsEvent(r, c)
	ev.Type, ev.PwaID = t, ""
	return ev
}

func (h *Handler) landingPwaPage(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	outcome := outcomeNotFound
	defer func() {
		metrics.TrackingLatencySeconds.Observe(time.Since(start).Seconds())
		metrics.TrackingRequestsTotal.WithLabelValues(string(outcome)).Inc()
	}()

	c, ok := h.landingPwaClaims(r, false)
	if !ok {
		http.NotFound(w, r)
		return
	}
	page, err := h.store.LoadLandingPage(r.Context(), c.OrganizationID, c.LandingID)
	if err != nil {
		if err != routingstore.ErrNotFound {
			h.logger.Error("loading landing for its app page, sending onward", "error", err, "landing_id", c.LandingID)
		}
		outcome = outcomeRedirected
		h.afterLanding(w, r, c, h.claimsEvent(r, c))
		return
	}
	base := landingPwaBase(r)
	if target := bounceTarget(r.UserAgent(), base); target != "" {
		// No event: the real view is recorded when it loads in the browser.
		outcome = outcomeLanding
		writeHTML(w, renderBounce(target, pageLang(r)))
		return
	}

	if !c.Test {
		view := h.claimsEvent(r, c)
		view.Type = event.LandView
		h.events.Enqueue(view)
		h.events.Enqueue(h.landingEvent(r, event.PwaView, c))
	}
	outcome = outcomeLanding
	assetBase := ""
	if h.assets != nil && page.BundlePrefix != "" {
		assetBase = h.assets.PublicURL(page.BundlePrefix + "/")
	}
	trackURL := requestOrigin(r) + landClickPath + chi.URLParam(r, "token")
	writeHTML(w, []byte(landing.RenderWithHead(page.Content, assetBase, trackURL, landingPwaHead(base, page.LogoURL))))
}

// landingPwaHead makes the landing installable: the manifest, the icon iOS
// uses, the service worker, and the install beacon. window.floxPwa lets
// the landing's own design offer an install button (install() shows the
// browser's prompt when it has one; installable() says whether it does;
// the "floxpwa:installable" event fires when it becomes so).
func landingPwaHead(base, logoURL string) string {
	b, _ := json.Marshal(base)
	var sb strings.Builder
	sb.WriteString(`<link rel="manifest" href="` + html.EscapeString(base) + `manifest.webmanifest">`)
	sb.WriteString(`<meta name="mobile-web-app-capable" content="yes"><meta name="apple-mobile-web-app-capable" content="yes">`)
	if logoURL != "" {
		sb.WriteString(`<link rel="apple-touch-icon" href="` + html.EscapeString(logoURL) + `">`)
	}
	sb.WriteString(`<script>(function(){var base=` + string(b) + `;var deferred=null;
if("serviceWorker" in navigator){navigator.serviceWorker.register(base+"sw.js",{scope:base}).catch(function(){});}
window.addEventListener("beforeinstallprompt",function(e){deferred=e;window.dispatchEvent(new Event("floxpwa:installable"));});
window.addEventListener("appinstalled",function(){deferred=null;try{fetch(base+"installed",{method:"POST",keepalive:true});}catch(err){}});
window.floxPwa={install:function(){if(!deferred)return false;deferred.prompt();return true;},installable:function(){return !!deferred;}};
})();</script>`)
	return sb.String()
}

func (h *Handler) landingPwaManifest(w http.ResponseWriter, r *http.Request) {
	c, ok := h.landingPwaClaims(r, true)
	if !ok {
		http.NotFound(w, r)
		return
	}
	page, err := h.store.LoadLandingPage(r.Context(), c.OrganizationID, c.LandingID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	base := landingPwaPrefix + chi.URLParam(r, "token") + "/"
	name := page.Title
	if name == "" {
		name = "App"
	}
	manifest := map[string]any{
		"id":               base,
		"name":             name,
		"short_name":       name,
		"start_url":        base + "open",
		"scope":            base,
		"display":          "standalone",
		"theme_color":      "#ffffff",
		"background_color": "#ffffff",
	}
	if page.LogoURL != "" {
		manifest["icons"] = []map[string]string{
			{"src": page.LogoURL, "sizes": "192x192", "type": iconType(page.LogoURL), "purpose": "any"},
			{"src": page.LogoURL, "sizes": "512x512", "type": iconType(page.LogoURL), "purpose": "any"},
		}
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(manifest)
}

func (h *Handler) landingPwaInstalled(w http.ResponseWriter, r *http.Request) {
	c, ok := h.landingPwaClaims(r, false)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !c.Test {
		h.events.Enqueue(h.landingEvent(r, event.PwaInstall, c))
	}
	w.WriteHeader(http.StatusNoContent)
}

// landingPwaOpen is the installed landing's start_url: PWA_OPEN (and on an
// iPhone's first launch IOS_INSTALL), then the funnel's next step after
// the landing.
func (h *Handler) landingPwaOpen(w http.ResponseWriter, r *http.Request) {
	c, ok := h.landingPwaClaims(r, true)
	if !ok {
		http.NotFound(w, r)
		return
	}
	open := h.landingEvent(r, event.PwaOpen, c)
	if !c.Test {
		path := landingPwaPrefix + chi.URLParam(r, "token") + "/"
		if iosFirstLaunchAt(w, r, open.OS, path) {
			install := open
			install.Type = event.IosInstall
			h.events.Enqueue(install)
		}
		h.events.Enqueue(open)
	}
	h.afterLanding(w, r, c, open)
}

// afterLanding sends the visitor to the step after the landing, as its
// click-through would: the PWA stage (a page, an external PWA or an app
// link), else the postlanding and the offer.
func (h *Handler) afterLanding(w http.ResponseWriter, r *http.Request, c landinglink.Claims, ev event.Event) {
	switch step := stepFromClaims(c); {
	case step.pageID != "":
		http.Redirect(w, r, pwaURL(requestOrigin(r), chi.URLParam(r, "token")), http.StatusFound)
	case step.redirectURL != "":
		h.sendToPwaRedirect(w, r, c, ev)
	default:
		h.continueOnward(w, r, c, func() event.Event { return ev })
	}
}
