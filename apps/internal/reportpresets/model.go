// Package reportpresets implements §27.5 — a saved, reusable
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

// validMetrics mirrors apps/web/src/types/analytics.ts's MetricKey union
// exactly — 27 entries. No Go-side metrics registry exists to import (the
// real /analytics/report endpoint always returns every metric; there's
// nothing server-side to validate a client-selected metric list against),
// so this is a deliberate, small, doc-commented duplicate — same tradeoff
// custommetrics/registry.go already accepted for its own frontend mirror.
var validMetrics = map[string]bool{
	"clicks": true, "uniqueClicks": true, "conversions": true, "revenue": true,
	"cost": true, "profit": true, "roi": true, "roas": true, "ctr": true,
	"cvr": true, "cpc": true, "cpa": true, "epc": true,
	"pwaInstalls": true, "registrations": true, "purchases": true, "repurchases": true,
	"declines": true, "trash": true,
	"clickToInstall": true, "installToReg": true, "regToPurchase": true,
	"costPerInstall": true, "costPerReg": true, "costPerPurchase": true,
	"blocked": true, "bots": true, "blockedRate": true, "botRate": true,
	"emptySubs": true, "emptySubsRate": true,
}

// maxDimensions matches maxReportDims in apps/internal/analytics/report.go
// — a preset that can't ever produce a valid report isn't useful to save.
const maxDimensions = 4

// Filter is one saved "dimension = value" condition, AND-combined with the
// others — the report builder's FilterCondition.
type Filter struct {
	Dimension string `json:"dimension"`
	Value     string `json:"value"`
}

// maxFilters / maxFilterValueLen bound what one preset may carry — a guard,
// not a spec number: a handful of conditions is a report, dozens is a bug.
const (
	maxFilters        = 10
	maxFilterValueLen = 200
	maxColumnOrder    = 60 // dimensions (4) + metrics (13) + custom metrics, with room to spare
	maxColumnIDLen    = 64
	minColumnWidth    = 48 // the UI never lets a column get narrower
	maxColumnWidth    = 2000
)

type ReportPreset struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	Name           string         `json:"name"`
	Dimensions     []string       `json:"dimensions"`
	Metrics        []string       `json:"metrics"`
	Filters        []Filter       `json:"filters"`
	ColumnOrder    []string       `json:"columnOrder"`
	ColumnWidths   map[string]int `json:"columnWidths"`
	HiddenColumns  []string       `json:"hiddenColumns"`
	GroupBy        string         `json:"groupBy"`
	Period         Period         `json:"period"`
	Timezone       string         `json:"timezone"`
	// DateBasis is how the report dates conversions: "conversion" or
	// "click" (tech audit AN5).
	DateBasis   string    `json:"dateBasis"`
	IsDefault   bool      `json:"isDefault"`
	CreatedByID *string   `json:"createdByUserId,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// CreateInput and UpdateInput never carry IsDefault — the default preset is
// system-seeded (Service.List, via Repository.EnsureDefault) and can never
// be created, renamed, or deleted through the normal CRUD surface.
type CreateInput struct {
	Name       string
	Dimensions []string
	Metrics    []string
	Filters    []Filter
	// ColumnOrder is the dragged column order; empty means the natural order.
	ColumnOrder []string
	// ColumnWidths is the dragged column widths in px by column id; empty means natural widths.
	ColumnWidths map[string]int
	// HiddenColumns is the ids of the columns hidden in the table; empty means all shown.
	HiddenColumns []string
	GroupBy       string
	Period        Period
	Timezone      string
	// DateBasis: "" means "conversion".
	DateBasis string
}

type UpdateInput struct {
	Name       *string
	Dimensions []string
	Metrics    []string
	// Filters: nil leaves them as they are; an empty non-nil slice clears them.
	Filters []Filter
	// ColumnOrder: nil leaves it as it is; an empty non-nil slice resets to the natural order.
	ColumnOrder []string
	// ColumnWidths: nil leaves them as they are; an empty non-nil map resets to natural widths.
	ColumnWidths map[string]int
	// HiddenColumns: nil leaves them as they are; an empty non-nil slice shows every column.
	HiddenColumns []string
	GroupBy       *string
	Period        *Period
	Timezone      *string
	DateBasis     *string
}

// defaultPresetSeed is what Repository.EnsureDefault creates the very
// first time an org has zero report presets — matching the mock's own
// seed data (lib/mock/report-presets.ts's "Campaign Overview (default)")
// so a real org's first-ever preset list looks the same as it did against
// the mock. The metric set is the Analytics page's default (analytics-
// view.tsx): the PWA → registration → purchase funnel with each step's rate
// and cost, then money, over the last 7 days (marketer audit #12;
// migration 00058 moved existing default presets onto it). The timezone is
// the organization's own, filled in by EnsureDefault. The UI shows the name
// translated; it is stored in English.
var defaultPresetSeed = ReportPreset{
	Name:       "Campaign Overview (default)",
	Dimensions: []string{"campaign"},
	Metrics: []string{
		"clicks", "pwaInstalls", "clickToInstall", "registrations", "installToReg",
		"purchases", "regToPurchase", "repurchases",
		"cost", "costPerInstall", "costPerReg", "costPerPurchase",
		"revenue", "profit", "roi", "epc",
	},
	GroupBy: "campaign",
	Period:  Period{Type: PeriodRelative, Days: intPtr(7)},
}

func intPtr(v int) *int { return &v }
