package landing

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"
)

// PreviewTrackURL is what trackUrl resolves to in the editor's preview: a
// click does nothing, so the preview stays put. Not "#" — with the
// injected <base href>, "#" resolves to the bundle's asset URL and a
// click would navigate the preview frame away to it.
const PreviewTrackURL = "javascript:void(0)"

var (
	trackURLMacro = regexp.MustCompile(`\{\{\s*trackUrl\s*\}\}`)
	doctypeTag    = regexp.MustCompile(`(?i)^\s*<!doctype[^>]*>`)
	htmlOpenTag   = regexp.MustCompile(`(?i)<html(?:\s[^>]*)?>`)
	headOpenTag   = regexp.MustCompile(`(?i)<head(?:\s[^>]*)?>`)
	baseTag       = regexp.MustCompile(`(?i)<base[\s>]`)
)

// runtimeScript is the platform script every internal landing gets
// (docs/landings.md, "Injected runtime"), matching the reference
// product's contract: a global `trackUrl`, a global
// `engine_onClickButton()` that moves the visitor on through the flow,
// and every <a> bound to both — including anchors a framework renders
// later, via one delegated capture-phase listener. `data-flox-skip` on an
// anchor opts it out (e.g. a privacy-policy link).
const runtimeScript = `<script>
var trackUrl = %s;
window.engine_onClickButton = function (e) {
  if (e && typeof e.preventDefault === "function") e.preventDefault();
  window.location.href = trackUrl;
};
(function () {
  function bind() {
    var links = document.getElementsByTagName("a");
    for (var i = 0; i < links.length; i++) {
      if (!links[i].hasAttribute("data-flox-skip")) links[i].setAttribute("href", trackUrl);
    }
  }
  document.addEventListener("click", function (e) {
    var a = e.target && e.target.closest ? e.target.closest("a") : null;
    if (a && !a.hasAttribute("data-flox-skip")) window.engine_onClickButton(e);
  }, true);
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", bind);
  else bind();
})();
</script>`

// Render turns a landing's stored entry-file source into the page a
// browser is served: `{{ trackUrl }}` macros substituted (HTML-escaped,
// as Twig's autoescape would), a <base href> pointing relative asset
// paths at the bundle in object storage (unless the page sets its own),
// and the runtime script — both injected at the very top of <head> so
// trackUrl exists before any of the page's own scripts run.
//
// Pure function: the editor preview (trackURL = PreviewTrackURL) and the
// tracker serving real visitors share it, so what the operator previews
// is exactly what visitors get.
func Render(source, assetBaseURL, trackURL string) string {
	out := trackURLMacro.ReplaceAllLiteralString(source, html.EscapeString(trackURL))

	encoded, _ := json.Marshal(trackURL) // json escapes <, >, & — safe inside <script>
	inject := strings.Replace(runtimeScript, "%s", string(encoded), 1)
	if assetBaseURL != "" && !baseTag.MatchString(out) {
		inject = `<base href="` + html.EscapeString(assetBaseURL) + `">` + inject
	}

	if loc := headOpenTag.FindStringIndex(out); loc != nil {
		return out[:loc[1]] + inject + out[loc[1]:]
	}
	if loc := htmlOpenTag.FindStringIndex(out); loc != nil {
		return out[:loc[1]] + "<head>" + inject + "</head>" + out[loc[1]:]
	}
	// A bare fragment: keep a leading doctype first, or the browser drops
	// into quirks mode.
	if loc := doctypeTag.FindStringIndex(out); loc != nil {
		return out[:loc[1]] + inject + out[loc[1]:]
	}
	return inject + out
}
