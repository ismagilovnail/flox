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

// Broadcast statuses. A scheduled broadcast has no deliveries until the
// worker sends it at SendAt (SendDue); the audience is fixed then. A
// triggered one is active (the worker sends it to each subscriber as they
// become due, SendTriggered) or paused.
const (
	BroadcastScheduled = "scheduled"
	BroadcastSent      = "sent"
	BroadcastCanceled  = "canceled"
	BroadcastActive    = "active"
	BroadcastPaused    = "paused"
	// BroadcastFinished: a recurring template past its end date.
	BroadcastFinished = "finished"
)

// Broadcast kinds: once (now or scheduled), triggered, or recurring — a
// template whose every send is a once broadcast with ParentID set to it.
const (
	KindOnce      = "once"
	KindTriggered = "triggered"
	KindRecurring = "recurring"
)

// Recurrence makes a broadcast go out on Weekdays (ISO: 1 = Monday … 7) at
// Time ("HH:MM") in Timezone — the organization's when it was created —
// until Until (a "YYYY-MM-DD" date in that zone, inclusive), or for good
// when Until is empty. Each send fixes its own audience then.
type Recurrence struct {
	Weekdays []int  `json:"weekdays"`
	Time     string `json:"time"`
	Timezone string `json:"timezone"`
	Until    string `json:"until"`
}

// RecurringLateness: a slot the worker reaches later than this (it was
// down) is skipped, not sent late — a "19:00" push at 3 a.m. is worse
// than none.
const RecurringLateness = time.Hour

// Trigger condition on the subscriber's click at send time.
const (
	ConditionNone    = "none"
	ConditionWith    = "with"    // has a conversion of one of Statuses
	ConditionWithout = "without" // has none of Statuses
)

// ConversionStatuses are the statuses a trigger condition can name (§43).
var ConversionStatuses = []string{"CPA_HOLD", "CPA_ACCEPT", "CPA_REDEP", "CPA_DECLINE", "CPA_TRASH"}

// Trigger makes a broadcast go to each subscriber DelayMinutes after they
// subscribed — only subscriptions made after the trigger was created, only
// within TriggerWindow of that moment — if Condition holds then.
type Trigger struct {
	DelayMinutes int      `json:"delayMinutes"`
	Condition    string   `json:"condition"`
	Statuses     []string `json:"statuses"`
}

// Trigger bounds. TriggerWindow: a subscriber is sent to only within this
// long of their due time, so resuming a paused trigger (or a worker
// outage) does not send day-old "1 hour after" messages; it also keeps the
// once-per-subscriber check within the delivery rows' retention.
const (
	MinTriggerDelay = 5
	MaxTriggerDelay = 30 * 24 * 60
	TriggerWindow   = 24 * time.Hour
)

// Broadcast is a message sent to the subscribers its filters match, now or
// at SendAt. The counters follow the deliveries: Delivered (the push
// service accepted it), Failed (could not be sent), Unsubscribed (the push
// service said the subscription is gone — counted in Failed too), Opened
// (clicked).
type Broadcast struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Title        string  `json:"title"`
	Body         string  `json:"body"`
	IconURL      string  `json:"iconUrl"`
	ImageURL     string  `json:"imageUrl"`
	Target       Target  `json:"target"`
	URL          string  `json:"url"`
	Filters      Filters `json:"filters"`
	Total        int     `json:"total"`
	Delivered    int     `json:"delivered"`
	Failed       int     `json:"failed"`
	Unsubscribed int     `json:"unsubscribed"`
	Opened       int     `json:"opened"`
	Kind         string  `json:"kind"`
	Status       string  `json:"status"`
	// Trigger is set for a triggered broadcast, whose Total grows as it
	// sends.
	Trigger *Trigger `json:"trigger"`
	// Recurrence is set for a recurring template; NextSendAt is its next
	// slot (null once finished), Occurrences how many sends it made — its
	// counters are theirs summed. ParentID is set on each of those sends.
	Recurrence  *Recurrence `json:"recurrence"`
	NextSendAt  *time.Time  `json:"nextSendAt"`
	Occurrences int         `json:"occurrences"`
	ParentID    *string     `json:"parentId"`
	SendAt      *time.Time  `json:"sendAt"`
	SentAt      *time.Time  `json:"sentAt"`
	CreatedAt   time.Time   `json:"createdAt"`
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
	// SendAt schedules the broadcast; nil sends it now.
	SendAt *time.Time
	// Trigger makes it a triggered broadcast (not with SendAt).
	Trigger *Trigger
	// Recurrence makes it a recurring template (not with SendAt or
	// Trigger); its Timezone is set by the service from the organization.
	Recurrence *Recurrence
}

// Scheduling bounds: a time in the past or a few seconds ahead is "now";
// further than MaxScheduleAhead is refused.
const (
	MinScheduleAhead = time.Minute
	MaxScheduleAhead = 30 * 24 * time.Hour
)

// StatsGroup is what push metrics are broken down by.
type StatsGroup string

const (
	StatsByDay       StatsGroup = "day"
	StatsByBroadcast StatsGroup = "broadcast"
	StatsByPwa       StatsGroup = "pwa"
	StatsByCampaign  StatsGroup = "campaign"
	StatsByCountry   StatsGroup = "country"
)

func (g StatsGroup) valid() bool {
	switch g {
	case StatsByDay, StatsByBroadcast, StatsByPwa, StatsByCampaign, StatsByCountry:
		return true
	}
	return false
}

// StatsCounts are push metrics over a set of sent broadcasts. Sent is the
// deliveries queued; Delivered/Failed/Unsubscribed/Opened as on Broadcast.
type StatsCounts struct {
	Sent         int `json:"sent"`
	Delivered    int `json:"delivered"`
	Failed       int `json:"failed"`
	Unsubscribed int `json:"unsubscribed"`
	Opened       int `json:"opened"`
}

// StatsRow is one group of push metrics: Key is the day (YYYY-MM-DD in the
// report's timezone), broadcast, PWA or campaign id, or country; Label the
// broadcast's name for StatsByBroadcast.
type StatsRow struct {
	Key   string `json:"key"`
	Label string `json:"label,omitempty"`
	StatsCounts
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
