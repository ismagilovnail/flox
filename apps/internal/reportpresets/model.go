// Package reportpresets implements §27.5 (Phase 14.7) — a saved, reusable
// report-builder configuration: {dimensions, metrics, groupBy, period,
// timezone}. Presets are team-scoped (organization-wide, not per-user) per
// the spec's own wording — see migration 00030's doc comment for why that
// means no owner-based access restriction, unlike Custom Metrics.
package reportpresets

import "time"

// PeriodType is the frontend's own ReportPeriod tagged-union discriminant
// (lib/mock/report-presets.ts), stored as flat columns rather than JSONB.
type PeriodType string

const (
	PeriodRelative PeriodType = "relative"
	PeriodCustom   PeriodType = "custom"
)

func (t PeriodType) valid() bool {
	return t == PeriodRelative || t == PeriodCustom
}

// Period mirrors {type:"relative",days} | {type:"custom",from,to} exactly.
// Days is set only for PeriodRelative; From/To only for PeriodCustom (dates
// as "YYYY-MM-DD" strings, matching the real /analytics/report wire format).
type Period struct {
	Type PeriodType `json:"type"`
	Days *int       `json:"days,omitempty"`
	From *string    `json:"from,omitempty"`
	To   *string    `json:"to,omitempty"`
}

// validMetrics mirrors features/analytics/registry.ts's MetricKey union
// exactly — 13 entries. No Go-side metrics registry exists to import (the
// real /analytics/report endpoint always returns every metric; there's
// nothing server-side to validate a client-selected metric list against),
// so this is a deliberate, small, doc-commented duplicate — same tradeoff
// custommetrics/registry.go already accepted for its own frontend mirror.
var validMetrics = map[string]bool{
	"clicks": true, "uniqueClicks": true, "conversions": true, "revenue": true,
	"cost": true, "profit": true, "roi": true, "roas": true, "ctr": true,
	"cvr": true, "cpc": true, "cpa": true, "epc": true,
}

// maxDimensions matches maxReportDims in apps/internal/analytics/report.go
// — a preset that can't ever produce a valid report isn't useful to save.
const maxDimensions = 4

type ReportPreset struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	Name           string    `json:"name"`
	Dimensions     []string  `json:"dimensions"`
	Metrics        []string  `json:"metrics"`
	GroupBy        string    `json:"groupBy"`
	Period         Period    `json:"period"`
	Timezone       string    `json:"timezone"`
	IsDefault      bool      `json:"isDefault"`
	CreatedByID    *string   `json:"createdByUserId,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// CreateInput and UpdateInput never carry IsDefault — the default preset is
// system-seeded (Service.List, via Repository.EnsureDefault) and can never
// be created, renamed, or deleted through the normal CRUD surface.
type CreateInput struct {
	Name       string
	Dimensions []string
	Metrics    []string
	GroupBy    string
	Period     Period
	Timezone   string
}

type UpdateInput struct {
	Name       *string
	Dimensions []string
	Metrics    []string
	GroupBy    *string
	Period     *Period
	Timezone   *string
}

// defaultPresetSeed is what Repository.EnsureDefault creates the very
// first time an org has zero report presets — matching the mock's own
// seed data (lib/mock/report-presets.ts's "Campaign Overview (default)")
// so a real org's first-ever preset list looks the same as it did against
// the mock. Days: 30 is the frontend's own default relative window.
var defaultPresetSeed = ReportPreset{
	Name:       "Campaign Overview (default)",
	Dimensions: []string{"campaign"},
	Metrics:    []string{"clicks", "conversions", "revenue", "roi"},
	GroupBy:    "campaign",
	Period:     Period{Type: PeriodRelative, Days: intPtr(30)},
	Timezone:   "UTC",
}

func intPtr(v int) *int { return &v }
