// Package eventmapping is the CRUD write path for event_mappings
// (migration 00012, §29) — per-network translation from a network's own
// raw postback status string to the canonical §43 CpaStatus. The read
// side already existed: apps/internal/conversion.PostgresMapper.MapStatus
// looks this table up at ingest time (Phase 23). This package never
// duplicates that lookup — it only writes rows for the existing reader to
// load, the same relationship apps/internal/streamset has to
// apps/internal/routingstore.
package eventmapping

import (
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// EventMapping mirrors the event_mappings row exactly. FloxStatus reuses
// event.Type rather than a locally-redeclared enum — the same "don't
// redefine an enum that already exists" call every other domain package
// in this codebase makes for routing.FilterField, routing.Joiner, etc.
type EventMapping struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organizationId"`
	NetworkID      string     `json:"networkId"`
	NetworkStatus  string     `json:"networkStatus"`
	FloxStatus     event.Type `json:"floxStatus"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type CreateInput struct {
	NetworkID     string
	NetworkStatus string
	FloxStatus    event.Type
}
