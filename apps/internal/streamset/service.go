package streamset

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/routing"
)

const (
	nameMinLen = 2
	nameMaxLen = 80
)

// validFields/validOperators mirror lib/filters.ts's FILTER_FIELDS (31)
// and FILTER_OPERATORS (16) exactly — the same enums internal/routing's
// FilterField/FilterOperator types already name as constants, just
// re-expressed as a membership set since routing.go deliberately has no
// Valid() method (it trusts routingstore's already-validated Postgres
// rows; validating untrusted client input is this package's job, not
// the pure decision engine's).
var validFields = map[routing.FilterField]bool{
	routing.FieldCountry: true, routing.FieldRegion: true, routing.FieldCity: true,
	routing.FieldDevice: true, routing.FieldPlatform: true, routing.FieldOS: true, routing.FieldOSVersion: true,
	routing.FieldBrowser: true, routing.FieldBrowserVersion: true, routing.FieldLanguage: true,
	routing.FieldBot: true, routing.FieldProxy: true, routing.FieldASN: true, routing.FieldConnectionType: true,
	routing.FieldReferrer: true, routing.FieldUTMSource: true, routing.FieldUTMMedium: true,
	routing.FieldUTMCampaign: true, routing.FieldUTMContent: true, routing.FieldUTMTerm: true,
	routing.FieldSub1: true, routing.FieldSub2: true, routing.FieldSub3: true, routing.FieldSub4: true,
	routing.FieldSub5: true, routing.FieldSub6: true, routing.FieldSub7: true, routing.FieldSub8: true,
	routing.FieldSub9: true, routing.FieldSub10: true, routing.FieldExternalClickID: true,
}

var validOperators = map[routing.FilterOperator]bool{
	routing.OpIs: true, routing.OpIsNot: true, routing.OpIn: true, routing.OpNotIn: true,
	routing.OpContains: true, routing.OpNotContains: true, routing.OpStartsWith: true, routing.OpEndsWith: true,
	routing.OpMatches: true, routing.OpExists: true, routing.OpNotExists: true,
	routing.OpGT: true, routing.OpGTE: true, routing.OpLT: true, routing.OpLTE: true, routing.OpBetween: true,
}

var operatorsWithoutValue = map[routing.FilterOperator]bool{routing.OpExists: true, routing.OpNotExists: true}

var isoAlpha2 = regexp.MustCompile(`^[A-Z]{2}$`)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, orgID, campaignID string) ([]StreamSet, error) {
	return s.repo.List(ctx, orgID, campaignID)
}

func (s *Service) Get(ctx context.Context, orgID, campaignID, id string) (StreamSet, error) {
	return s.repo.GetByID(ctx, orgID, campaignID, id)
}

func (s *Service) Create(ctx context.Context, orgID, campaignID string, in CreateInput) (StreamSet, error) {
	in.Name = strings.TrimSpace(in.Name)
	fields := map[string]string{}
	if len(in.Name) < nameMinLen || len(in.Name) > nameMaxLen {
		fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
	}
	if in.FallbackURL != "" && !isValidURL(in.FallbackURL) {
		fields["fallbackUrl"] = "must be empty or a valid absolute URL"
	}
	if err := validateFilterNode(in.RootFilter, true); err != "" {
		fields["rootFilter"] = err
	}
	if err := s.validateFlows(ctx, orgID, in.Flows); err != "" {
		fields["flows"] = err
	}
	if len(fields) > 0 {
		return StreamSet{}, apierror.Validation("invalid stream set", fields)
	}

	belongs, err := s.repo.CampaignBelongsToOrg(ctx, orgID, campaignID)
	if err != nil {
		return StreamSet{}, fmt.Errorf("checking campaign: %w", err)
	}
	if !belongs {
		return StreamSet{}, apierror.NotFound("campaign not found")
	}

	in.Flows, err = s.resolveFlowNetworks(ctx, orgID, in.Flows)
	if err != nil {
		return StreamSet{}, err
	}

	// Priority is never client-supplied (no field for it in the frontend
	// form — matching stream-set-schema.ts exactly): a new stream set is
	// always appended after every existing one, same as
	// stores/stream-sets.ts's addStreamSet (`priority: list.length + 1`).
	// Reordering afterward is Reorder's job, not Create's.
	existing, err := s.repo.List(ctx, orgID, campaignID)
	if err != nil {
		return StreamSet{}, fmt.Errorf("counting existing stream sets: %w", err)
	}
	in.Priority = len(existing) + 1

	return s.repo.Create(ctx, idgen.New(), orgID, campaignID, in)
}

