package landing

import (
	"strings"
	"testing"
)

func TestRenderInjectsAtTopOfHead(t *testing.T) {
	src := `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><script>console.log(trackUrl)</script></head><body><a href="#">Go</a></body></html>`
	out := Render(src, "https://cdn.example/landings/o/l/v/", "https://trk.example/c/1?a=1&b=2")

	if !strings.HasPrefix(out, "<!DOCTYPE html>") {
		t.Fatal("doctype is no longer first")
	}
	head := strings.Index(out, "<head>")
	base := strings.Index(out, `<base href="https://cdn.example/landings/o/l/v/">`)
	runtime := strings.Index(out, "var trackUrl = ")
	own := strings.Index(out, "console.log(trackUrl)")
	if !(head >= 0 && head < base && base < runtime && runtime < own) {
		t.Fatalf("want <head> < <base> < runtime < page scripts, got %d %d %d %d:\n%s", head, base, runtime, own, out)
	}
	if !strings.Contains(out, `var trackUrl = "https://trk.example/c/1?a=1\u0026b=2";`) {
		t.Fatalf("trackUrl not JSON-encoded into the runtime:\n%s", out)
	}
	if !strings.Contains(out, "engine_onClickButton") {
		t.Fatal("engine_onClickButton missing")
	}
}

func TestRenderSubstitutesTrackURLMacro(t *testing.T) {
	out := Render(`<a href="{{ trackUrl }}">a</a><a href="{{trackUrl}}">b</a>`, "", `https://t.example/?x=1&y="2"`)
	want := `href="https://t.example/?x=1&amp;y=&#34;2&#34;"`
	if strings.Count(out, want) != 2 {
		t.Fatalf("macros not substituted HTML-escaped (want 2x %s):\n%s", want, out)
	}
	if strings.Contains(out, "{{") {
		t.Fatal("a macro survived")
	}
}

func TestRenderKeepsPageOwnBase(t *testing.T) {
	out := Render(`<html><head><base href="https://own.example/"></head></html>`, "https://cdn.example/x/", "#")
	if strings.Contains(out, "cdn.example") {
		t.Fatalf("overrode the page's own <base>:\n%s", out)
	}
}

func TestRenderWithoutHead(t *testing.T) {
	out := Render(`<html><body>x</body></html>`, "https://cdn.example/x/", "#")
	if !strings.HasPrefix(out, "<html><head><base ") || !strings.Contains(out, "</head><body>") {
		t.Fatalf("no <head> synthesized after <html>:\n%s", out)
	}

	out = Render("<!doctype html>\n<p>fragment</p>", "", "#")
	if !strings.HasPrefix(out, "<!doctype html><script>") {
		t.Fatalf("fragment: runtime not placed right after the doctype:\n%s", out)
	}

	out = Render(`<header>not a head</header>`, "", "#")
	if !strings.HasPrefix(out, "<script>") {
		t.Fatalf("<header> mistaken for <head>:\n%s", out)
	}
}

func TestRenderScriptBreakoutIsEscaped(t *testing.T) {
	out := Render("<head></head>", "", `</script><script>alert(1)</script>`)
	if strings.Contains(out, "<script>alert(1)") {
		t.Fatalf("trackUrl broke out of the runtime script:\n%s", out)
	}
}
