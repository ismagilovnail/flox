package main

import (
	_ "embed"
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
//	GET  /pwa/{token}/open                 installed app's start_url → [IOS_INSTALL] PWA_OPEN → destination
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
	h.registerPush(r)
	h.registerPwaExternal(r)
	h.registerLandingPwa(r)
}

// decidePwa is the flow's PWA stage for a visitor on os (pwa_external.go):
// an internal page, an external PWA, or an app link; nothing when the
// stage is off, its PWA paused (an ios_app stage then keeps only its app
// link), or click links cannot be signed.
func (h *Handler) decidePwa(loaded routingstore.CampaignRouting, flowID, os string) pwaStep {
	if h.links == nil || flowID == "" {
		return pwaStep{}
	}
	stage, ok := loaded.FlowPwas[flowID]
	if !ok {
		return pwaStep{}
	}
	if stage.Status == "paused" {
		if stage.Type != pwaTypeIOSApp {
			return pwaStep{}
		}
		stage.PwaID = ""
	}
	return choosePwaStep(stage, os)
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
	// Only on the host the link was issued for: the tracker serves every
	// organization's domains (tech audit M4).
	if err != nil || c.PwaID == "" || !c.OnHost(r.Host) {
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
		h.continueOnward(w, r, c, func() event.Event { return h.claimsEvent(r, c) })
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
	lang := pageLang(r)
	var comments []routingstore.PwaComment
	if c.Test {
		// Hand-entered comments are part of the constructor's test view
		// only; a real visitor's page never carries them.
		var err error
		if comments, err = h.store.LoadPwaComments(r.Context(), c.OrganizationID, c.PwaID); err != nil {
			h.logger.Error("loading pwa comments", "error", err, "pwa_id", c.PwaID)
		}
	}
	body, err := renderPwaPage(page, c.Test, lang, commentsForLang(comments, lang))
	if err != nil {
		// A broken template must not strand the visitor on half a page.
		h.logger.Error("rendering pwa page", "error", err, "pwa_id", c.PwaID)
		outcome = outcomeRedirected
		h.continueOnward(w, r, c, func() event.Event { return h.claimsEvent(r, c) })
		return
	}
	writeHTML(w, body)
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
// It also shows push notifications and opens their click-through URL
// (push.go, docs/push.md).
func (h *Handler) pwaServiceWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(`self.addEventListener("install", function () { self.skipWaiting(); });
self.addEventListener("activate", function (e) { e.waitUntil(self.clients.claim()); });
self.addEventListener("fetch", function (e) { e.respondWith(fetch(e.request)); });
self.addEventListener("push", function (e) {
  var d = {};
  try { d = e.data ? e.data.json() : {}; } catch (err) {}
  e.waitUntil(self.registration.showNotification(d.title || "", {
    body: d.body || "", icon: d.icon || undefined, image: d.image || undefined, data: { url: d.url || "" }
  }));
});
self.addEventListener("notificationclick", function (e) {
  e.notification.close();
  var url = e.notification.data && e.notification.data.url;
  if (url) e.waitUntil(self.clients.openWindow(url));
});
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
// on the original click_id and continues to the destination. On iOS the
// first launch also records IOS_INSTALL (iosFirstLaunch).
func (h *Handler) pwaOpen(w http.ResponseWriter, r *http.Request) {
	c, ok := h.pwaClaims(r, true)
	if !ok {
		http.NotFound(w, r)
		return
	}
	open := h.claimsEvent(r, c)
	if !c.Test {
		if h.iosFirstLaunch(w, r, open.OS) {
			install := open
			install.Type = event.IosInstall
			h.events.Enqueue(install)
		}
		open.Type = event.PwaOpen
		h.events.Enqueue(open)
	}
	// A PWA that asks for notifications shows its launch page first; it
	// continues through pwaGo (push.go).
	if page, ok := h.loadPwa(r, c); ok && h.pushReady(c, page.PushEnabled) {
		body, err := renderLaunch(page.Name, page.IconURL, page.ThemeColor, h.vapidPublicKey, pageLang(r))
		if err == nil {
			writeHTML(w, body)
			return
		}
		h.logger.Error("rendering launch page", "error", err, "pwa_id", c.PwaID)
	}
	h.continueOnward(w, r, c, func() event.Event { return open })
}

// iosInstalledCookie marks an installed iOS app that has launched. Safari
// on iOS never fires appinstalled, so the install page's beacon
// (PWA_INSTALL) cannot see an iOS install; its first launch from the Home
// Screen is the first sign of it. A Home Screen app has its own cookie
// store, which starts without this cookie: no cookie on start_url means
// this installation's first launch (docs/pwa-serving.md). Scoped to the
// token's path, so Safari's own visits to the install page never set it.
const iosInstalledCookie = "sf_pwa_inst"

// iosInstalledMaxAge is the longest browsers keep a cookie (400 days).
const iosInstalledMaxAge = 400 * 24 * 60 * 60

// iosFirstLaunch reports whether this start_url request is an iOS app's
// first launch, marking it launched. Not iOS (os from the classifier,
// iPhone and iPad), or already marked: false.
func (h *Handler) iosFirstLaunch(w http.ResponseWriter, r *http.Request, os string) bool {
	return iosFirstLaunchAt(w, r, os, pwaPathPrefix+chi.URLParam(r, "token")+"/")
}

// iosFirstLaunchAt is iosFirstLaunch with the cookie scoped to path (the
// installed app's own path: /pwa/<token>/ or /lpwa/<token>/).
func iosFirstLaunchAt(w http.ResponseWriter, r *http.Request, os, path string) bool {
	if os != "ios" {
		return false
	}
	if _, err := r.Cookie(iosInstalledCookie); err == nil {
		return false
	}
	http.SetCookie(w, &http.Cookie{
		Name:     iosInstalledCookie,
		Value:    "1",
		Path:     path,
		MaxAge:   iosInstalledMaxAge,
		HttpOnly: true,
		Secure:   strings.HasPrefix(requestOrigin(r), "https:"),
		SameSite: http.SameSiteLaxMode,
	})
	return true
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
	h.continueOnward(w, r, c, func() event.Event { return h.claimsEvent(r, c) })
}

// recordClaimsEvent records t on the token's click_id with this request's
// geo/device — unless the token is a constructor test link.
func (h *Handler) recordClaimsEvent(r *http.Request, t event.Type, c landinglink.Claims) {
	if c.Test {
		return
	}
	ev := h.claimsEvent(r, c)
	ev.Type = t
	h.events.Enqueue(ev)
}

// claimsEvent is the event of this request on the token's journey: the
// token's routing/attribution context, this request's geo/device. Its Type
// is the caller's to set.
func (h *Handler) claimsEvent(r *http.Request, c landinglink.Claims) event.Event {
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
	return landClickEvent(c, attrs, ip, r)
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

// pwaPageTemplate is the install page every visitor and the constructor's
// test link get: the editor preview card laid out one to one (store-style
// header, rating and star distribution, downloads, size, screenshots, About
// and details sheet, data safety, ratings and reviews, What's new, footer,
// bottom nav) with its light/dark scheme and accent. The numbers are exactly
// what the operator typed into the constructor form.
//
//go:embed templates/pwa.html
var pwaPageSource string

var pwaPageTemplate = template.Must(template.New("pwa").Funcs(template.FuncMap{
	"stars": func() []int { return []int{1, 2, 3, 4, 5} },
	// starText draws a comment's own 1..5 rating as filled/empty stars.
	"starText": func(n int) string {
		if n < 0 || n > 5 {
			return ""
		}
		return strings.Repeat("★", n) + strings.Repeat("☆", 5-n)
	},
	// fill is how much of star i (1..5) a rating covers, as a percentage.
	"fill": func(rating float64, i int) int {
		v := rating - float64(i-1)
		switch {
		case v >= 1:
			return 100
		case v <= 0:
			return 0
		}
		return int(v * 100)
	},
}).Parse(pwaPageSource))

var pwaStoreCopy = map[string]map[string]string{
	"ru": {
		"adFree": "Без рекламы и платного контента", "noData": "Пока нет оценок", "reviews": "отзывов",
		"downloadsLabel": "Скачиваний", "size": "Размер", "share": "Поделиться", "wishlist": "В список желаний",
		"availableOnDevices": "Это приложение доступно на всех ваших устройствах",
		"dataSafetyTitle":    "Безопасность данных",
		"dataSafetyText":     "Безопасность начинается с понимания того, как разработчики собирают и передают ваши данные. Меры обеспечения конфиденциальности и безопасности данных могут зависеть от того, как вы используете приложение, а также от вашего региона и возраста. Эта информация предоставлена разработчиком и может меняться.",
		"devReply":           "Ответ разработчика", "reviewsTitle": "Оценки и отзывы", "reviewsVerified": "Оценки и отзывы проверены",
		"footerStore": "Приложения", "footerGiftCards": "Подарочные карты", "footerRedeemCode": "Использовать код",
		"footerFamilyLibrary": "Семейная библиотека", "footerKidsFamilyTitle": "Детям и семье",
		"footerParentGuide": "Руководство для родителей", "footerTerms": "Условия использования",
		"footerPrivacy": "Конфиденциальность", "footerDevelopers": "Разработчики",
		"navGames": "Игры", "navApps": "Приложения", "navBooks": "Книги", "navKids": "Детям",
		"containsAds": "Есть реклама", "inAppPurchases": "Покупки в приложении",
		"version": "Текущая версия", "category": "Категория", "ageRating": "Возраст",
		"interactiveElements": "Интерактивные элементы", "developer": "Разработчик",
		"dSize": "Размер", "dDownloads": "Скачивания", "dAge": "Возраст", "placeholderDeveloper": "Разработчик",
	},
	"en": {
		"adFree": "No ads or paid content", "noData": "No rating yet", "reviews": "reviews",
		"downloadsLabel": "Downloads", "size": "Size", "share": "Share", "wishlist": "Wishlist",
		"availableOnDevices": "This app is available on all your devices",
		"dataSafetyTitle":    "Data safety",
		"dataSafetyText":     "Safety starts with understanding how developers collect and share your data. Data privacy and security practices may vary based on your use, region, and age. The developer provided this information and may update it over time.",
		"devReply":           "Developer response", "reviewsTitle": "Ratings and reviews", "reviewsVerified": "Ratings and reviews are verified",
		"footerStore": "Apps", "footerGiftCards": "Gift cards", "footerRedeemCode": "Redeem code",
		"footerFamilyLibrary": "Family library", "footerKidsFamilyTitle": "Kids & family",
		"footerParentGuide": "Parent guide", "footerTerms": "Terms of Service",
		"footerPrivacy": "Privacy", "footerDevelopers": "Developers",
		"navGames": "Games", "navApps": "Apps", "navBooks": "Books", "navKids": "Kids",
		"containsAds": "Contains ads", "inAppPurchases": "In-app purchases",
		"version": "Current version", "category": "Category", "ageRating": "Age",
		"interactiveElements": "Interactive elements", "developer": "Developer",
		"dSize": "Size", "dDownloads": "Downloads", "dAge": "Age", "placeholderDeveloper": "Developer",
	},
}

type pwaDetail struct {
	Label, Value string
	Notes        []string
}

// commentsForLang picks the comment set shown to a page language: the
// entries whose language matches, else the first set entered.
func commentsForLang(all []routingstore.PwaComment, lang string) []routingstore.PwaComment {
	var out []routingstore.PwaComment
	for _, c := range all {
		if c.Lang == lang {
			out = append(out, c)
		}
	}
	if len(out) > 0 || len(all) == 0 {
		return out
	}
	for _, c := range all {
		if c.Geo == all[0].Geo && c.Lang == all[0].Lang {
			out = append(out, c)
		}
	}
	return out
}

// applyCommentStats replaces the typed rating, count and star shares with
// the ones the shown comments add up to.
func applyCommentStats(p *routingstore.PwaPage, comments []routingstore.PwaComment) {
	var counts [6]int
	sum := 0
	for _, c := range comments {
		counts[c.Rating]++
		sum += c.Rating
	}
	n := float64(len(comments))
	round1 := func(v float64) float64 { return float64(int(v*10+0.5)) / 10 }
	p.Rating = round1(float64(sum) / n)
	p.RatingCount = len(comments)
	pct := func(i int) float64 { return round1(float64(counts[i]) / n * 100) }
	p.RatingStars5, p.RatingStars4, p.RatingStars3, p.RatingStars2, p.RatingStars1 = pct(5), pct(4), pct(3), pct(2), pct(1)
}

func renderPwaPage(p routingstore.PwaPage, test bool, lang string, comments []routingstore.PwaComment) ([]byte, error) {
	if len(comments) > 0 {
		applyCommentStats(&p, comments)
	}
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
	// pwaCopy is shared between requests: extend a clone, never the map.
	merged := make(map[string]string, len(cp)+len(pwaStoreCopy[lang]))
	for k, v := range cp {
		merged[k] = v
	}
	for k, v := range pwaStoreCopy[lang] {
		if _, ok := merged[k]; !ok {
			merged[k] = v
		}
	}
	cp = merged
	add(cp["dUpdated"], p.UpdatedDate)
	if p.SizeMB > 0 {
		add(cp["dSize"], strconv.FormatFloat(p.SizeMB, 'f', -1, 64)+" MB")
	}
	add(cp["dDownloads"], p.DownloadsLabel)
	add(cp["dVersion"], p.Version)
	add(cp["dAge"], p.AgeRating)
	add(cp["dDeveloper"], p.Developer, notes...)
	add(cp["dCategory"], p.Category)
	add(cp["dInteractive"], p.InteractiveElements)
	// The editor's card flips light/dark from the PWA's background color.
	bg := cssColor(p.BackgroundColor, "#ffffff")
	schemes := map[bool][5]string{
		false: {"#0f172a", "#475569", "#334155", "#dadce0", "#f1f5f9"},
		true:  {"#f4f4f5", "#a1a1aa", "#d4d4d8", "#3f3f46", "#27272a"},
	}
	dark := isDarkColor(string(bg))
	if p.ThemeAuto {
		// Auto: light is the base palette, dark comes from the visitor's
		// prefers-color-scheme (see the template's media query).
		dark = false
	}
	scheme := schemes[dark]
	pageBg := "#ffffff"
	if dark {
		pageBg = "#18181b"
	}
	darkScheme := schemes[true]
	jsCopy := map[string]string{"pleaseWait": cp["pleaseWait"]}
	var b strings.Builder
	err := pwaPageTemplate.Execute(&b, map[string]any{
		"Lang": lang, "Copy": cp, "JSCopy": jsCopy, "Page": p, "Comments": comments, "Test": test, "Short": short,
		"Theme": cssColor(p.ThemeColor, "#00A173"), "Bg": template.CSS(pageBg),
		"Text": template.CSS(scheme[0]), "Muted": template.CSS(scheme[1]), "Subtle": template.CSS(scheme[2]),
		"Border": template.CSS(scheme[3]), "Chip": template.CSS(scheme[4]),
		"Notes": strings.Join(notes, " · "), "Details": details,
		"Auto": p.ThemeAuto, "DarkBg": template.CSS("#18181b"),
		"DarkText": template.CSS(darkScheme[0]), "DarkMuted": template.CSS(darkScheme[1]), "DarkSubtle": template.CSS(darkScheme[2]),
		"DarkBorder": template.CSS(darkScheme[3]), "DarkChip": template.CSS(darkScheme[4]),
	})
	if err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
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
