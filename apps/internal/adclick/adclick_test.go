package adclick_test

import (
	"net/url"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/adclick"
)

// Audit UNI-2: one network click id per click, the first present in
// Params' order, and the network each belongs to.
func TestFromQuery(t *testing.T) {
	for _, c := range []struct {
		query, param, id, platform string
	}{
		{"gclid=Cj0K&yclid=73", adclick.GCLID, "Cj0K", adclick.Google},
		{"wbraid=W1&gbraid=G1", adclick.GBRAID, "G1", adclick.Google},
		{"wbraid=W1", adclick.WBRAID, "W1", adclick.Google},
		{"yclid=7364217983742390271&utm_source=yandex", adclick.YCLID, "7364217983742390271", adclick.Yandex},
		{"msclkid=1c5a", adclick.MSCLKID, "1c5a", adclick.Microsoft},
		{"gclid=&yclid=73", adclick.YCLID, "73", adclick.Yandex},
		{"fbclid=fb.1&ttclid=tt", "", "", ""},
	} {
		q, err := url.ParseQuery(c.query)
		if err != nil {
			t.Fatal(err)
		}
		param, id := adclick.FromQuery(q)
		if param != c.param || id != c.id || adclick.Platform(param) != c.platform {
			t.Errorf("%s: %q=%q (%q), want %q=%q (%q)", c.query, param, id, adclick.Platform(param), c.param, c.id, c.platform)
		}
	}
}
