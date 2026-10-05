package main

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/classifier"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/routing"
	"github.com/ismagilovnail/flox/apps/internal/routingstore"
)

// PWA stage (docs/pwa-serving.md). A flow's PWA stage of type "internal"
// sends the visitor to an install page this tracker serves at
// /pwa/{token}/ — right after the click, or after an internal landing's
// click-through. Everything under that prefix shares one signed token
// (apps/internal/landinglink), so the page, its manifest, its service
// worker scope and the installed app's start_url all stay bound to the
// original click_id for as long as the app is installed.
//
//	GET  /pwa/{token}/                     install page            → PWA_VIEW
//	GET  /pwa/{token}/manifest.webmanifest web app manifest
//	GET  /pwa/{token}/sw.js                service worker (scope = /pwa/{token}/)
//	POST /pwa/{token}/installed            page beacon on appinstalled → PWA_INSTALL
//	GET  /pwa/{token}/open                 installed app's start_url → PWA_OPEN → destination
//	GET  /pwa/{token}/continue             "continue without installing" → destination
//
// The install page is the operator's own app presented as itself — name,
// icon, colors, developer, description, screenshots. It deliberately does
// NOT imitate any app store: no store branding or chrome, and none of the
// store-listing fields (ratings, review counts, download counts) the PWA
// editor also holds are ever shown to a visitor. A page that passes itself
// off as an official store with made-up social proof deceives the people
// installing; serving that is out of scope for this tracker.
const pwaPathPrefix = "/pwa/"

func (h *Handler) registerPwa(r chi.Router) {
	r.Get(pwaPathPrefix+"{token}", h.pwaRedirectSlash)
	r.Get(pwaPathPrefix+"{token}/", h.pwaPage)
	r.Get(pwaPathPrefix+"{token}/manifest.webmanifest", h.pwaManifest)
	r.Get(pwaPathPrefix+"{token}/sw.js", h.pwaServiceWorker)
	r.Post(pwaPathPrefix+"{token}/installed", h.pwaInstalled)
	r.Get(pwaPathPrefix+"{token}/open", h.pwaOpen)
	r.Get(pwaPathPrefix+"{token}/continue", h.pwaContinue)
}

// decidePwa returns the flow's PWA id when its PWA stage applies: enabled,
// type "internal" (the only type with a page to serve — "external" and
// "ios_app" carry no target URL in this schema, so they're skipped rather
// than guessed at), not paused, and click links signable.
func (h *Handler) decidePwa(loaded routingstore.CampaignRouting, flowID string) string {
	if h.links == nil || flowID == "" {
		return ""
	}
	stage, ok := loaded.FlowPwas[flowID]
	if !ok || stage.Type != "internal" || stage.Status == "paused" {
		return ""
	}
	return stage.PwaID
}

func pwaURL(origin, token string) string { return origin + pwaPathPrefix + token + "/" }

func (h *Handler) pwaRedirectSlash(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, pwaPathPrefix+chi.URLParam(r, "token")+"/", http.StatusMovedPermanently)
}

// pwaClaims verifies the request's token. anyAge relaxes MaxAge for the
// routes an installed app keeps using (manifest, start_url).
func (h *Handler) pwaClaims(r *http.Request, anyAge bool) (landinglink.Claims, bool) {
	if h.links == nil {
		return landinglink.Claims{}, false
	}
	verify := h.links.Verify
	if anyAge {
		verify = h.links.VerifyAnyAge
	}
	c, err := verify(chi.URLParam(r, "token"), time.Now())
	if err != nil || c.PwaID == "" {
		return landinglink.Claims{}, false
	}
	return c, true
}

