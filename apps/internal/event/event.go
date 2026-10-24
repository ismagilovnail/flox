// Package event is the authoritative, typed event model (§43) shared
// across tracker, worker, analytics, and pixels — one definition, so a
// pixel firing on CPA_ACCEPT and an analytics query counting CPA_ACCEPT
// can never disagree about the string.
//
// CLAUDE.md non-negotiable #2: the full ~20-type model exists from day
// one and is never truncated, and CPA statuses are distinct enum members,
// never collapsed into a single "conversion" type. Adding types later is a
// migration on live ClickHouse data, so every type is declared here,
// emitted or not.
package event

import "time"

type Type string

const (
	// TRAFFIC
	SourceClick  Type = "SOURCE_CLICK"  // click on the tracking link
	SourceFilter Type = "SOURCE_FILTER" // click blocked by campaign filters (bot/geo/device/…)

	// LANDING
	LandView         Type = "LAND_VIEW"
	LandClick        Type = "LAND_CLICK"
	PostlandingView  Type = "POSTLANDING_VIEW"
	PostlandingClick Type = "POSTLANDING_CLICK"

	// PWA
	PwaView    Type = "PWA_VIEW"
	PwaOpen    Type = "PWA_OPEN"
	PwaInstall Type = "PWA_INSTALL"
	IosInstall Type = "IOS_INSTALL"

	// PUSH
	NotificationRequest     Type = "NOTIFICATION_REQUEST"
	NotificationSubscribe   Type = "NOTIFICATION_SUBSCRIBE"
	NotificationDecline     Type = "NOTIFICATION_DECLINE"
	NotificationUnsubscribe Type = "NOTIFICATION_UNSUBSCRIBE"
	NotificationClick       Type = "NOTIFICATION_CLICK"

	// TELEGRAM
	TgJoin  Type = "TG_JOIN"
	TgStart Type = "TG_START"

	// CPA CONVERSIONS — status is an enum, NOT a single "conversion" type.
	CpaHold    Type = "CPA_HOLD"    // registration
	CpaAccept  Type = "CPA_ACCEPT"  // first deposit / FTD (key conversion)
	CpaRedep   Type = "CPA_REDEP"   // re-deposit (drives LTV)
	CpaDecline Type = "CPA_DECLINE" // rejected
	CpaTrash   Type = "CPA_TRASH"   // junk / duplicate
)

// All is every event type in the model, declaration-ordered. Used by
// validation and by anything that needs to enumerate the model (e.g. the
// ClickHouse schema) without re-listing it and
// risking drift.
var All = []Type{
	SourceClick, SourceFilter,
	LandView, LandClick, PostlandingView, PostlandingClick,
	PwaView, PwaOpen, PwaInstall, IosInstall,
	NotificationRequest, NotificationSubscribe, NotificationDecline, NotificationUnsubscribe, NotificationClick,
	TgJoin, TgStart,
	CpaHold, CpaAccept, CpaRedep, CpaDecline, CpaTrash,
}

var valid = func() map[Type]bool {
	m := make(map[Type]bool, len(All))
	for _, t := range All {
		m[t] = true
	}
	return m
}()

func (t Type) Valid() bool { return valid[t] }

// IsCPA reports whether t is one of the five CPA conversion statuses.
// Callers that need "was this a conversion" must ask this rather than
// string-matching a "conversion" type that deliberately does not exist.
func (t Type) IsCPA() bool {
	switch t {
	case CpaHold, CpaAccept, CpaRedep, CpaDecline, CpaTrash:
		return true
	default:
		return false
	}
}

// IsClick reports whether t is the tracker's own redirect-time decision
// (SOURCE_CLICK/SOURCE_FILTER) — §48's click_events table, as opposed to
// every other non-CPA event (tracking_events).
func (t Type) IsClick() bool {
	switch t {
	case SourceClick, SourceFilter:
		return true
	default:
		return false
	}
}

// Subs are the sub1..sub10 pass-through parameters (§42). A fixed-size
// array rather than a map: the set is fixed at exactly ten, and this keeps
// "present but empty" (the common Facebook case) distinct from "not a
// field at all" without any nil-map handling.
type Subs [10]string

