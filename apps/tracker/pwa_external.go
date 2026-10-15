package main

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/macro"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
)

// PWA types (docs/pwa-serving.md "PWA types"): a flow's PWA stage is an
// internal page this tracker serves (pwa.go), an external PWA it sends the
// visitor to — which reports back through /pwx/ — or, for ios_app, an app
// link for iPhone/iPad visitors (other devices get the stage's internal PWA
// if one is set, or skip the stage).

const (
	pwaTypeInternal = "internal"
	pwaTypeExternal = "external"
	pwaTypeIOSApp   = "ios_app"
)

// pwaExternalPrefix is where an external PWA reports: {pwa_link} is
// <origin>/pwx/<token>/, and the PWA calls {pwa_link}installed and uses
// {pwa_link}open as its start_url.
const pwaExternalPrefix = "/pwx/"

// pwaStep is what the PWA stage does for this visitor: show an internal
// PWA's page (pageID), or redirect (redirectType external / ios_app, to
// redirectURL — macros unresolved). The zero value: no PWA stage.
type pwaStep struct {
	pageID       string
	redirectType string
	redirectURL  string
}

// choosePwaStep picks the step for a visitor on os (the classifier's
// value, "ios" for iPhone/iPad) from a flow's stage.
func choosePwaStep(stage routingstore.FlowPwa, os string) pwaStep {
	switch stage.Type {
	case pwaTypeInternal:
		return pwaStep{pageID: stage.PwaID}
	case pwaTypeExternal:
		return pwaStep{redirectType: pwaTypeExternal, redirectURL: stage.URL}
	case pwaTypeIOSApp:
		if os == "ios" {
			return pwaStep{redirectType: pwaTypeIOSApp, redirectURL: stage.URL}
		}
		return pwaStep{pageID: stage.PwaID}
	}
	return pwaStep{}
}

// applyTo carries the step on the links that lead to it (the landing's
// click link, {pwa_link}).
func (s pwaStep) applyTo(c *landinglink.Claims) {
	c.PwaID = s.pageID
	c.PwaType = s.redirectType
	c.PwaURL = s.redirectURL
}

// stepFromClaims is the step a signed link carries.
func stepFromClaims(c landinglink.Claims) pwaStep {
	return pwaStep{pageID: c.PwaID, redirectType: c.PwaType, redirectURL: c.PwaURL}
}

// sendToPwaRedirect takes the visitor to an external PWA or an app link,
// its URL resolved against the visit (ev: its geo/device and subs). An
// external PWA gets {pwa_link} and PWA_VIEW is recorded — the visitor was
// sent to the PWA; an app link records nothing (an App Store install is
// not something this tracker sees: it comes back as a conversion).
func (h *Handler) sendToPwaRedirect(w http.ResponseWriter, r *http.Request, c landinglink.Claims, ev event.Event) {
	values := clickMacros(ev)
	if c.PwaType == pwaTypeExternal {
		values.PwaLink = requestOrigin(r) + pwaExternalPrefix + h.links.Sign(c) + "/"
		if !c.Test {
			view := ev
			view.Type, view.EventAt, view.PwaID = event.PwaView, time.Now().UTC(), ""
			h.events.Enqueue(view)
		}
	}
	http.Redirect(w, r, macro.ResolveURL(c.PwaURL, values), http.StatusFound)
}

func (h *Handler) registerPwaExternal(r chi.Router) {
	r.Get(pwaExternalPrefix+"{token}/installed", h.pwaExternalInstalled)
	r.Post(pwaExternalPrefix+"{token}/installed", h.pwaExternalInstalled)
	r.Options(pwaExternalPrefix+"{token}/installed", h.pwaExternalPreflight)
	r.Get(pwaExternalPrefix+"{token}/open", h.pwaExternalOpen)
}

// pwaExternalClaims verifies a {pwa_link} token: issued for an external
// PWA, on this host. anyAge for the start_url an installed app keeps.
func (h *Handler) pwaExternalClaims(r *http.Request, anyAge bool) (landinglink.Claims, bool) {
	if h.links == nil {
		return landinglink.Claims{}, false
	}
	verify := h.links.Verify
	if anyAge {
		verify = h.links.VerifyAnyAge
	}
	c, err := verify(chi.URLParam(r, "token"), time.Now())
	if err != nil || c.PwaType != pwaTypeExternal || !c.OnHost(r.Host) {
		return landinglink.Claims{}, false
	}
	return c, true
}

// pwaExternalInstalled records PWA_INSTALL for the external PWA's click:
// GET (an image beacon), POST (fetch / navigator.sendBeacon) — from the
// PWA's own origin, so CORS is open; the signed token is what authorises.
func (h *Handler) pwaExternalInstalled(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")
	c, ok := h.pwaExternalClaims(r, false)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.recordExternal(r, event.PwaInstall, c)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) pwaExternalPreflight(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.WriteHeader(http.StatusNoContent)
}

// pwaExternalOpen is the external PWA's start_url, if it uses
// {pwa_link}open: PWA_OPEN on the click, then on through the funnel
// (postlanding, offer) like an internal PWA's launch.
func (h *Handler) pwaExternalOpen(w http.ResponseWriter, r *http.Request) {
	c, ok := h.pwaExternalClaims(r, true)
	if !ok {
		http.NotFound(w, r)
		return
	}
	open := h.claimsEvent(r, c)
	open.PwaID = ""
	if !c.Test {
		open.Type = event.PwaOpen
		h.events.Enqueue(open)
	}
	h.continueOnward(w, r, c, func() event.Event { return open })
}

func (h *Handler) recordExternal(r *http.Request, t event.Type, c landinglink.Claims) {
	if c.Test {
		return
	}
	ev := h.claimsEvent(r, c)
	ev.Type, ev.PwaID = t, ""
	h.events.Enqueue(ev)
}