// loadPwa reads the page; a paused PWA is unavailable to real traffic
// (test links still see it — testing a paused draft is the point).
func (h *Handler) loadPwa(r *http.Request, c landinglink.Claims) (routingstore.PwaPage, bool) {
	p, err := h.store.LoadPwaPage(r.Context(), c.OrganizationID, c.PwaID)
	if err != nil {
		if err != routingstore.ErrNotFound {
			h.logger.Error("loading pwa page", "error", err, "pwa_id", c.PwaID)
		}
		return routingstore.PwaPage{}, false
	}
	if p.Status == "paused" && !c.Test {
		return routingstore.PwaPage{}, false
	}
	return p, true
}

func (h *Handler) pwaPage(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	outcome := outcomeNotFound
	defer func() {
		metrics.TrackingLatencySeconds.Observe(time.Since(start).Seconds())
		metrics.TrackingRequestsTotal.WithLabelValues(string(outcome)).Inc()
	}()

	c, ok := h.pwaClaims(r, false)
	if !ok {
		http.NotFound(w, r)
		return
	}
	page, ok := h.loadPwa(r, c)
	if !ok {
		// PWA gone or paused since the link was issued: don't strand the
		// visitor, send them on.
		outcome = outcomeRedirected
		http.Redirect(w, r, c.Destination, http.StatusFound)
		return
	}

	self := requestOrigin(r) + r.URL.Path
	if page.BounceInAppWebview {
		if target := bounceTarget(r.UserAgent(), self); target != "" {
			// §28/§73: provider-neutral in-app WebView → external browser
			// bounce so the install prompt can fire. No event here: the
			// real view is recorded when the page loads in the browser.
			outcome = outcomePwa
			writeHTML(w, renderBounce(target, pageLang(r)))
			return
		}
	}

	h.recordClaimsEvent(r, event.PwaView, c)
	outcome = outcomePwa
	writeHTML(w, renderPwaPage(page, c.Test, pageLang(r)))
}

func (h *Handler) pwaManifest(w http.ResponseWriter, r *http.Request) {
	c, ok := h.pwaClaims(r, true)
	if !ok {
		http.NotFound(w, r)
		return
	}
	page, ok := h.loadPwa(r, c)
	if !ok {
		http.NotFound(w, r)
		return
	}
	base := pwaPathPrefix + chi.URLParam(r, "token") + "/"
	short := page.ShortName
	if short == "" {
		short = page.Name
	}
	manifest := map[string]any{
		"id":               base,
		"name":             page.Name,
		"short_name":       short,
		"description":      page.Description,
		"start_url":        base + "open",
		"scope":            base,
		"display":          "standalone",
		"theme_color":      page.ThemeColor,
		"background_color": page.BackgroundColor,
		"icons": []map[string]string{
			{"src": page.IconURL, "sizes": "192x192", "type": iconType(page.IconURL), "purpose": "any"},
			{"src": page.IconURL, "sizes": "512x512", "type": iconType(page.IconURL), "purpose": "any"},
		},
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(manifest)
}

// pwaServiceWorker: installability needs a registered worker with a fetch
// handler. This one only passes requests through — no caching, so a
// visitor never sees a stale page and nothing about the click is stored.
func (h *Handler) pwaServiceWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(`self.addEventListener("install", function () { self.skipWaiting(); });
self.addEventListener("activate", function (e) { e.waitUntil(self.clients.claim()); });
self.addEventListener("fetch", function (e) { e.respondWith(fetch(e.request)); });
`))
}

func (h *Handler) pwaInstalled(w http.ResponseWriter, r *http.Request) {
	c, ok := h.pwaClaims(r, false)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.recordClaimsEvent(r, event.PwaInstall, c)
	w.WriteHeader(http.StatusNoContent)
}

// pwaOpen is the installed app's start_url: every launch records PWA_OPEN
// on the original click_id and continues to the destination.
func (h *Handler) pwaOpen(w http.ResponseWriter, r *http.Request) {
	c, ok := h.pwaClaims(r, true)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.recordClaimsEvent(r, event.PwaOpen, c)
	http.Redirect(w, r, c.Destination, http.StatusFound)
}