func (s *Service) Update(ctx context.Context, orgID, campaignID, id string, in UpdateInput) (StreamSet, error) {
	fields := map[string]string{}
	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		in.Name = &trimmed
		if len(trimmed) < nameMinLen || len(trimmed) > nameMaxLen {
			fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
		}
	}
	if in.Status != nil && *in.Status != StatusActive && *in.Status != StatusPaused {
		fields["status"] = "must be active or paused"
	}
	if in.FallbackURL != nil && *in.FallbackURL != "" && !isValidURL(*in.FallbackURL) {
		fields["fallbackUrl"] = "must be empty or a valid absolute URL"
	}
	if in.RootFilter != nil {
		if err := validateFilterNode(*in.RootFilter, true); err != "" {
			fields["rootFilter"] = err
		}
	}
	if in.Flows != nil {
		if err := s.validateFlows(ctx, orgID, *in.Flows); err != "" {
			fields["flows"] = err
		}
	}
	if len(fields) > 0 {
		return StreamSet{}, apierror.Validation("invalid stream set", fields)
	}

	if in.Flows != nil {
		resolved, err := s.resolveFlowNetworks(ctx, orgID, *in.Flows)
		if err != nil {
			return StreamSet{}, err
		}
		in.Flows = &resolved
	}

	return s.repo.Update(ctx, orgID, campaignID, id, in)
}

func (s *Service) Delete(ctx context.Context, orgID, campaignID, id string) error {
	return s.repo.Delete(ctx, orgID, campaignID, id)
}

// Duplicate keeps status as-is, same reasoning as every other domain's
// Duplicate this session — StreamSetStatus has no draft-equivalent to
// reset to. Appended to the end of priority order, matching
// stores/stream-sets.ts's own duplicateStreamSet.
func (s *Service) Duplicate(ctx context.Context, orgID, campaignID, id string) (StreamSet, error) {
	source, err := s.repo.GetByID(ctx, orgID, campaignID, id)
	if err != nil {
		return StreamSet{}, err
	}

	flowInputs := make([]FlowInput, len(source.Flows))
	for i, f := range source.Flows {
		flowInputs[i] = FlowInput{Name: f.Name, Active: f.Active, Weight: f.Weight, Destination: f.Destination}
	}

	existing, err := s.repo.List(ctx, orgID, campaignID)
	if err != nil {
		return StreamSet{}, fmt.Errorf("counting existing stream sets: %w", err)
	}

	created, err := s.repo.Create(ctx, idgen.New(), orgID, campaignID, CreateInput{
		Name:        source.Name + " (Copy)",
		Priority:    len(existing) + 1,
		FallbackURL: source.FallbackURL,
		RootFilter:  source.RootFilter,
		Flows:       flowInputs,
	})
	if err != nil {
		return StreamSet{}, err
	}
	if source.Status != StatusActive {
		status := source.Status
		return s.repo.Update(ctx, orgID, campaignID, created.ID, UpdateInput{Status: &status})
	}
	return created, nil
}

// Reorder rewrites priority = index+1 for every id in orderedIds, in one
// transaction — matching stores/stream-sets.ts's reorder exactly (a
// single drag-end event reorders the campaign's whole list, not one
// stream set at a time).
func (s *Service) Reorder(ctx context.Context, orgID, campaignID string, orderedIDs []string) ([]StreamSet, error) {
	if err := s.repo.Reorder(ctx, orgID, campaignID, orderedIDs); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, orgID, campaignID)
}

