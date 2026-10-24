// Package adclick is the search and ad networks' own click ids a tracking
// link can carry (audit UNI-2): Google's gclid (gbraid/wbraid on iOS
// app-to-web clicks), Yandex's yclid, Microsoft's msclkid. The networks
// append them to the link themselves when auto-tagging is on; FLOX keeps
// the one that arrived, so a conversion can later be reported back to the
// network by it (offline conversions).
//
// A click keeps one: a network sends one of its own, and a link carrying
// two networks' ids is a copied URL, not two clicks. Params is the order
// the first present one is taken in.
//
// fbclid/ttclid stay where they were (event.Event FBClickID/TTClickID):
// pixels match by them.
package adclick

import "net/url"

const (
	GCLID   = "gclid"
	GBRAID  = "gbraid"
	WBRAID  = "wbraid"
	YCLID   = "yclid"
	MSCLKID = "msclkid"
)

// Params are the query parameters read, in the order the first present one
// is taken in.
var Params = []string{GCLID, GBRAID, WBRAID, YCLID, MSCLKID}

// Platforms a param belongs to — the routing filter's and the click log's
// value (routing.FieldAdPlatform).
const (
	Google    = "google"
	Yandex    = "yandex"
	Microsoft = "microsoft"
)

// FromQuery is the first of Params present in q, and its value; "", "" when
// none is.
func FromQuery(q url.Values) (param, id string) {
	for _, p := range Params {
		if v := q.Get(p); v != "" {
			return p, v
		}
	}
	return "", ""
}

// Platform is the network param belongs to; "" for "" or an unknown one.
func Platform(param string) string {
	switch param {
	case GCLID, GBRAID, WBRAID:
		return Google
	case YCLID:
		return Yandex
	case MSCLKID:
		return Microsoft
	}
	return ""
}
