// Package postbacklogs is the browser-facing read layer over ClickHouse's
// postback_events (§48) — the "Postback Logs" tab. Like
// apps/internal/analytics and apps/internal/conversions, it is
// deliberately narrow: no writes, no replay. Read-only for now; the
// replay action (re-invoking apps/internal/conversion for an incoming
// row, or re-enqueuing a apps/internal/postback delivery for an outgoing
// one — both real, buildable actions with no schema changes needed) was
// deliberately scoped out of this phase to keep it reviewable. Not to be
// confused with apps/internal/postbacklog (singular) — Phase 24's
// write-side queue/producer that feeds postback_events in the first
// place, untouched by this package.
package postbacklogs

import "time"

// PostbackLog is one attempt row for the list page — both directions
// mixed, discriminated by Direction, matching the frontend's single
// table. EventRef and OrganizationID from the underlying ClickHouse row
// are dropped: the UI never renders either.
type PostbackLog struct {
	EventAt            time.Time `json:"eventAt"`
	Direction          string    `json:"direction"`
	NetworkID          string    `json:"networkId"`
	ClickID            string    `json:"clickId"`
	Status             string    `json:"status,omitempty"`    // mapped FLOX status; empty for an incoming error before mapping
	RawStatus          string    `json:"rawStatus,omitempty"` // incoming only
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
