// Package custommetrics implements §30.5 — user-built formulas over the
// metrics registry (§50). Formula computation itself stays client-side
// (features/analytics/report-table.tsx already evaluates a published
// metric's formula against real /analytics/report row data); this
// package owns definitions only: real, tenant-scoped, RBAC-gated CRUD
// plus server-side validation mirroring apps/web/src/lib/formula-
// engine.ts's rules exactly, so a malicious or buggy client can never
// save a formula the UI itself would have rejected.
package custommetrics

import "time"

type Format string

const (
	FormatInt      Format = "int"
	FormatCurrency Format = "currency"
	FormatPercent  Format = "percent"
	FormatRatio    Format = "ratio"
)

func (f Format) valid() bool {
	switch f {
	case FormatInt, FormatCurrency, FormatPercent, FormatRatio:
		return true
	}
	return false
}

type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
)

func (s Status) valid() bool {
	return s == StatusDraft || s == StatusPublished
}

// ShowInTarget mirrors the frontend's ShowInTarget union exactly —
// which surfaces a published metric can be exposed on. offers_table/
// sources_table are valid targets today but never computable (no
// backing metrics on either surface yet — see surfaceAvailableMetricIDs),
// so they're accepted here but Service.filterComputableTargets always
// drops them, same as the frontend's own surfaceCanCompute check.
type ShowInTarget string

const (
	TargetReportBuilder  ShowInTarget = "report_builder"
	TargetCampaignsTable ShowInTarget = "campaigns_table"
	TargetOffersTable    ShowInTarget = "offers_table"
	TargetSourcesTable   ShowInTarget = "sources_table"
)

func (t ShowInTarget) valid() bool {
	switch t {
	case TargetReportBuilder, TargetCampaignsTable, TargetOffersTable, TargetSourcesTable:
		return true
	}
	return false
}

type CustomMetric struct {
	ID              string    `json:"id"`
	OrganizationID  string    `json:"organizationId"`
	Name            string    `json:"name"`
	GroupName       string    `json:"group"`
	Formula         string    `json:"formula"`
	Format          Format    `json:"format"`
	Status          Status    `json:"status"`
	Active          bool      `json:"active"`
	Targets         []string  `json:"targets"`
	CreatedByUserID string    `json:"createdByUserId"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type CreateInput struct {
	Name      string
	GroupName string
	Formula   string
	Format    Format
	Status    Status
	Targets   []string
}

type UpdateInput struct {
	Name      *string
	GroupName *string
	Formula   *string
	Format    *Format
	Status    *Status
	Targets   *[]string
	Active    *bool
}
