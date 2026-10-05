package main

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
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
		"pleaseWait":   "Пожалуйста, подождите…", "about": "О приложении", "aboutOpen": "Подробнее", "close": "Закрыть",
		"whatsNew": "Что нового", "updated": "Дата обновления", "details": "Подробная информация",
		"dUpdated": "Обновлено", "dVersion": "Текущая версия", "dDeveloper": "Разработчик",
		"dCategory": "Категория", "dInteractive": "Интерактивные элементы",
		"ads": "Есть реклама", "iap": "Покупки в приложении",
	},
	"en": {
		"install": "Install", "continue": "Continue without installing", "by": "Developer",
		"iosTitle": "Install on iPhone", "iosStep1": "Tap Share in Safari's toolbar",
		"iosStep2": "Choose “Add to Home Screen”", "iosStep3": "Open the app from your home screen",
		"installed": "App installed — open it from your home screen", "open": "Open",
		"test":        "Test mode: no events are recorded",
		"bounceTitle": "Open this page in your browser", "bounceHint": "If nothing happens, tap ⋯ and choose “Open in browser”.",
		"bounceButton": "Open in browser",
		"pleaseWait":   "Please wait…", "about": "About this app", "aboutOpen": "More details", "close": "Close",
		"whatsNew": "What's new", "updated": "Updated on", "details": "Details",
		"dUpdated": "Updated", "dVersion": "Current version", "dDeveloper": "Developer",
		"dCategory": "Category", "dInteractive": "Interactive elements",
		"ads": "Contains ads", "iap": "In-app purchases",
	},
}