// resolveFlowNetworks derives each offer-destination flow's NetworkID
// from the offer's own network_id rather than trusting whatever the
// client sent — there is exactly one correct network for a given offer,
// and the flows table's own CHECK constraint requires both ids to be
// consistent. Also confirms OfferID belongs to orgID (§36-TENANCY).
func (s *Service) resolveFlowNetworks(ctx context.Context, orgID string, flows []FlowInput) ([]FlowInput, error) {
	out := make([]FlowInput, len(flows))
	for i, f := range flows {
		if f.Destination.Kind == routing.DestinationOffer {
			networkID, ok, err := s.repo.OfferNetworkID(ctx, orgID, f.Destination.OfferID)
			if err != nil {
				return nil, fmt.Errorf("checking offer: %w", err)
			}
			if !ok {
				return nil, apierror.Validation("invalid stream set", map[string]string{"flows": fmt.Sprintf("offer %q not found in this organization", f.Destination.OfferID)})
			}
			f.Destination.NetworkID = networkID
		}
		out[i] = f
	}
	return out, nil
}

func (s *Service) validateFlows(ctx context.Context, orgID string, flows []FlowInput) string {
	if len(flows) == 0 {
		return "at least one flow is required"
	}
	for _, f := range flows {
		if strings.TrimSpace(f.Name) == "" {
			return "every flow needs a name"
		}
		if f.Weight < 0 {
			return "weight can't be negative"
		}
		switch f.Destination.Kind {
		case routing.DestinationOffer:
			if !idgen.IsValid(f.Destination.OfferID) {
				return "choose an offer"
			}
		case routing.DestinationRedirect:
			if !isValidURL(f.Destination.URL) {
				return "enter a valid redirect URL"
			}
		default:
			return fmt.Sprintf("destination kind must be %q or %q", routing.DestinationOffer, routing.DestinationRedirect)
		}
	}
	return ""
}

// validateFilterNode mirrors stream-set-schema.ts's superRefine rules —
// the same "never trust the client as the source of truth" reasoning
// lib/filters.ts's own checkRE2Compatible comment gives for why its own
// heuristic check isn't the real enforcement (CLAUDE.md #8: RE2
// validated at save time, here via Go's stdlib regexp.Compile, which IS
// RE2 — no separate heuristic needed server-side, just the real compile).
func validateFilterNode(node FilterNode, isRoot bool) string {
	switch node.Kind {
	case NodeGroup:
		if isRoot && node.Joiner == "" {
			return "root filter must be a group"
		}
		if node.Joiner != routing.JoinAND && node.Joiner != routing.JoinOR {
			return "group joiner must be AND or OR"
		}
		for _, child := range node.Children {
			if err := validateFilterNode(child, false); err != "" {
				return err
			}
		}
		return ""
	case NodeCondition:
		if isRoot {
			return "root filter must be a group, not a condition"
		}
		if !validFields[node.Field] {
			return fmt.Sprintf("unknown filter field %q", node.Field)
		}
		if !validOperators[node.Operator] {
			return fmt.Sprintf("unknown filter operator %q", node.Operator)
		}
		if operatorsWithoutValue[node.Operator] {
			return ""
		}
		if node.Field == routing.FieldCountry && node.Value != "" {
			if err := validateCountryValue(node.Value); err != "" {
				return err
			}
		}
		if node.Operator == routing.OpMatches {
			if _, err := regexp.Compile(node.Value); err != nil {
				return "MATCHES value must be a valid RE2 regular expression"
			}
		}
		if node.Operator == routing.OpBetween {
			if node.Value == "" || node.ValueTo == "" {
				return "BETWEEN requires both range bounds"
			}
			return ""
		}
		if node.Value == "" {
			return "condition value is required"
		}
		return ""
	default:
		return fmt.Sprintf("unknown filter node type %q", node.Kind)
	}
}

func validateCountryValue(value string) string {
	tokens := strings.Split(value, ",")
	for _, raw := range tokens {
		t := strings.ToUpper(strings.TrimSpace(raw))
		if t == "" {
			continue
		}
		if t == "UK" {
			return `"UK" is not an ISO-3166 code — use "GB" for United Kingdom`
		}
		if !isoAlpha2.MatchString(t) {
			return fmt.Sprintf("%q is not a 2-letter ISO-3166 country code (e.g. US, GB, DE)", t)
		}
	}
	return ""
}

func isValidURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