// SubCount is how many of sub1..sub10 arrived non-empty. Feeds the
// "empty subs %" diagnostic §42 requires so buyers can *see* subs-less
// traffic instead of it silently miscounting attribution.
func (s Subs) SubCount() int {
	n := 0
	for _, v := range s {
		if v != "" {
			n++
		}
	}
	return n
}

// Event is one record in the pipeline (tracker → queue → worker →
// ClickHouse).
type Event struct {
	// ID identifies the event itself, stable from its creation through
	// every retry (tech audit D2): assigned by eventbuf.Writer.Enqueue
	// when empty, and for a conversion it is the postbacks row id. It is
	// the event_queue row id (a re-sent write is a no-op) and the
	// ClickHouse event_id the reconciliation repairs by.
	ID string `json:"id,omitempty"`

	Type           Type      `json:"type"`
	EventAt        time.Time `json:"eventAt"`
	OrganizationID string    `json:"organizationId"`
	CampaignID     string    `json:"campaignId"`

	// ClickID chains every event for one user journey together (§43).
	ClickID string `json:"clickId"`

	// Routing decision, for "why did this click go where it went".
	StreamSetID   string `json:"streamSetId,omitempty"`
	FlowID        string `json:"flowId,omitempty"`
	Destination   string `json:"destination,omitempty"`
	StickyApplied bool   `json:"stickyApplied,omitempty"`
	ConfigVersion int64  `json:"configVersion,omitempty"`
	// LandingID is the landing the visitor was shown on this journey —
	// set on SOURCE_CLICK/LAND_VIEW/LAND_CLICK by apps/tracker and
	// inherited by conversions via attribution. Empty when no landing
	// stage applied.
	LandingID string `json:"landingId,omitempty"`
	// OfferID is the offer the click was routed to (routing.RouteResult
	// .OfferID — empty unless the flow's offer was the destination
	// actually used), carried onto LAND_* events and, via attribution,
	// conversions.
	OfferID string `json:"offerId,omitempty"`
	// PwaID is the flow's PWA stage when the click's routed path includes
	// it (apps/tracker/pwa.go) — on SOURCE_CLICK, LAND_*, PWA_* and, via
	// attribution, conversions.
	PwaID string `json:"pwaId,omitempty"`
	// PostlandingID is the flow's postlanding stage when the click's routed
	// path includes it (apps/tracker/postlanding.go) — on SOURCE_CLICK,
	// LAND_*, PWA_*, POSTLANDING_* and, via attribution, conversions.
	PostlandingID string `json:"postlandingId,omitempty"`
	// TrafficSourceID is the traffic source of the click's campaign
	// (apps/tracker, from the campaign row at click time) — on every
	// click-derived event and, via attribution, conversions. Backs the
	// report builder's `source` dimension.
	TrafficSourceID string `json:"trafficSourceId,omitempty"`

	// Classified request attributes.
	Country        string `json:"country,omitempty"`
	Region         string `json:"region,omitempty"`
	City           string `json:"city,omitempty"`
	Device         string `json:"device,omitempty"`
	Platform       string `json:"platform,omitempty"`
	OS             string `json:"os,omitempty"`
	OSVersion      string `json:"osVersion,omitempty"`
	Browser        string `json:"browser,omitempty"`
	BrowserVersion string `json:"browserVersion,omitempty"`
	Language       string `json:"language,omitempty"`
	IsBot          bool   `json:"isBot,omitempty"`
	IsProxy        bool   `json:"isProxy,omitempty"`
	ASN            string `json:"asn,omitempty"`
	ConnectionType string `json:"connectionType,omitempty"`
	IP             string `json:"ip,omitempty"`
	UserAgent      string `json:"userAgent,omitempty"`
	Referrer       string `json:"referrer,omitempty"`
	// VisitorKey is the keyed pseudonym of IP + user agent unique clicks
	// count by (internal/visitorkey); click events only. It outlives the
	// raw IP and user agent, which expire (RETENTION_IP_DAYS).
	VisitorKey string `json:"visitorKey,omitempty"`
	// IPVisitorKey is the pseudonym of IP + user agent (visitorkey.Of),
	// kept beside the cookie-based VisitorKey so the two ways of counting
	// unique clicks can be compared on real traffic (audit DATA-5).
	IPVisitorKey string `json:"ipVisitorKey,omitempty"`

	// Pass-through attribution parameters (§42). Persisted exactly as
	// they arrived — missing means empty, never a fabricated placeholder.
	UTMSource   string `json:"utmSource,omitempty"`
	UTMMedium   string `json:"utmMedium,omitempty"`
	UTMCampaign string `json:"utmCampaign,omitempty"`
	UTMContent  string `json:"utmContent,omitempty"`
	UTMTerm     string `json:"utmTerm,omitempty"`
	Subs        Subs   `json:"subs"`

	// External click IDs. ExternalClickID is FLOX's own generic field;
	// FBClickID/TTClickID capture the network-specific parameters
	// (fbclid/ttclid) that arrive on the URL.
	ExternalClickID string `json:"externalClickId,omitempty"`
	FBClickID       string `json:"fbClickId,omitempty"`
	TTClickID       string `json:"ttClickId,omitempty"`
	// AdClickParam/AdClickID: the search or ad network's click id the
	// link carried — gclid, gbraid, wbraid, yclid or msclkid, and its value
	// (internal/adclick, audit UNI-2). Click events only.
	AdClickParam string `json:"adClickParam,omitempty"`
	AdClickID    string `json:"adClickId,omitempty"`

	// FilterReason is set on SOURCE_FILTER only — why the click was
	// blocked, so a buyer can tell bot-filtering from geo-filtering.
	FilterReason string `json:"filterReason,omitempty"`

	// NetworkID is, on a CPA_* event, the network that sent the postback;
	// on a click and the events after it, the network of the offer the
	// click was routed to (apps/tracker; empty for redirects and
	// fallbacks). Backs the `network` report dimension.
	NetworkID string `json:"networkId,omitempty"`

	// TelegramChannelID is, on TG_JOIN, the FLOX id of the channel joined
	// (telegram_channels.id). Backs the `telegram_channel` report
	// dimension; empty on every other event.
	TelegramChannelID string `json:"telegramChannelId,omitempty"`

	// CPA conversion fields (§45), set on CPA_* events only.

	// Revenue/Currency are exactly as the network reported them; USDValue
	// is normalized using the FX rate on EventAt's date, never the current
	// rate (§50-FX, CLAUDE.md #7). HasUSDValue distinguishes "converts to
	// 0 USD" from "no FX rate was on file for that currency/date" — a
	// missing rate is never silently treated as zero.
	Revenue     float64 `json:"revenue,omitempty"`
	Currency    string  `json:"currency,omitempty"`
	USDValue    float64 `json:"usdValue,omitempty"`
	HasUSDValue bool    `json:"hasUsdValue,omitempty"`

	// EventRef is the dedup key's third component (§45): the network's
	// transaction id for CPA_REDEP, empty for every other status even when
	// the network sent one. NetworkTxnID is stored regardless — the raw
	// value is kept even on statuses where it doesn't participate in the
	// dedup key.
	EventRef     string `json:"eventRef,omitempty"`
	NetworkTxnID string `json:"networkTxnId,omitempty"`

	// AttributionOutcome/AttributionMethod carry §44's attribution
	// decision (e.g. "attributed"/"click_id", or "unknown_click"/"") onto
	// the event so an unattributed conversion is stored, not discarded
	// (docs/attribution.md).
	AttributionOutcome string `json:"attributionOutcome,omitempty"`
	AttributionMethod  string `json:"attributionMethod,omitempty"`

	// TimeToConversion is the conversion timestamp minus the click's own,
	// in milliseconds. Can be negative (clock skew / replay) — reported,
	// never clamped.
	TimeToConversionMS int64 `json:"timeToConversionMs,omitempty"`
}
