/* FLOX conversion script for a site without a server (audit UNI-1 part 2;
 * docs/networks-offers.md "Own site without a server").
 *
 * On every page: <script src="https://TRACKER/flox.js" data-source="NETWORK_ID" async></script>
 *   keeps the click id from the URL (?click_id=…) in a first-party cookie.
 * On the thank-you page, add data-event="purchase" (or "lead") and, if
 * known, data-revenue, data-currency, data-order — or call
 *   flox("purchase", { revenue: 1990, currency: "RUB", order: "A-1001" }).
 * A page can report only a lead or a purchase, once per click. */
(function () {
  var s = document.currentScript;
  if (!s) return;
  var KEY = "flox_click_id";
  var source = s.getAttribute("data-source") || "";
  var name = s.getAttribute("data-param") || "click_id";
  var base = s.src.replace(/\/flox\.js(\?.*)?$/, "");

  function fromURL() {
    var m = new RegExp("[?&]" + name + "=([^&#]*)").exec(window.location.search);
    return m ? decodeURIComponent(m[1].replace(/\+/g, " ")) : "";
  }
  function keep(id) {
    document.cookie = KEY + "=" + encodeURIComponent(id) + "; path=/; max-age=" + 90 * 86400 + "; SameSite=Lax";
    try { window.localStorage.setItem(KEY, id); } catch (e) { /* storage blocked */ }
  }
  function kept() {
    var m = new RegExp("(?:^|;\\s*)" + KEY + "=([^;]*)").exec(document.cookie);
    if (m) return decodeURIComponent(m[1]);
    try { return window.localStorage.getItem(KEY) || ""; } catch (e) { return ""; }
  }

  var id = fromURL();
  if (id) keep(id);

  function send(status, o) {
    var click = kept();
    if (!source || !click || !status) return false;
    o = o || {};
    var q = "click_id=" + encodeURIComponent(click) + "&status=" + encodeURIComponent(status);
    if (o.revenue !== undefined && o.revenue !== null && o.revenue !== "") q += "&revenue=" + encodeURIComponent(o.revenue);
    if (o.currency) q += "&currency=" + encodeURIComponent(o.currency);
    if (o.order) q += "&txn_id=" + encodeURIComponent(o.order);
    var url = base + "/c/" + encodeURIComponent(source) + "?" + q;
    if (navigator.sendBeacon && navigator.sendBeacon(url)) return true;
    new Image().src = url;
    return true;
  }

  window.flox = send;
  var ev = s.getAttribute("data-event");
  if (ev) send(ev, { revenue: s.getAttribute("data-revenue"), currency: s.getAttribute("data-currency"), order: s.getAttribute("data-order") });
})();