// pwaContinue is the page's "continue without installing" — a browser
// that can't install (desktop, unsupported) must never strand a visitor.
// Not an open and not an install, so no PWA event.
func (h *Handler) pwaContinue(w http.ResponseWriter, r *http.Request) {
	c, ok := h.pwaClaims(r, false)
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, c.Destination, http.StatusFound)
}

// recordClaimsEvent records t on the token's click_id with this request's
// geo/device — unless the token is a constructor test link.
func (h *Handler) recordClaimsEvent(r *http.Request, t event.Type, c landinglink.Claims) {
	if c.Test {
		return
	}
	ip := h.clientIP.NetIP(r)
	attrs, err := h.classifier.Classify(r.Context(), classifier.Input{
		IP:             ip,
		UserAgent:      r.UserAgent(),
		AcceptLanguage: r.Header.Get("Accept-Language"),
	})
	if err != nil {
		h.logger.Error("classifying request", "error", err)
		attrs = routing.Attributes{}
	}
	ev := landClickEvent(c, attrs, ip, r)
	ev.Type = t
	h.events.Enqueue(ev)
}

// inAppWebviewMarkers identify in-app browsers by their own self-
// declared user-agent tokens — the generic, provider-neutral WebView
// detection §73 requires for the PWA install bounce. This answers "can
// this browser install a PWA", never "who is this visitor": it is not,
// and must never become, moderator/reviewer detection (also §73).
var inAppWebviewMarkers = []string{"FBAN/", "FBAV/", "FB_IAB", "Instagram", "BytedanceWebview", "musical_ly", "TikTok", "Line/", "Snapchat"}

// bounceTarget returns the URL that reopens self in the device's external
// browser, or "" when the request isn't from an in-app WebView (or the
// platform has no known way out).
func bounceTarget(ua, self string) string {
	inApp := false
	for _, m := range inAppWebviewMarkers {
		if strings.Contains(ua, m) {
			inApp = true
			break
		}
	}
	if !inApp {
		return ""
	}
	u, err := url.Parse(self)
	if err != nil {
		return ""
	}
	switch {
	case strings.Contains(ua, "Android"):
		// Android intent: hand the URL to the default browser.
		return "intent://" + u.Host + u.RequestURI() + "#Intent;scheme=" + u.Scheme + ";action=android.intent.action.VIEW;S.browser_fallback_url=" + url.QueryEscape(self) + ";end"
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad"):
		return "x-safari-" + self
	}
	return ""
}

func iconType(u string) string {
	switch {
	case strings.HasSuffix(strings.ToLower(u), ".webp"):
		return "image/webp"
	case strings.HasSuffix(strings.ToLower(u), ".jpg"), strings.HasSuffix(strings.ToLower(u), ".jpeg"):
		return "image/jpeg"
	default:
		return "image/png"
	}
}

func writeHTML(w http.ResponseWriter, page []byte) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(page)
}

// pageLang picks the visitor-facing copy: Russian when the browser prefers
// it, English otherwise.
func pageLang(r *http.Request) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Accept-Language"))), "ru") {
		return "ru"
	}
	return "en"
}

var pwaCopy = map[string]map[string]string{
	"ru": {
		"install": "Установить", "continue": "Продолжить без установки", "by": "Разработчик",
		"iosTitle": "Как установить на iPhone", "iosStep1": "Нажмите «Поделиться» в нижней панели Safari",
		"iosStep2": "Выберите «На экран Домой»", "iosStep3": "Откройте приложение с главного экрана",
		"installed": "Приложение установлено — откройте его с главного экрана", "open": "Открыть",
		"test":        "Тестовый режим: события не записываются",
		"bounceTitle": "Откройте страницу в браузере", "bounceHint": "Если ничего не произошло, нажмите ⋯ и выберите «Открыть в браузере».",
		"bounceButton": "Открыть в браузере",
	},
	"en": {
		"install": "Install", "continue": "Continue without installing", "by": "Developer",
		"iosTitle": "Install on iPhone", "iosStep1": "Tap Share in Safari's toolbar",
		"iosStep2": "Choose “Add to Home Screen”", "iosStep3": "Open the app from your home screen",
		"installed": "App installed — open it from your home screen", "open": "Open",
		"test":        "Test mode: no events are recorded",
		"bounceTitle": "Open this page in your browser", "bounceHint": "If nothing happens, tap ⋯ and choose “Open in browser”.",
		"bounceButton": "Open in browser",
	},
}

