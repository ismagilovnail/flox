// Package offlineconv reports conversions back to the ad network that
// bought the click: to Google Ads by the click's gclid/gbraid/wbraid,
// through Google's Data Manager API (audit UNI-2 part 2), and to Yandex
// Metrica by its yclid (part 3, metrica.go; docs/offline-conversions.md).
//
// A traffic source connected to Google Ads (internal/adaccount) turns it
// on and names the conversion action each status is reported as. From
// then on apps/worker:
//
//   - Enqueue: every new lead, purchase or repeat purchase postback of an
//     organization with the export on gets a row in
//     offline_conversion_uploads — pending when its click came from Google
//     with a click id and its campaign's source has the export on, skipped
//     (with the reason) otherwise. A refund changes what is left of a sent
//     purchase: the row goes back to pending with the new value.
//   - Send: pending rows due, per source and action, in requests of at
//     most 2000; the transaction id is the postback id, so sending it again
//     with another value restates it instead of adding a conversion.
//
// Google refuses a whole request for one bad event; Send then sends the
// request's events one by one, and only the bad ones fail.
//
// A source's Metrica connection works the same way for clicks with yclid,
// with what Metrica's API allows: a file per counter, no restating (a
// refund after the upload stays in Metrica), and Check reading back how
// each file was matched.
package offlineconv

import "time"

// Upload statuses (migration 00104).
const (
	StatusPending = "pending"
	StatusSent    = "sent"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
)

// Skip reasons.
const (
	SkipNotAttributed = "not_attributed"  // the postback has no attributed click
	SkipClickNotFound = "click_not_found" // the attributed click is not in click_lookup
	SkipNoGoogleClick = "no_google_click" // the click carried no gclid/gbraid/wbraid
	SkipNoYandexClick = "no_yandex_click" // the click carried no yclid (a source exporting to Metrica only)
	SkipVisitTooOld   = "visit_too_old"   // the yclid click is older than Metrica attaches offline data to
	SkipNotConnected  = "source_off"      // the click's source has no connection for its network, or the export off
	SkipNoAction      = "no_action"       // no conversion action for the status
)

// Settings is a traffic source's export settings, on its ad account
// connection. An action id is Google Ads' numeric conversion action id;
// an empty one leaves that status unsent.
type Settings struct {
	Enabled            bool       `json:"enabled"`
	Since              *time.Time `json:"since"`
	LoginAccountID     string     `json:"loginAccountId"`
	LeadActionID       string     `json:"leadActionId"`
	PurchaseActionID   string     `json:"purchaseActionId"`
	RepurchaseActionID string     `json:"repurchaseActionId"`
}

// Status is what the source's page shows: the settings, how many
// conversions are in each state, why some were skipped, and the last
// error Google gave.
type Status struct {
	Settings
	Counts      map[string]int `json:"counts"`
	Skipped     map[string]int `json:"skipped"`
	LastError   string         `json:"lastError"`
	LastErrorAt *time.Time     `json:"lastErrorAt"`
	LastSentAt  *time.Time     `json:"lastSentAt"`
}

// MetricaSettings is a traffic source's export to Yandex Metrica. A goal
// is the identifier of a JavaScript-event goal of the counter; an empty
// one leaves that status unsent. Currency is the counter's: Metrica
// zeroes a price in any other.
type MetricaSettings struct {
	Enabled        bool       `json:"enabled"`
	Since          *time.Time `json:"since"`
	CounterID      string     `json:"counterId"`
	Currency       string     `json:"currency"`
	LeadGoal       string     `json:"leadGoal"`
	PurchaseGoal   string     `json:"purchaseGoal"`
	RepurchaseGoal string     `json:"repurchaseGoal"`
}

// MetricaInput is a write: the settings and, to connect or replace it,
// the OAuth token (empty keeps the stored one).
type MetricaInput struct {
	MetricaSettings
	Token string `json:"token"`
}

// MetricaStatus is what the source's page shows. Connected false: no
// counter yet (the other fields are defaults). The token is never sent
// back, only its last 4 characters. Processing: uploads Metrica has not
// finished matching.
type MetricaStatus struct {
	Connected    bool   `json:"connected"`
	TokenPreview string `json:"tokenPreview"`
	MetricaSettings
	Counts      map[string]int `json:"counts"`
	Skipped     map[string]int `json:"skipped"`
	Processing  int            `json:"processing"`
	LastError   string         `json:"lastError"`
	LastErrorAt *time.Time     `json:"lastErrorAt"`
	LastSentAt  *time.Time     `json:"lastSentAt"`
}
