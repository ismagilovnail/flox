package main

import (
	"net/http"
	"strconv"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/routing"
)

// params are the pass-through attribution parameters a tracking link
// carries (§42): utm_*, sub1..sub10, and external click IDs.
//
// §42's "unfilled FB subs" rule is implemented by simply not having any
// special case: whatever arrives is persisted, and whatever doesn't
// arrive stays an empty string. There is deliberately no placeholder, no
// "unknown", and no inference — an empty sub is recorded as empty, and
// event.Subs.SubCount() makes the share of subs-less traffic measurable
// downstream instead of silently miscounting attribution.
type params struct {
	utmSource   string
	utmMedium   string
	utmCampaign string
	utmContent  string
	utmTerm     string

	subs event.Subs

	externalClickID string
	fbClickID       string
	ttClickID       string

	referrer string
}

func parseParams(r *http.Request) params {
	q := r.URL.Query()

	p := params{
		utmSource:   q.Get("utm_source"),
		utmMedium:   q.Get("utm_medium"),
		utmCampaign: q.Get("utm_campaign"),
		utmContent:  q.Get("utm_content"),
		utmTerm:     q.Get("utm_term"),

		fbClickID: q.Get("fbclid"),
		ttClickID: q.Get("ttclid"),

		referrer: r.Referer(),
	}

	for i := range p.subs {
		p.subs[i] = q.Get("sub" + strconv.Itoa(i+1))
	}

	// Only what arrived as external_click_id. fbclid/ttclid already have
	// columns of their own, and copying them here stored each click id
	// twice — a third of a click's bytes (audit DATA-7). Readers take
	// external_click_id, else fbclid, else ttclid: sourceClickID here,
	// chstore.SourceClickIDExpr in ClickHouse.
	p.externalClickID = q.Get("external_click_id")

	return p
}

// applyTo merges the pass-through parameters into the classified
// attributes, so stream-set filters can match on them. These fields are
// raw request data, not classification output — which is exactly why
// internal/classifier does not produce them (documented there).
func (p params) applyTo(attrs routing.Attributes) {
	attrs[routing.FieldReferrer] = p.referrer
	attrs[routing.FieldUTMSource] = p.utmSource
	attrs[routing.FieldUTMMedium] = p.utmMedium
	attrs[routing.FieldUTMCampaign] = p.utmCampaign
	attrs[routing.FieldUTMContent] = p.utmContent
	attrs[routing.FieldUTMTerm] = p.utmTerm
	attrs[routing.FieldExternalClickID] = p.sourceClickID()

	subFields := [10]routing.FilterField{
		routing.FieldSub1, routing.FieldSub2, routing.FieldSub3, routing.FieldSub4, routing.FieldSub5,
		routing.FieldSub6, routing.FieldSub7, routing.FieldSub8, routing.FieldSub9, routing.FieldSub10,
	}
	for i, f := range subFields {
		attrs[f] = p.subs[i]
	}
}

// sourceClickID: the traffic source's click id as filters and postbacks
// see it — external_click_id, else fbclid, else ttclid.
func (p params) sourceClickID() string {
	return firstNonEmpty(p.externalClickID, p.fbClickID, p.ttClickID)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