var pwaPageTemplate = template.Must(template.New("pwa").Parse(`<!DOCTYPE html>
<html lang="{{.Lang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>{{.Page.Name}}</title>
<meta name="theme-color" content="{{.Theme}}">
<link rel="manifest" href="manifest.webmanifest">
<link rel="icon" href="{{.Page.IconURL}}">
<link rel="apple-touch-icon" href="{{.Page.IconURL}}">
<meta name="apple-mobile-web-app-capable" content="yes">
<meta name="apple-mobile-web-app-title" content="{{.Short}}">
<style>
:root{--theme:{{.Theme}};--bg:{{.Background}}}
*{box-sizing:border-box}
body{margin:0;font:16px/1.45 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;background:var(--bg);color:#1b1b1f}
main{max-width:640px;margin:0 auto;padding:24px 16px 40px}
.test{background:#fff4d6;color:#6b4e00;border-radius:8px;padding:8px 12px;font-size:13px;margin-bottom:16px}
.head{display:flex;gap:16px;align-items:center}
.icon{width:88px;height:88px;border-radius:22px;object-fit:cover;flex:none;box-shadow:0 1px 4px rgba(0,0,0,.15)}
h1{font-size:24px;margin:0}
.dev{color:#55565c;font-size:14px;margin-top:4px}
.actions{margin:24px 0}
button,.btn{display:block;width:100%;border:0;border-radius:12px;padding:14px 16px;font-size:16px;font-weight:600;text-align:center;text-decoration:none;cursor:pointer}
.primary{background:var(--theme);color:#fff}
.secondary{background:transparent;color:#55565c;font-weight:500;margin-top:8px}
.shots{display:flex;gap:10px;overflow-x:auto;padding:4px 0 8px;scroll-snap-type:x mandatory}
.shots img{height:280px;border-radius:12px;scroll-snap-align:start;flex:none}
.desc{white-space:pre-line;color:#2c2d31}
.ios{background:#fff;border-radius:12px;padding:16px;margin-top:16px;display:none}
.ios h2{font-size:16px;margin:0 0 8px}
.ios ol{margin:0;padding-left:20px}
.done{display:none;text-align:center;color:#2c2d31;margin-top:12px}
[hidden]{display:none!important}
</style>
</head>
<body>
<main>
{{if .Test}}<div class="test">{{index .Copy "test"}}</div>{{end}}
<div class="head">
  {{if .Page.IconURL}}<img class="icon" src="{{.Page.IconURL}}" alt="">{{end}}
  <div>
    <h1>{{.Page.Name}}</h1>
    {{if .Page.Developer}}<div class="dev">{{index .Copy "by"}}: {{.Page.Developer}}</div>{{end}}
  </div>
</div>
<div class="actions">
  <button id="install" class="primary" hidden>{{index .Copy "install"}}</button>
  <a id="open" class="btn primary" href="open" hidden>{{index .Copy "open"}}</a>
  <div id="done" class="done">{{index .Copy "installed"}}</div>
  <div id="ios" class="ios">
    <h2>{{index .Copy "iosTitle"}}</h2>
    <ol><li>{{index .Copy "iosStep1"}}</li><li>{{index .Copy "iosStep2"}}</li><li>{{index .Copy "iosStep3"}}</li></ol>
  </div>
  <a class="btn secondary" href="continue">{{index .Copy "continue"}}</a>
</div>
{{if .Page.Screenshots}}<div class="shots">{{range .Page.Screenshots}}<img src="{{.}}" alt="" loading="lazy">{{end}}</div>{{end}}
{{if .Page.Description}}<p class="desc">{{.Page.Description}}</p>{{end}}
</main>
<script>
(function () {
  if ("serviceWorker" in navigator) navigator.serviceWorker.register("sw.js", { scope: "./" });
  var installBtn = document.getElementById("install");
  var reported = false;
  function reportInstalled() {
    if (reported) return;
    reported = true;
    if (navigator.sendBeacon) navigator.sendBeacon("installed");
    else fetch("installed", { method: "POST", keepalive: true });
  }
  var standalone = window.matchMedia("(display-mode: standalone)").matches || navigator.standalone === true;
  if (standalone) { location.replace("open"); return; }
  var ua = navigator.userAgent;
  var isIOS = /iPhone|iPad|iPod/.test(ua) || (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);
  if (isIOS) document.getElementById("ios").style.display = "block";
  var deferred = null;
  window.addEventListener("beforeinstallprompt", function (e) {
    e.preventDefault();
    deferred = e;
    installBtn.hidden = false;
  });
  installBtn.addEventListener("click", function () {
    if (!deferred) return;
    deferred.prompt();
    deferred.userChoice.then(function (choice) {
      if (choice.outcome === "accepted") reportInstalled();
      deferred = null;
      installBtn.hidden = true;
    });
  });
  window.addEventListener("appinstalled", function () {
    reportInstalled();
    installBtn.hidden = true;
    document.getElementById("done").style.display = "block";
    document.getElementById("open").hidden = false;
  });
})();
</script>
</body>
</html>
`))

