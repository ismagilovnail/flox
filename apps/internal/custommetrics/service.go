package custommetrics

import (
	"context"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

const (
	nameMinLen  = 2
	nameMaxLen  = 80
	groupMaxLen = 40
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// tier mirrors the frontend's own canManageAny/canCreate/canManage
// logic (features/custom-metrics/custom-metrics-list.tsx) exactly —
// ported here so the same rule is actually enforced, not just assumed
// client-side (MeResponse's own doc comment: "frontend permissions are
// only UX").
type tier struct {
	manageAny bool // Owner/Admin — any metric, org-wide
	canCreate bool // manageAny || Manager
	role      string
	userID    string
}

func (s *Service) callerTier(ctx context.Context, orgID, userID string) (tier, error) {
	role, err := s.repo.CallerRoleKey(ctx, orgID, userID)
	if err != nil {
		return tier{}, err
	}
	manageAny := role == "Owner" || role == "Admin"
	return tier{manageAny: manageAny, canCreate: manageAny || role == "Manager", role: role, userID: userID}, nil
}

func (t tier) canManage(m CustomMetric) bool {
	return t.manageAny || (t.role == "Manager" && m.CreatedByUserID == t.userID)
}

func (s *Service) List(ctx context.Context, orgID, userID string) ([]CustomMetric, error) {
	t, err := s.callerTier(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.List(ctx, orgID, t.canCreate)
}

func (s *Service) Get(ctx context.Context, orgID, userID, id string) (CustomMetric, error) {
	m, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return CustomMetric{}, err
	}
	if m.Status == StatusPublished && m.Active {
		return m, nil
	}
	t, err := s.callerTier(ctx, orgID, userID)
	if err != nil {
		return CustomMetric{}, err
	}
	if !t.canManage(m) {
		return CustomMetric{}, apierror.NotFound("custom metric not found")
	}
	return m, nil
}

// validateAndFilterTargets runs the same three formula checks the
// frontend's validateFormula does (syntax, unknown-metric/LTV-forbidden,
// single-data-source), then silently drops any requested target the
// formula can't actually feed — same as custom-metrics-list.tsx's own
// `targets.filter(t => surfaceCanCompute(...))` at save time, not an
// error, since the UI's own "Show in" checkboxes are already disabled
// for those cases and a client bypassing that shouldn't get a scarier
// failure than "it just won't show there."
func validateAndFilterTargets(formula string, targets []string) ([]string, error) {
	v := validateFormula(formula)
	if !v.Valid {
		return nil, apierror.Validation(v.Error, map[string]string{"formula": v.Error})
	}
	kept := make([]string, 0, len(targets))
	for _, t := range targets {
		if surfaceCanCompute(ShowInTarget(t), v.UsedMetricIDs) {
			kept = append(kept, t)
		}
	}
	return kept, nil
}

func validateNameGroupFormat(name, group string, format Format) (string, string, error) {
	name = strings.TrimSpace(name)
	if len(name) < nameMinLen {
		return "", "", apierror.Validation("name must be at least 2 characters", map[string]string{"name": "too short"})
	}
	if len(name) > nameMaxLen {
		return "", "", apierror.Validation("name is too long", map[string]string{"name": "too long"})
	}
	group = strings.TrimSpace(group)
	if group == "" {
		return "", "", apierror.Validation("group is required", map[string]string{"group": "required"})
	}
	if len(group) > groupMaxLen {
		return "", "", apierror.Validation("group is too long", map[string]string{"group": "too long"})
	}
	if !format.valid() {
		return "", "", apierror.Validation("invalid format", map[string]string{"format": "invalid"})
	}
	return name, group, nil
}

func (s *Service) Create(ctx context.Context, orgID, userID string, in CreateInput) (CustomMetric, error) {
	name, group, err := validateNameGroupFormat(in.Name, in.GroupName, in.Format)
	if err != nil {
		return CustomMetric{}, err
	}
	if !in.Status.valid() {
		return CustomMetric{}, apierror.Validation("invalid status", map[string]string{"status": "invalid"})
	}
	targets, err := validateAndFilterTargets(in.Formula, in.Targets)
	if err != nil {
		return CustomMetric{}, err
	}
	// A draft is invisible in reports until published (§30.5) — force no
	// exposure targets rather than trust the caller to have cleared them,
	// same rule Update enforces on a publish -> draft transition below.
	// []string{}, not nil: pgx encodes a nil Go slice as SQL NULL, which
	// the column's NOT NULL constraint rejects outright (its DEFAULT '{}'
	// only applies when the column is omitted from the INSERT, not when
	// NULL is passed explicitly) — caught by the integration tests below,
	// not by inspection.
	if in.Status == StatusDraft {
		targets = []string{}
	}

	return s.repo.Create(ctx, orgID, userID, CreateInput{
		Name: name, GroupName: group, Formula: in.Formula, Format: in.Format, Status: in.Status, Targets: targets,
	})
}

func (s *Service) Update(ctx context.Context, orgID, userID, id string, in UpdateInput) (CustomMetric, error) {
	existing, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return CustomMetric{}, err
	}
	t, err := s.callerTier(ctx, orgID, userID)
	if err != nil {
		return CustomMetric{}, err
	}
	if !t.canManage(existing) {
		return CustomMetric{}, apierror.Forbidden("you can only manage custom metrics you created")
	}

	if in.Name != nil || in.GroupName != nil {
		name := existing.Name
		if in.Name != nil {
			name = *in.Name
		}
		group := existing.GroupName
		if in.GroupName != nil {
			group = *in.GroupName
		}
		format := existing.Format
		if in.Format != nil {
			format = *in.Format
		}
		validName, validGroup, err := validateNameGroupFormat(name, group, format)
		if err != nil {
			return CustomMetric{}, err
		}
		in.Name, in.GroupName = &validName, &validGroup
	} else if in.Format != nil && !in.Format.valid() {
		return CustomMetric{}, apierror.Validation("invalid format", map[string]string{"format": "invalid"})
	}

	if in.Status != nil && !in.Status.valid() {
		return CustomMetric{}, apierror.Validation("invalid status", map[string]string{"status": "invalid"})
	}

	formula := existing.Formula
	if in.Formula != nil {
		formula = *in.Formula
	}
	nextStatus := existing.Status
	if in.Status != nil {
		nextStatus = *in.Status
	}
	if in.Formula != nil || in.Targets != nil {
		requested := existing.Targets
		if in.Targets != nil {
			requested = *in.Targets
		}
		targets, err := validateAndFilterTargets(formula, requested)
		if err != nil {
			return CustomMetric{}, err
		}
		in.Targets = &targets
	}
	// Unpublishing clears exposure targets server-side — the single
	// place this rule is enforced, rather than trusting every caller
	// (row-actions' publish toggle, the edit form, a future API client)
	// to remember to clear them themselves.
	if nextStatus == StatusDraft {
		empty := []string{}
		in.Targets = &empty
	}

	return s.repo.Update(ctx, orgID, id, in)
}

func (s *Service) Delete(ctx context.Context, orgID, userID, id string) error {
	existing, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return err
	}
	t, err := s.callerTier(ctx, orgID, userID)
	if err != nil {
		return err
	}
	if !t.canManage(existing) {
		return apierror.Forbidden("you can only manage custom metrics you created")
	}
	if existing.Status == StatusPublished || len(existing.Targets) > 0 {
		return apierror.Conflict("can't delete — unpublish and clear its Show-in targets first, or deactivate it instead")
	}
	return s.repo.Delete(ctx, orgID, id)
}

// Duplicate mirrors the frontend's own duplicate(): a fresh draft copy,
// no exposure targets, owned by the duplicating caller (not the
// original creator) — same as clicking "New Custom Metric" yourself
// with the fields pre-filled.
func (s *Service) Duplicate(ctx context.Context, orgID, userID, id string) (CustomMetric, error) {
	existing, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return CustomMetric{}, err
	}
	t, err := s.callerTier(ctx, orgID, userID)
	if err != nil {
		return CustomMetric{}, err
	}
	if !t.canManage(existing) {
		return CustomMetric{}, apierror.Forbidden("you can only manage custom metrics you created")
	}
	return s.repo.Create(ctx, orgID, userID, CreateInput{
		Name:      existing.Name + " (Copy)",
		GroupName: existing.GroupName,
		Formula:   existing.Formula,
		Format:    existing.Format,
		Status:    StatusDraft,
		Targets:   []string{},
	})
}
