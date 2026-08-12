// Package event is the authoritative, typed event model (§43) shared
// across tracker, worker, analytics, and pixels — one definition, so a
// pixel firing on CPA_ACCEPT and an analytics query counting CPA_ACCEPT
// can never disagree about the string.
//
// CLAUDE.md non-negotiable #2: the full ~20-type model exists from day
// one and is never truncated, and CPA statuses are distinct enum members,
// never collapsed into a single "conversion" type. Adding types later is a
// migration on live ClickHouse data, so they are all declared now even
// though only SOURCE_CLICK and SOURCE_FILTER are actually emitted in
// Phase 21.
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
// ClickHouse enum definition in a later phase) without re-listing it and
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
// ClickHouse). Field set covers what Phase 21 can actually populate;
// later phases extend it rather than redefining it.
type Event struct {
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

	// FilterReason is set on SOURCE_FILTER only — why the click was
	// blocked, so a buyer can tell bot-filtering from geo-filtering.
	FilterReason string `json:"filterReason,omitempty"`
}
