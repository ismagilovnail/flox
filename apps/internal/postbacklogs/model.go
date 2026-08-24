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
// Incoming replay (ReplayIncoming) re-invokes
// apps/internal/conversion.Service.Record for a past incoming row, through
// the decoupled IncomingRecorder/IncomingNetworkLookup interfaces below —
// this package still never imports apps/internal/conversion directly (see
// apps/internal/conversion/replay.go for the adapters that satisfy them).
// See docs/postback-logs.md for why incoming and outgoing replay shipped
// as separate phases. Not to be confused with apps/internal/postbacklog
// (singular) — Phase 24's write-side queue/producer that feeds
// postback_events in the first place, untouched by this package.
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

// ReplayIncomingInput is what the browser posts to replay one incoming
// postback attempt — exactly the fields a PostbackLog row already carries
// client-side, no second fetch. EventRef doubles as the CPA_REDEP network
// transaction id on replay: conversion.Service derives event_ref FROM the
// txn id for that status only (conversion.eventRefFor), so passing a log
// row's own event_ref back in reproduces the identical dedup key for
// every status, redeposits included.
type ReplayIncomingInput struct {
	NetworkID string   `json:"networkId"`
	ClickID   string   `json:"clickId"`
	RawStatus string   `json:"rawStatus"`
	EventRef  string   `json:"eventRef,omitempty"`
	Revenue   *float64 `json:"revenue,omitempty"`
	Currency  string   `json:"currency,omitempty"`
}

type ReplayIncomingResult struct {
	ID      string `json:"id"`
	Result  string `json:"result"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
}
