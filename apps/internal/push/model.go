// Package push is Web Push for installed PWAs (docs/push.md; §17 "Push",
// §43 NOTIFICATION_*): the subscriptions an installed app collects from its
// visitors (apps/tracker stores them), and broadcasts an operator sends to
// a segment of them now (apps/api queues them; internal/pushsend in
// apps/worker sends).
package push

import (
	"encoding/base64"
	"net/url"
	"strings"
	"time"
)

// Subscription is one browser's subscription to one PWA, bound to the
// click the app was installed from.
type Subscription struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	PwaID          string `json:"pwaId"`
	CampaignID     string `json:"campaignId"`
	ClickID        string `json:"clickId"`
	// PushService is the endpoint's host — which push service holds it.
	// The endpoint itself is a capability (anyone holding it with the keys
	// can message the visitor) and never leaves the server.
	PushService    string     `json:"pushService"`
	Country        string     `json:"country"`
	Device         string     `json:"device"`
	Language       string     `json:"language"`
	Status         string     `json:"status"`
	UnsubscribedAt *time.Time `json:"unsubscribedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
}

// NewSubscription is what apps/tracker stores when an installed app
// subscribes.
type NewSubscription struct {
	OrganizationID string
	PwaID          string
	CampaignID     string
	ClickID        string
	Endpoint       string
	P256dh, Auth   string
	// LinkToken is the click's signed link; Context its claims as JSON;
	// Origin the tracker scheme+host the app subscribed on.
	LinkToken string
	Context   []byte
	Origin    string
	Country   string
	Device    string
	Language  string
}

const (
	StatusActive       = "active"
	StatusUnsubscribed = "unsubscribed"
)

// Target is where a notification's click leads.
type Target string

const (
	// TargetDestination: the destination the subscriber's click was routed
	// to — the funnel continues on the same click.
	TargetDestination Target = "destination"
	// TargetURL: the broadcast's own URL, the click's macros resolved.
	TargetURL Target = "url"
)

// Filters select a broadcast's audience among the organization's active
// subscriptions. An empty list does not filter.
type Filters struct {
	PwaIDs      []string `json:"pwaIds"`
	CampaignIDs []string `json:"campaignIds"`
	Countries   []string `json:"countries"`
}

// Broadcast is a message sent now to the subscribers its filters match.
// The counters follow the deliveries: Delivered (the push service accepted
// it), Failed (could not be sent), Unsubscribed (the push service said the
// subscription is gone — counted in Failed too), Opened (clicked).
type Broadcast struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	IconURL      string    `json:"iconUrl"`
	ImageURL     string    `json:"imageUrl"`
	Target       Target    `json:"target"`
	URL          string    `json:"url"`
	Filters      Filters   `json:"filters"`
	Total        int       `json:"total"`
	Delivered    int       `json:"delivered"`
	Failed       int       `json:"failed"`
	Unsubscribed int       `json:"unsubscribed"`
	Opened       int       `json:"opened"`
	CreatedAt    time.Time `json:"createdAt"`
}

type CreateBroadcastInput struct {
	Name     string
	Title    string
	Body     string
	IconURL  string
	ImageURL string
	Target   Target
	URL      string
	Filters  Filters
}

// Summary is the subscriber overview: totals and the active subscribers
// by PWA, campaign and country.
type Summary struct {
	Active       int          `json:"active"`
	Unsubscribed int          `json:"unsubscribed"`
	ByPwa        []GroupCount `json:"byPwa"`
	ByCampaign   []GroupCount `json:"byCampaign"`
	ByCountry    []GroupCount `json:"byCountry"`
}

type GroupCount struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// Limits a notification has to fit in: a push message carries at most
// about 4 KB, and a notification shows little more than this anyway.
const (
	MaxTitle = 80
	MaxBody  = 240
	MaxURL   = 1024
)

// pushServiceHosts are the push services a subscription endpoint may point
// at: the worker posts to the endpoint, which comes from the browser, so
// an arbitrary one would make the worker call anywhere (§54). These cover
// Chrome/Edge/Opera/Samsung (FCM), Firefox, Safari and Windows.
var pushServiceHosts = []string{
	"fcm.googleapis.com",
	"android.googleapis.com",
	"updates.push.services.mozilla.com",
	"web.push.apple.com",
}

var pushServiceSuffixes = []string{
	".push.services.mozilla.com",
	".push.apple.com",
	".notify.windows.com",
}

// ValidEndpoint reports whether endpoint is an https URL on a known push
// service.
func ValidEndpoint(endpoint string) bool {
	if len(endpoint) > 2048 {
		return false
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, h := range pushServiceHosts {
		if host == h {
			return true
		}
	}
	for _, s := range pushServiceSuffixes {
		if strings.HasSuffix(host, s) {
			return true
		}
	}
	return false
}

// ValidKeys reports whether p256dh and auth are a subscription's keys: an
// uncompressed P-256 point (65 bytes) and a 16-byte secret, base64url.
func ValidKeys(p256dh, auth string) bool {
	return decodedLen(p256dh) == 65 && decodedLen(auth) == 16
}

func decodedLen(s string) int {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
	if err != nil {
		return -1
	}
	return len(b)
}

// serviceHost is what Subscription.PushService shows.
func serviceHost(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