func renderPwaPage(p routingstore.PwaPage, test bool, lang string) []byte {
	short := p.ShortName
	if short == "" {
		short = p.Name
	}
	var b strings.Builder
	_ = pwaPageTemplate.Execute(&b, map[string]any{
		"Lang": lang, "Copy": pwaCopy[lang], "Page": p, "Test": test, "Short": short,
		"Theme": cssColor(p.ThemeColor, "#1a73e8"), "Background": cssColor(p.BackgroundColor, "#ffffff"),
	})
	return []byte(b.String())
}

var bounceTemplate = template.Must(template.New("bounce").Parse(`<!DOCTYPE html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{index .Copy "bounceTitle"}}</title>
<style>body{margin:0;font:16px/1.45 system-ui,sans-serif;display:flex;min-height:100vh;align-items:center;justify-content:center;text-align:center;padding:24px}a{display:inline-block;margin-top:16px;padding:12px 20px;border-radius:10px;background:#1a73e8;color:#fff;text-decoration:none}</style>
</head><body><div>
<h1 style="font-size:20px">{{index .Copy "bounceTitle"}}</h1>
<p>{{index .Copy "bounceHint"}}</p>
<a href="{{.Target}}">{{index .Copy "bounceButton"}}</a>
</div>
<script>location.href = {{.Target}};</script>
</body></html>
`))

func renderBounce(target, lang string) []byte {
	var b strings.Builder
	_ = bounceTemplate.Execute(&b, map[string]any{"Lang": lang, "Copy": pwaCopy[lang], "Target": template.URL(target)})
	return []byte(b.String())
}

// cssColor admits only "#rgb"/"#rrggbb" — the values pwa's own service
// validates — so nothing else can reach the page's <style>.
func cssColor(v, fallback string) template.CSS {
	if len(v) == 4 || len(v) == 7 {
		if v[0] == '#' && strings.Trim(strings.ToLower(v[1:]), "0123456789abcdef") == "" {
			return template.CSS(v)
		}
	}
	return template.CSS(fallback)
}
