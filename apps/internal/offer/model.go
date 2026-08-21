// Package offer implements the Offer API (§27/Phase 11): handler →
// service → repository, modeling the real hierarchy the spec names —
// Network → Offer → Offer Link. An Offer always belongs to a Network
// (offers.network_id is NOT NULL) and always carries at least one Offer
// Link (a labeled tracking URL) — both are enforced at the service layer,
// matching the frontend form's own "at least one link" requirement
// (offer-form-sheet.tsx).
package offer

import (
	"encoding/json"
	"time"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusArchived Status = "archived"
)

func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusPaused, StatusArchived:
		return true
	default:
		return false
	}
}

// Link is one row of offer_links — always read/written as part of its
// parent Offer, never addressed independently (no standalone Link CRUD
// endpoint), matching offer-form-sheet.tsx's whole-array field editing.
type Link struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	URL   string `json:"url"`
}

type LinkInput struct {
	Label string
	URL   string
}

type Offer struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organizationId"`
	NetworkID      string   `json:"networkId"`
	Name           string   `json:"name"`
	Countries      []string `json:"countries"`
	Payout         float64  `json:"payout"`
	Currency       string   `json:"currency"`
	// Cap is nil for "uncapped" — matching cap's NULL-able DB column,
	// never a sentinel like 0 or -1.
	Cap       *int      `json:"cap"`
	Status    Status    `json:"status"`
	Links     []Link    `json:"links"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CreateInput struct {
	NetworkID string
	Name      string
	Countries []string
	Payout    float64
	Currency  string
	Cap       *int
	Links     []LinkInput
}

// OptionalCap distinguishes PATCH's three real states for a nullable
// field — "not sent" (the whole *OptionalCap is nil), "sent as null,
// i.e. uncapped" (Set true, Value nil), and "sent as a number" (Set true,
// Value non-nil). A plain *int in UpdateInput can't tell "not sent" apart
// from "sent as null" — encoding/json collapses both to a nil pointer —
// and Cap is the one field in this API that's genuinely both nullable
// (uncapped) and optional-in-a-partial-PATCH (the Archive convenience
// endpoint sends {"status":"archived"} alone, same as campaign/
// trafficsource, and must leave Cap untouched).
type OptionalCap struct {
	Set   bool
	Value *int
}

func (o *OptionalCap) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var v int
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// UpdateInput fields are pointers so PATCH can distinguish "not sent"
// from "sent" — matching campaign/trafficsource's own UpdateInput shape.
// Links is nil for "leave links unchanged" vs a (possibly empty, though
// the service rejects empty) slice for "replace every link with this
// set" — the same whole-array-replace semantics the frontend form
// already uses (useFieldArray, submitted as one array).
type UpdateInput struct {
	NetworkID *string
	Name      *string
	Countries *[]string
	Payout    *float64
	Currency  *string
	Cap       *OptionalCap
	Status    *Status
	Links     *[]LinkInput
}