// pwaPageTemplate mirrors the PWA editor's own preview card (header block,
// ring-around-the-icon install animation, screenshot strip, About, What's
// new, details sheet) with its light/dark scheme and accent. It leaves out
// everything the editor can only fake: no rating, review count, downloads,
// age rating or "verified" badge, and no store logo, nav or footer chrome.
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
:root{--accent:{{.Theme}};--bg:{{.Bg}};--text:{{.Text}};--muted:{{.Muted}};--subtle:{{.Subtle}};--border:{{.Border}};--chip:{{.Chip}}}
*{box-sizing:border-box}
body{margin:0;font:16px/1.45 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:var(--bg);color:var(--text)}
main{max-width:480px;margin:0 auto;padding:20px 0 40px}
.pad{padding:0 20px}
.test{background:#fff4d6;color:#6b4e00;border-radius:8px;padding:8px 12px;font-size:13px;margin:0 20px 16px}
.head{display:flex;gap:16px;align-items:flex-start;padding-top:0;transition:padding-top .5s ease-out}
.head.busy{padding-top:12px}
.iconbox{position:relative;width:72px;height:72px;flex:none}
.iconbox .clip{position:absolute;inset:0;overflow:hidden;border-radius:16px;transition:all .5s ease-out}
.head.busy .clip{inset:6px;border-radius:50%}
.iconbox img{width:100%;height:100%;object-fit:cover;display:block}
.ring{position:absolute;inset:0;opacity:0;transition:opacity .25s ease-out}
.head.busy .ring{opacity:1}
.ring circle{fill:none;stroke-width:3}
.ring .track{stroke:var(--border)}
.ring .arc{stroke:var(--accent);stroke-linecap:round;stroke-dasharray:201.06;stroke-dashoffset:201.06;transform:rotate(-90deg);transform-origin:50% 50%}
.head.busy .ring .arc{stroke-dashoffset:0;transition:stroke-dashoffset 1.8s linear}
.meta{min-width:0;display:flex;flex-direction:column;gap:2px}
h1{font-size:22px;font-weight:600;line-height:1.2;margin:0}
.dev{font-size:16px;font-weight:500;color:var(--accent)}
.head.busy .dev{color:var(--text);font-weight:600}
.note{font-size:12px;color:var(--muted)}
.head.busy .note{display:none}
.actions{margin:20px 20px 0}
button,.btn{display:block;width:100%;border:0;border-radius:8px;padding:10px 16px;font-size:14px;font-weight:700;text-align:center;text-decoration:none;cursor:pointer;font-family:inherit}
.primary{background:var(--accent);color:#fff}
.secondary{background:transparent;color:var(--muted);font-weight:500;margin-top:8px}
.collapse{display:grid;grid-template-rows:1fr;opacity:1;transition:grid-template-rows .5s ease-out,opacity .5s ease-out}
.collapse>div{overflow:hidden}
.busy-actions .collapse{grid-template-rows:0fr;opacity:0}
.shots{display:flex;gap:12px;overflow-x:auto;padding:20px 16px 0;height:224px;scroll-snap-type:x proximity}
.shots img{height:204px;width:auto;border-radius:12px;flex:none;scroll-snap-align:start}
.sec{padding:20px 16px 0}
.sec h2{font-size:18px;font-weight:700;margin:0}
.row{display:flex;align-items:center;justify-content:space-between}
.arrow{width:auto;padding:4px 8px;background:transparent;color:var(--text);font-size:18px;border-radius:6px}
.arrow:hover{background:var(--chip)}
.desc{white-space:pre-line;font-size:14px;line-height:1.6;margin:12px 0 0;display:-webkit-box;-webkit-line-clamp:5;-webkit-box-orient:vertical;overflow:hidden}
.full{white-space:pre-line;font-size:14px;line-height:1.6;margin:0}
.upd{font-size:14px;font-weight:700;color:var(--subtle);margin:0 0 4px}
.upd+p{font-size:14px;margin:0}
.line{border-top:1px solid var(--border);margin-top:20px}
.news{font-size:14px;line-height:1.6;white-space:pre-line;margin:12px 0 0}
.ios{background:var(--chip);border-radius:12px;padding:16px;margin-top:16px;display:none}
.ios h2{font-size:16px;margin:0 0 8px}
.ios ol{margin:0;padding-left:20px}
.done{display:none;text-align:center;color:var(--muted);margin-top:12px;font-size:14px}
[hidden]{display:none!important}
dialog{width:calc(100% - 32px);max-width:448px;max-height:80%;padding:0;border:0;border-radius:16px;background:var(--bg);color:var(--text);box-shadow:0 20px 50px rgba(0,0,0,.3)}
dialog::backdrop{background:rgba(0,0,0,.3)}
.dh{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:12px 16px;border-bottom:1px solid var(--border)}
.dh b{font-size:16px}
.x{width:auto;padding:4px 10px;background:transparent;color:var(--muted);font-size:16px;border-radius:999px}
.x:hover{background:var(--chip)}
.db{padding:16px;display:flex;flex-direction:column;gap:20px}
.grid{display:grid;grid-template-columns:1fr 1fr;gap:16px}
.grid b{display:block;font-size:14px}
.grid span{display:block;font-size:14px;color:var(--muted)}
</style>
</head>
<body>
<main>
{{if .Test}}<div class="test">{{index .Copy "test"}}</div>{{end}}
<div class="pad">
<div class="head" id="head">
  {{if .Page.IconURL}}<div class="iconbox"><div class="clip"><img src="{{.Page.IconURL}}" alt=""></div>
    <svg class="ring" viewBox="0 0 72 72" aria-hidden="true"><circle class="track" cx="36" cy="36" r="32"/><circle class="arc" cx="36" cy="36" r="32"/></svg></div>{{end}}
  <div class="meta">
    <h1>{{.Page.Name}}</h1>
    <div class="dev" id="dev">{{if .Page.Developer}}{{.Page.Developer}}{{end}}</div>
    {{if .Notes}}<div class="note">{{.Notes}}</div>{{end}}
  </div>
</div>
</div>
<div class="actions" id="actions">
  <div class="collapse"><div>
    <button id="install" class="primary" hidden>{{index .Copy "install"}}</button>
    <a id="open" class="btn primary" href="open" hidden>{{index .Copy "open"}}</a>
  </div></div>
  <div id="done" class="done">{{index .Copy "installed"}}</div>
  <div id="ios" class="ios">
    <h2>{{index .Copy "iosTitle"}}</h2>
    <ol><li>{{index .Copy "iosStep1"}}</li><li>{{index .Copy "iosStep2"}}</li><li>{{index .Copy "iosStep3"}}</li></ol>
  </div>
  <a class="btn secondary" href="continue">{{index .Copy "continue"}}</a>
</div>
{{if .Page.Screenshots}}<div class="shots">{{range .Page.Screenshots}}<img src="{{.}}" alt="" loading="lazy">{{end}}</div>{{end}}
{{if .Page.Description}}<div class="sec">
  <div class="row"><h2>{{index .Copy "about"}}</h2><button class="arrow" id="more" type="button" aria-label="{{index .Copy "aboutOpen"}}">→</button></div>
  <p class="desc">{{.Page.Description}}</p>
</div>{{end}}
{{if .Page.UpdatedDate}}<div class="sec"><p class="upd">{{index .Copy "updated"}}</p><p>{{.Page.UpdatedDate}}</p></div>{{end}}
{{if .Page.WhatsNew}}<div class="line"></div><div class="sec"><h2>{{index .Copy "whatsNew"}}</h2><p class="news">{{.Page.WhatsNew}}</p></div>{{end}}
</main>
{{if .Page.Description}}<dialog id="sheet" aria-label="{{index .Copy "about"}}">
  <div class="dh"><b>{{.Page.Name}}</b><button class="x" id="close" type="button" aria-label="{{index .Copy "close"}}">✕</button></div>
  <div class="db">
    <p class="full">{{.Page.Description}}</p>
    {{if .Details}}<div class="line" style="margin:0"></div>
    <div><h2 style="font-size:18px;margin:0 0 16px">{{index .Copy "details"}}</h2>
    <div class="grid">{{range .Details}}<div><b>{{.Label}}</b><span>{{.Value}}</span>{{range .Notes}}<span>{{.}}</span>{{end}}</div>{{end}}</div></div>{{end}}
  </div>
</dialog>{{end}}
<script>
(function () {
  var COPY = {{.JSCopy}};
  if ("serviceWorker" in navigator) navigator.serviceWorker.register("sw.js", { scope: "./" });
  var installBtn = document.getElementById("install");
  var head = document.getElementById("head");
  var dev = document.getElementById("dev");
  var devText = dev.textContent;
  var actions = document.getElementById("actions");
  var reported = false;
  function reportInstalled() {
    if (reported) return;
    reported = true;
    if (navigator.sendBeacon) navigator.sendBeacon("installed");
    else fetch("installed", { method: "POST", keepalive: true });
  }
  var more = document.getElementById("more"), sheet = document.getElementById("sheet");
  if (more && sheet && sheet.showModal) {
    more.addEventListener("click", function () { sheet.showModal(); });
    document.getElementById("close").addEventListener("click", function () { sheet.close(); });
    sheet.addEventListener("click", function (e) { if (e.target === sheet) sheet.close(); });
  }
  var standalone = window.matchMedia("(display-mode: standalone)").matches || navigator.standalone === true;
  if (standalone) { location.replace("open"); return; }
  var ua = navigator.userAgent;
  var isIOS = /iPhone|iPad|iPod/.test(ua) || (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);
  if (isIOS) document.getElementById("ios").style.display = "block";
  var deferred = null;
  // Same sequence as the editor preview: the button collapses, a ring fills
  // around the icon with "Please wait…", then the page offers "Open".
  function showInstalled() {
    head.classList.remove("busy");
    actions.classList.remove("busy-actions");
    dev.textContent = devText;
    installBtn.hidden = true;
    document.getElementById("open").hidden = false;
    document.getElementById("done").style.display = "block";
  }
  function showInstalling() {
    head.classList.add("busy");
    actions.classList.add("busy-actions");
    dev.textContent = COPY.pleaseWait;
    setTimeout(showInstalled, 1800);
  }
  window.addEventListener("beforeinstallprompt", function (e) {
    e.preventDefault();
    deferred = e;
    installBtn.hidden = false;
  });
  installBtn.addEventListener("click", function () {
    if (!deferred) return;
    deferred.prompt();
    deferred.userChoice.then(function (choice) {
      if (choice.outcome === "accepted") { reportInstalled(); showInstalling(); }
      deferred = null;
    });
  });
  window.addEventListener("appinstalled", function () {
    reportInstalled();
    if (!head.classList.contains("busy")) showInstalled();
  });
})();
</script>
</body>
</html>
`))

type pwaDetail struct {
	Label, Value string
	Notes        []string
}

func renderPwaPage(p routingstore.PwaPage, test bool, lang string) []byte {
	short := p.ShortName
	if short == "" {
		short = p.Name
	}
	cp := pwaCopy[lang]
	var notes []string
	if p.ContainsAds {
		notes = append(notes, cp["ads"])
	}
	if p.InAppPurchases {
		notes = append(notes, cp["iap"])
	}
	var details []pwaDetail
	add := func(label, value string, n ...string) {
		if value != "" {
			details = append(details, pwaDetail{Label: label, Value: value, Notes: n})
		}
	}
	add(cp["dUpdated"], p.UpdatedDate)
	add(cp["dVersion"], p.Version)
	add(cp["dDeveloper"], p.Developer, notes...)
	add(cp["dCategory"], p.Category)
	add(cp["dInteractive"], p.InteractiveElements)
	// The editor's card flips light/dark from the PWA's background color.
	bg := cssColor(p.BackgroundColor, "#ffffff")
	scheme := map[bool][5]string{
		false: {"#0f172a", "#475569", "#334155", "#dadce0", "#f1f5f9"},
		true:  {"#f4f4f5", "#a1a1aa", "#d4d4d8", "#3f3f46", "#27272a"},
	}[isDarkColor(string(bg))]
	pageBg := "#ffffff"
	if isDarkColor(string(bg)) {
		pageBg = "#18181b"
	}
	jsCopy := map[string]string{"pleaseWait": cp["pleaseWait"]}
	var b strings.Builder
	_ = pwaPageTemplate.Execute(&b, map[string]any{
		"Lang": lang, "Copy": cp, "JSCopy": jsCopy, "Page": p, "Test": test, "Short": short,
		"Theme": cssColor(p.ThemeColor, "#00A173"), "Bg": template.CSS(pageBg),
		"Text": template.CSS(scheme[0]), "Muted": template.CSS(scheme[1]), "Subtle": template.CSS(scheme[2]),
		"Border": template.CSS(scheme[3]), "Chip": template.CSS(scheme[4]),
		"Notes": strings.Join(notes, " · "), "Details": details,
	})
	return []byte(b.String())
}

// isDarkColor is the editor preview's perceived-brightness check
// (ITU-R BT.601 weights) on a "#rrggbb" value; anything else reads as light.
func isDarkColor(hex string) bool {
	if len(hex) != 7 || hex[0] != '#' {
		return false
	}
	v, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		return false
	}
	r, g, b := (v>>16)&0xff, (v>>8)&0xff, v&0xff
	return (r*299+g*587+b*114)/1000 < 128
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
