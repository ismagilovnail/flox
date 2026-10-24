package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/macro"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/postlanding"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
)

// postClickPath is the tracker route a postlanding's {next_url} points at.
const postClickPath = "/pc/"

// decidePostlanding returns the routed flow's postlanding stage when it
// applies: enabled, not paused, with a URL, and click links signable (the
// stage exists to hand the page a signed {next_url}). The caller decides
// reachability — a postlanding behind an external landing is never reached.
// Archived postlandings are still served, as landings are.
func (h *Handler) decidePostlanding(loaded routingstore.CampaignRouting, flowID string) (routingstore.FlowPostlanding, bool) {
	if h.links == nil || flowID == "" {
		return routingstore.FlowPostlanding{}, false
	}
	stage, ok := loaded.FlowPostlandings[flowID]
	if !ok || !servable(stage) {
		return routingstore.FlowPostlanding{}, false
	}
	return stage, true
}

func servable(p routingstore.FlowPostlanding) bool {
	return p.URL != "" && p.Status != string(postlanding.StatusPaused)
}

// sendToPostlanding records POSTLANDING_VIEW and redirects the visitor to
// the postlanding (docs/postlanding.md, "Serving visitors"). The view is
// recorded when the tracker sends the visitor there: the page is someone
// else's, so whether it finished loading is not known (decision 2026-10-13).
//
// The URL's macros are the click's, plus {next_url}: a signed link on this
// tracker that records POSTLANDING_CLICK and continues to the destination.
// It is re-issued now, so a journey resumed from an old token — an
// installed PWA's launch — still gets the full link lifetime.
func (h *Handler) sendToPostlanding(w http.ResponseWriter, r *http.Request, c landinglink.Claims, url string, view event.Event) {
	if !c.Test {
		view.Type = event.PostlandingView
		h.events.Enqueue(view)
	}

	c.IssuedAt = time.Now().Unix()
	next := requestOrigin(r) + postClickPath + h.links.Sign(c)
	target := macro.ResolveURL(url, macro.Values{
		ClickID:    c.ClickID,
		CampaignID: c.CampaignID,
		OfferID:    c.OfferID,
		Country:    view.Country,
		Device:     view.Device,
		Subs:       landinglink.ExpandSubs(c.Subs),
		NextURL:    next,
	})
	http.Redirect(w, r, target, http.StatusFound)
}

// continueOnward sends a visitor holding a verified link to the next stage
// after the current one: the postlanding when the link carries one that is
// still servable, else the destination. The postlanding is read as it is
// now — paused or emptied since the link was issued, it is skipped rather
// than a dead end. view builds the visitor's event, only when needed.
func (h *Handler) continueOnward(w http.ResponseWriter, r *http.Request, c landinglink.Claims, view func() event.Event) {
	if c.PostlandingID != "" && h.links != nil {
		p, err := h.store.LoadPostlanding(r.Context(), c.OrganizationID, c.PostlandingID)
		switch {
		case err == nil && servable(p):
			h.sendToPostlanding(w, r, c, p.URL, view())
			return
		case err != nil && !errors.Is(err, routingstore.ErrNotFound):
			h.logger.Error("loading postlanding, redirecting past it", "error", err, "postlanding_id", c.PostlandingID)
		}
	}
	http.Redirect(w, r, c.Destination, http.StatusFound)
}

// postClick answers GET /pc/{token} — the visitor leaving a postlanding
// through its {next_url}. Records POSTLANDING_CLICK on the original
// click_id and redirects to the destination. Like /lc/, no database read:
// everything is in the signed token. Every click is recorded.
func (h *Handler) postClick(w http.ResponseWriter, r *http.Request) {
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
	c, err := h.links.Verify(chi.URLParam(r, "token"), time.Now())
	// Only on the host the link was issued for (tech audit M4), and only a
	// link issued for a postlanding.
	if err != nil || c.PostlandingID == "" || !c.OnHost(r.Host) {
		http.NotFound(w, r)
		return
	}
	// Over the per-IP limit (audit SEC-4): onward as usual, unrecorded.
	if now := time.Now(); h.clicks.allow(h.clientIP.NetIP(r), now) {
		h.recordClaimsEvent(r, event.PostlandingClick, c)
		outcome = outcomePostClick
	} else {
		outcome = outcomeRateLimited
		h.limited.Add(c.OrganizationID, c.CampaignID, now)
	}
	http.Redirect(w, r, c.Destination, http.StatusFound)
}
