package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/macro"
	"github.com/ismagilovnail/flox/apps/internal/push"
)

// Web Push from installed PWAs (docs/push.md). A PWA with push enabled
// shows a short launch page when its app starts (pwaOpen): "allow
// notifications / not now" — the permission prompt needs a tap — then
// continues to the next stage (pwaGo). The subscription is stored bound to
// the click the app was installed from; a notification's click comes back
// through pushClick.

// pushStore is the slice of push.Repository the tracker uses.
type pushStore interface {
	SaveSubscription(ctx context.Context, s push.NewSubscription) (bool, error)
	RecordOpen(ctx context.Context, orgID, clickID, deliveryID string) (push.Open, bool, error)
}

func (h *Handler) registerPush(r chi.Router) {
	r.Get(pwaPathPrefix+"{token}/go", h.pwaGo)
	r.Post(pwaPathPrefix+"{token}/push/request", h.pushEvent(event.NotificationRequest))
	r.Post(pwaPathPrefix+"{token}/push/decline", h.pushEvent(event.NotificationDecline))
	r.Post(pwaPathPrefix+"{token}/push/subscribe", h.pushSubscribe)
	r.Get(pwaPathPrefix+"{token}/push/click/{delivery}", h.pushClick)
}

// pushReady reports whether an installed app with this token asks for
// notifications: push configured on the tracker, the PWA's switch on, a
// real visitor (a constructor test link never subscribes).
func (h *Handler) pushReady(c landinglink.Claims, pushEnabled bool) bool {
	return h.push != nil && h.vapidPublicKey != "" && pushEnabled && !c.Test
}

// pwaGo continues an installed app's launch after its launch page.
func (h *Handler) pwaGo(w http.ResponseWriter, r *http.Request) {
	c, ok := h.pwaClaims(r, true)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.continueOnward(w, r, c, func() event.Event { return h.claimsEvent(r, c) })
}

// pushEvent records a permission request or a decline from the launch page.
func (h *Handler) pushEvent(t event.Type) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, ok := h.pwaClaims(r, true)
		if !ok || h.push == nil {
			http.NotFound(w, r)
			return
		}
		h.recordClaimsEvent(r, t, c)
		w.WriteHeader(http.StatusNoContent)
	}
}

type subscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// pushSubscribe stores the app's subscription and records
// NOTIFICATION_SUBSCRIBE when it is new. The endpoint is checked here
// (https, a known push service): the worker will post to it.
func (h *Handler) pushSubscribe(w http.ResponseWriter, r *http.Request) {
	c, ok := h.pwaClaims(r, true)
	if !ok || h.push == nil || c.Test {
		http.NotFound(w, r)
		return
	}
	var req subscribeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil ||
		!push.ValidEndpoint(req.Endpoint) || !push.ValidKeys(req.Keys.P256dh, req.Keys.Auth) {
		http.Error(w, "invalid subscription", http.StatusBadRequest)
		return
	}
	ev := h.claimsEvent(r, c)
	claims, _ := json.Marshal(c)
	created, err := h.push.SaveSubscription(r.Context(), push.NewSubscription{
		OrganizationID: c.OrganizationID,
		PwaID:          c.PwaID,
		CampaignID:     c.CampaignID,
		ClickID:        c.ClickID,
		Endpoint:       req.Endpoint,
		P256dh:         req.Keys.P256dh,
		Auth:           req.Keys.Auth,
		LinkToken:      chi.URLParam(r, "token"),
		Context:        claims,
		Origin:         requestOrigin(r),
		Country:        ev.Country,
		Device:         ev.Device,
		Language:       ev.Language,
	})
	if err != nil {
		h.logger.Error("storing push subscription", "error", err, "pwa_id", c.PwaID)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if created {
		ev.Type = event.NotificationSubscribe
		h.events.Enqueue(ev)
	}
	w.WriteHeader(http.StatusNoContent)
}

// pushClick is a notification's click-through: NOTIFICATION_CLICK on the
// subscriber's click, the broadcast's open counted once per delivery, then
// the broadcast's target — its URL (the click's macros resolved), or the
// next stage of the click's own journey. A delivery that is not this
// click's, or no longer kept, still leads on; it is just not counted.
func (h *Handler) pushClick(w http.ResponseWriter, r *http.Request) {
	c, ok := h.pwaClaims(r, true)
	if !ok || h.push == nil || c.Test {
		http.NotFound(w, r)
		return
	}
	open, found, err := h.push.RecordOpen(r.Context(), c.OrganizationID, c.ClickID, chi.URLParam(r, "delivery"))
	if err != nil {
		h.logger.Error("recording push open", "error", err)
	}
	click := h.claimsEvent(r, c)
	click.Type = event.NotificationClick
	h.events.Enqueue(click)
	if found && open.Target == push.TargetURL && open.URL != "" {
		http.Redirect(w, r, macro.ResolveURL(open.URL, macro.Values{
			ClickID: c.ClickID, CampaignID: c.CampaignID, OfferID: c.OfferID,
			Country: click.Country, Device: click.Device, Subs: landinglink.ExpandSubs(c.Subs),
		}), http.StatusFound)
		return
	}
	h.continueOnward(w, r, c, func() event.Event { return click })
}

//go:embed templates/launch.html
var launchPageSource string

var launchTemplate = template.Must(template.New("launch").Parse(launchPageSource))

var launchCopy = map[string]map[string]string{
	"en": {"title": "Stay in the loop", "text": "Allow notifications so you don't miss anything new.", "allow": "Allow notifications", "later": "Not now"},
	"ru": {"title": "Будьте в курсе", "text": "Разрешите уведомления, чтобы не пропустить новое.", "allow": "Разрешить уведомления", "later": "Не сейчас"},
}

func renderLaunch(name, icon, accent, vapidKey, lang string) ([]byte, error) {
	var buf bytes.Buffer
	err := launchTemplate.Execute(&buf, map[string]any{
		"Lang":     lang,
		"Name":     name,
		"Icon":     icon,
		"Accent":   cssColor(accent, "#1a73e8"),
		"VAPIDKey": vapidKey,
		"Copy":     launchCopy[lang],
	})
	return buf.Bytes(), err
}
