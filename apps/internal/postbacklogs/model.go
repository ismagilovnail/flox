// Package postbacklogs is the browser-facing layer over ClickHouse's
// postback_events (§48) — the "Postback Logs" tab. Like
// apps/internal/analytics and apps/internal/conversions, reads are
// deliberately narrow (no writes into ClickHouse itself). The one write
// this package does perform is outgoing replay (ReplayOutgoing): it never
// writes to postback_events directly — apps/internal/postbacklog's flusher
// still owns that — it only re-enqueues a delivery through
// apps/internal/postback.Store, the exact path a first attempt already
// takes, so the replayed attempt gets logged the normal way once
// apps/worker's Deliverer picks it up.
//
// Incoming replay (re-invoking apps/internal/conversion.Service.Record for
// an incoming row) is still deliberately deferred — see docs/postback-logs.md
// for why the two turned out to be very different sizes. Not to be
// confused with apps/internal/postbacklog (singular) — Phase 24's
// write-side queue/producer that feeds postback_events in the first
// place, untouched by this package.
package postbacklogs

import "time"

// PostbackLog is one attempt row for the list page — both directions
// mixed, discriminated by Direction, matching the frontend's single
// table. OrganizationID from the underlying ClickHouse row is dropped (the
// UI never renders it); EventRef is kept — outgoing replay needs it back
// from the browser to resolve the exact source postbacks row (see
// Service.ReplayOutgoing), since a CPA_REDEP click can have more than one
// successful row, one per redeposit, each with its own event_ref.
type PostbackLog struct {
	EventAt            time.Time `json:"eventAt"`
	Direction          string    `json:"direction"`
	NetworkID          string    `json:"networkId"`
	ClickID            string    `json:"clickId"`
	Status             string    `json:"status,omitempty"`    // mapped FLOX status; empty for an incoming error before mapping
	RawStatus          string    `json:"rawStatus,omitempty"` // incoming only
	EventRef           string    `json:"eventRef,omitempty"`
	Result             string    `json:"result"`
	Message            string    `json:"message"`
	AttemptCount       int64     `json:"attemptCount,omitempty"`       // outgoing only
	ResponseStatusCode int64     `json:"responseStatusCode,omitempty"` // outgoing only
	URL                string    `json:"url,omitempty"`                // outgoing only
	Revenue            float64   `json:"revenue,omitempty"`            // incoming only
	Currency           string    `json:"currency,omitempty"`
}

type ListResult struct {
	Logs  []PostbackLog `json:"logs"`
	Total int           `json:"total"`
}

// ReplayOutgoingInput is what the browser posts to replay one outgoing
// delivery attempt — exactly the fields off a PostbackLog row the UI
// already has in hand, no second fetch needed.
type ReplayOutgoingInput struct {
	NetworkID string `json:"networkId"`
	ClickID   string `json:"clickId"`
	Status    string `json:"status"`
	EventRef  string `json:"eventRef"`
	URL       string `json:"url"`
}

type ReplayOutgoingResult struct {
	DeliveryID string `json:"deliveryId"`
}
