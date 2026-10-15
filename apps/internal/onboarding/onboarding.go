// Package onboarding answers "what is left before this organization sees
// its first conversion": the Overview's "First steps" checklist (audit
// 2026-10-15, UX-3). Each step is read from the organization's real data —
// nothing is stored, so a step is done the moment the thing exists and
// undone if it is removed.
//
// handler → service → repository, like every other domain; every query is
// scoped to the organization from the session (CLAUDE.md #5).
package onboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// The steps, in the order an operator meets them. The ids are the
// contract with apps/web (features/dashboard/first-steps.tsx).
const (
	StepSource     = "source"     // a traffic source
	StepNetwork    = "network"    // an affiliate network
	StepMapping    = "mapping"    // a network status mapped to a FLOX status
	StepOffer      = "offer"      // an offer
	StepCampaign   = "campaign"   // an active campaign with an active tracking link
	StepClick      = "click"      // a first click
	StepConversion = "conversion" // a first accepted postback
	StepCost       = "cost"       // spend entered or an ad account connected
)

var order = []string{StepSource, StepNetwork, StepMapping, StepOffer, StepCampaign, StepClick, StepConversion, StepCost}

type Step struct {
	ID   string `json:"id"`
	Done bool   `json:"done"`
	// Unknown: the step could not be checked (no ClickHouse); it is shown
	// as not done, with no claim either way.
	Unknown bool `json:"unknown,omitempty"`
}

type Checklist struct {
	Steps []Step `json:"steps"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

// Repository reads the Postgres side.
type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// Done reports, per Postgres-backed step, whether the organization has it.
func (r *Repository) Done(ctx context.Context, orgID string) (map[string]bool, error) {
	var source, network, mapping, offer, campaign, conversion, cost bool
	err := r.db.QueryRow(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM traffic_sources WHERE organization_id = $1 AND status <> 'archived'),
			EXISTS (SELECT 1 FROM networks WHERE organization_id = $1 AND status <> 'archived'),
			EXISTS (SELECT 1 FROM event_mappings WHERE organization_id = $1),
			EXISTS (SELECT 1 FROM offers WHERE organization_id = $1 AND status <> 'archived'),
			EXISTS (SELECT 1 FROM tracking_links tl JOIN campaigns c ON c.id = tl.campaign_id
			        WHERE tl.organization_id = $1 AND c.organization_id = $1
			          AND tl.status = 'active' AND c.status = 'active'),
			EXISTS (SELECT 1 FROM postbacks WHERE organization_id = $1 AND result = 'success'),
			EXISTS (SELECT 1 FROM cost_entries WHERE organization_id = $1)
			  OR EXISTS (SELECT 1 FROM ad_account_connections WHERE organization_id = $1)`,
		orgID,
	).Scan(&source, &network, &mapping, &offer, &campaign, &conversion, &cost)
	if err != nil {
		return nil, fmt.Errorf("onboarding: reading steps: %w", err)
	}
	return map[string]bool{
		StepSource: source, StepNetwork: network, StepMapping: mapping, StepOffer: offer,
		StepCampaign: campaign, StepConversion: conversion, StepCost: cost,
	}, nil
}

// ClickProbe says whether the organization has any click. ClickHouse holds
// clicks; its sort key leads with organization_id, so this reads one granule.
type ClickProbe interface {
	HasClick(ctx context.Context, orgID string) (bool, error)
}

type CHClickProbe struct{ conn driver.Conn }

func NewCHClickProbe(conn driver.Conn) *CHClickProbe { return &CHClickProbe{conn: conn} }

func (p *CHClickProbe) HasClick(ctx context.Context, orgID string) (bool, error) {
	var n uint64
	if err := p.conn.QueryRow(ctx,
		`SELECT count() FROM (SELECT 1 FROM click_events WHERE organization_id = ? AND type = 'SOURCE_CLICK' LIMIT 1)`,
		orgID,
	).Scan(&n); err != nil {
		return false, fmt.Errorf("onboarding: probing clicks: %w", err)
	}
	return n > 0, nil
}

// StepReader is what Service needs from the Postgres side; *Repository is
// the real one.
type StepReader interface {
	Done(ctx context.Context, orgID string) (map[string]bool, error)
}

type Service struct {
	repo   StepReader
	clicks ClickProbe // nil: the click step is unknown
	logger *slog.Logger
}

func NewService(repo StepReader, clicks ClickProbe, logger *slog.Logger) *Service {
	return &Service{repo: repo, clicks: clicks, logger: logger}
}

func (s *Service) Checklist(ctx context.Context, orgID string) (Checklist, error) {
	if orgID == "" {
		return Checklist{}, apierror.Validation("missing organization", nil)
	}
	done, err := s.repo.Done(ctx, orgID)
	if err != nil {
		return Checklist{}, err
	}
	clickUnknown := s.clicks == nil
	if s.clicks != nil {
		has, err := s.clicks.HasClick(ctx, orgID)
		if err != nil {
			// ClickHouse down must not take the Overview's checklist with
			// it: the step reads as unknown.
			s.logger.Warn("onboarding: click probe failed", "error", err)
			clickUnknown = true
		}
		done[StepClick] = has
	}
	// A conversion needs a click to attribute to, so a recorded one proves
	// a click even when ClickHouse cannot say.
	if done[StepConversion] {
		done[StepClick], clickUnknown = true, false
	}

	out := Checklist{Total: len(order)}
	for _, id := range order {
		st := Step{ID: id, Done: done[id], Unknown: id == StepClick && clickUnknown}
		if st.Done {
			out.Done++
		}
		out.Steps = append(out.Steps, st)
	}
	return out, nil
}

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Register mounts GET /onboarding. The caller applies tenant.Middleware
// and the read gate.
func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.get)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	c, err := h.svc.Checklist(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(c)
}
