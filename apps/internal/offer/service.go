package offer

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

const (
	nameMinLen = 2
	nameMaxLen = 100
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, orgID string) ([]Offer, error) {
	return s.repo.List(ctx, orgID)
}

func (s *Service) Get(ctx context.Context, orgID, id string) (Offer, error) {
	return s.repo.GetByID(ctx, orgID, id)
}

func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (Offer, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.Countries = normalizeCountries(in.Countries)
	for i := range in.Links {
		in.Links[i].Label = strings.TrimSpace(in.Links[i].Label)
	}

	if fields := validate(in.Name, in.NetworkID, in.Countries, in.Payout, in.Currency, in.Cap, in.Links); len(fields) > 0 {
		return Offer{}, apierror.Validation("invalid offer", fields)
	}

	belongs, err := s.repo.NetworkBelongsToOrg(ctx, orgID, in.NetworkID)
	if err != nil {
		return Offer{}, fmt.Errorf("checking network: %w", err)
	}
	if !belongs {
		return Offer{}, apierror.Validation("invalid offer", map[string]string{"networkId": "no such network in this organization"})
	}

	return s.repo.Create(ctx, idgen.New(), orgID, in)
}

func (s *Service) Update(ctx context.Context, orgID, id string, in UpdateInput) (Offer, error) {
	fields := map[string]string{}

	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		in.Name = &trimmed
		if len(trimmed) < nameMinLen || len(trimmed) > nameMaxLen {
			fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
		}
	}
	if in.Countries != nil {
		normalized := normalizeCountries(*in.Countries)
		in.Countries = &normalized
		if len(normalized) == 0 {
			fields["countries"] = "select at least one country"
		}
	}
	if in.Payout != nil && *in.Payout <= 0 {
		fields["payout"] = "must be greater than 0"
	}
	if in.Currency != nil {
		upper := strings.ToUpper(strings.TrimSpace(*in.Currency))
		in.Currency = &upper
		if len(upper) != 3 {
			fields["currency"] = "must be a 3-letter currency code"
		}
	}
	if in.Cap != nil && in.Cap.Value != nil && *in.Cap.Value < 0 {
		fields["cap"] = "must not be negative"
	}
	if in.Status != nil && !in.Status.Valid() {
		fields["status"] = "must be one of active, paused, archived"
	}
	if in.Links != nil {
		links := *in.Links
		for i := range links {
			links[i].Label = strings.TrimSpace(links[i].Label)
		}
		if linkFields := validateLinks(links); len(linkFields) > 0 {
			fields["links"] = linkFields
		}
		in.Links = &links
	}
	if len(fields) > 0 {
		return Offer{}, apierror.Validation("invalid offer", fields)
	}

	if in.NetworkID != nil {
		belongs, err := s.repo.NetworkBelongsToOrg(ctx, orgID, *in.NetworkID)
		if err != nil {
			return Offer{}, fmt.Errorf("checking network: %w", err)
		}
		if !belongs {
			return Offer{}, apierror.Validation("invalid offer", map[string]string{"networkId": "no such network in this organization"})
		}
	}

	return s.repo.Update(ctx, orgID, id, in)
}

func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	return s.repo.Delete(ctx, orgID, id)
}

// Duplicate keeps status as-is (same reasoning as network/trafficsource's
// own Duplicate) and copies every link with a fresh id — the frontend
// mock this replaces (stores/offers.ts duplicateOffer) did the same.
func (s *Service) Duplicate(ctx context.Context, orgID, id string) (Offer, error) {
	source, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Offer{}, err
	}

	links := make([]LinkInput, len(source.Links))
	for i, l := range source.Links {
		links[i] = LinkInput{Label: l.Label, URL: l.URL}
	}

	created, err := s.repo.Create(ctx, idgen.New(), orgID, CreateInput{
		NetworkID: source.NetworkID,
		Name:      source.Name + " (Copy)",
		Countries: source.Countries,
		Payout:    source.Payout,
		Currency:  source.Currency,
		Cap:       source.Cap,
		Links:     links,
	})
	if err != nil {
		return Offer{}, err
	}
	if source.Status != StatusActive {
		status := source.Status
		return s.repo.Update(ctx, orgID, created.ID, UpdateInput{Status: &status})
	}
	return created, nil
}

func (s *Service) Pause(ctx context.Context, orgID, id string) (Offer, error) {
	o, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Offer{}, err
	}
	switch o.Status {
	case StatusPaused:
		return o, nil
	case StatusActive:
		status := StatusPaused
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Offer{}, apierror.Conflict(fmt.Sprintf("cannot pause an offer with status %q", o.Status))
	}
}

func (s *Service) Activate(ctx context.Context, orgID, id string) (Offer, error) {
	o, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Offer{}, err
	}
	switch o.Status {
	case StatusActive:
		return o, nil
	case StatusPaused:
		status := StatusActive
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Offer{}, apierror.Conflict(fmt.Sprintf("cannot activate an offer with status %q", o.Status))
	}
}

func normalizeCountries(in []string) []string {
	out := make([]string, 0, len(in))
	for _, c := range in {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}

func validate(name, networkID string, countries []string, payout float64, currency string, cap *int, links []LinkInput) map[string]string {
	fields := map[string]string{}
	if len(name) < nameMinLen || len(name) > nameMaxLen {
		fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
	}
	if !idgen.IsValid(networkID) {
		fields["networkId"] = "required"
	}
	if len(countries) == 0 {
		fields["countries"] = "select at least one country"
	}
	if payout <= 0 {
		fields["payout"] = "must be greater than 0"
	}
	if len(currency) != 3 {
		fields["currency"] = "must be a 3-letter currency code"
	}
	if cap != nil && *cap < 0 {
		fields["cap"] = "must not be negative"
	}
	if linkFields := validateLinks(links); linkFields != "" {
		fields["links"] = linkFields
	}
	return fields
}

func validateLinks(links []LinkInput) string {
	if len(links) == 0 {
		return "add at least one offer link"
	}
	for _, l := range links {
		if l.Label == "" {
			return "every link needs a label"
		}
		if !isValidURL(l.URL) {
			return "every link needs a valid absolute URL"
		}
	}
	return ""
}

func isValidURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
