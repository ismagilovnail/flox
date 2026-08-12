package main

import (
	"net/http"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/routing"
)

// stickyCookie is the parsed sf_{campaignId} cookie (§39-STICKY), whose
// value is "setId:flowId[:clickId]".
//
// It carries ClickID, which routing.StickyState deliberately does not:
// the click_id affects attribution only, never which flow gets selected,
// so internal/routing has no business seeing it (documented in that
// package). The tracker parses the full cookie here and hands the engine
// only the two fields a routing decision actually depends on.
type stickyCookie struct {
	StreamSetID string
	FlowID      string
	ClickID     string
}

func stickyCookieName(campaignID string) string { return "sf_" + campaignID }

func parseStickyCookie(r *http.Request, campaignID string) *stickyCookie {
	c, err := r.Cookie(stickyCookieName(campaignID))
	if err != nil || c.Value == "" {
		return nil
	}
	parts := strings.Split(c.Value, ":")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		// Malformed cookie: ignore it and re-pick, rather than routing on
		// half-parsed state.
		return nil
	}
	sc := &stickyCookie{StreamSetID: parts[0], FlowID: parts[1]}
	if len(parts) >= 3 {
		sc.ClickID = parts[2]
	}
	return sc
}

// routingState narrows the cookie to just what the routing engine needs.
func (s *stickyCookie) routingState() *routing.StickyState {
	if s == nil {
		return nil
	}
	return &routing.StickyState{StreamSetID: s.StreamSetID, FlowID: s.FlowID}
}

// stickyMaxAge is deliberately long (§39-STICKY: the cookie is the source
// of truth and must "survive Redis eviction, restarts, and cross-session
// returns"). A short-lived cookie would silently corrupt A/B tests by
// letting returning users be re-bucketed.
const stickyMaxAge = 365 * 24 * 60 * 60 // one year, in seconds

func setStickyCookie(w http.ResponseWriter, campaignID string, result routing.RouteResult, clickID string, keepClickID bool) {
	value := result.StreamSetID + ":" + result.FlowID
	if keepClickID {
		value += ":" + clickID
	}
	http.SetCookie(w, &http.Cookie{
		Name:     stickyCookieName(campaignID),
		Value:    value,
		Path:     "/",
		MaxAge:   stickyMaxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
