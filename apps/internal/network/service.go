package network

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
	"github.com/ismagilovnail/flox/apps/internal/urlsafety"
)

const (
	nameMinLen = 2
	nameMaxLen = 80
)

// currencyPattern accepts any ISO 4217-shaped code, like the organization's
// currency (internal/auth): a currency without a rate on file is still
// stored, the conversion engine just cannot price it.
var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

const currencyMessage = "must be a 3-letter currency code, e.g. USD"

// normalizeCurrency upper-cases a currency; empty stays empty.
func normalizeCurrency(c string) string { return strings.ToUpper(strings.TrimSpace(c)) }

type Service struct {
	repo *Repository
	// trackerURL is TRACKER_PUBLIC_URL, the base of IncomingPostbackURL.
	trackerURL string
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// WithTrackerURL sets the public tracker URL incoming postback URLs are
// built on; without it Network.IncomingPostbackURL stays empty.
func (s *Service) WithTrackerURL(u string) *Service {
	s.trackerURL = strings.TrimRight(strings.TrimSpace(u), "/")
	return s
}

// withURL fills the computed IncomingPostbackURL on what is returned.
func (s *Service) withURL(n Network, err error) (Network, error) {
	if err == nil && s.trackerURL != "" && n.ID != "" {
		n.IncomingPostbackURL = s.trackerURL + "/postback/" + n.ID
	}
	return n, err
}

// List returns one page of the organization's networks and their total.
func (s *Service) List(ctx context.Context, orgID string, page pagination.Page) ([]Network, int, error) {
	items, err := s.repo.List(ctx, orgID, page)
	if err != nil {
		return nil, 0, err
	}
	for i := range items {
		items[i], _ = s.withURL(items[i], nil)
	}
	total, err := s.repo.Count(ctx, orgID)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *Service) Get(ctx context.Context, orgID, id string) (Network, error) {
	return s.withURL(s.repo.GetByID(ctx, orgID, id))
}

// Diagnostics is what the network's postbacks say about its setup.
func (s *Service) Diagnostics(ctx context.Context, orgID, id string) (Diagnostics, error) {
	return s.repo.Diagnostics(ctx, orgID, id)
}

func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (CreateResult, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.PostbackURL = strings.TrimSpace(in.PostbackURL)
	in.Currency = normalizeCurrency(in.Currency)
	in.TemplateKey = strings.TrimSpace(in.TemplateKey)
	if in.Currency == "" {
		in.Currency = "USD"
	}
	if fields := validateCreate(in); len(fields) > 0 {
		return CreateResult{}, apierror.Validation("invalid network", fields)
	}
	secret, secretHash, err := newPostbackSecret()
	if err != nil {
		return CreateResult{}, err
	}
	n, err := s.withURL(s.repo.Create(ctx, idgen.New(), orgID, in, secretHash))
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Network: n, PostbackSecret: secret}, nil
}

// RegeneratePostbackSecret issues a fresh secret and invalidates the
// previous one immediately — for when an operator suspects theirs leaked,
// or simply lost it (it's shown once and never stored recoverably).
func (s *Service) RegeneratePostbackSecret(ctx context.Context, orgID, id string) (string, error) {
	secret, secretHash, err := newPostbackSecret()
	if err != nil {
		return "", err
	}
	if err := s.repo.RegenerateSecret(ctx, orgID, id, secretHash); err != nil {
		return "", err
	}
	return secret, nil
}

func (s *Service) Update(ctx context.Context, orgID, id string, in UpdateInput) (Network, error) {
	fields := map[string]string{}

	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		in.Name = &trimmed
		if len(trimmed) < nameMinLen || len(trimmed) > nameMaxLen {
			fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
		}
	}
	if in.PostbackURL != nil {
		trimmed := strings.TrimSpace(*in.PostbackURL)
		in.PostbackURL = &trimmed
		if trimmed != "" && urlsafety.ValidateURL(trimmed) != nil {
			fields["postbackUrl"] = "must be a valid absolute URL"
		}
	}
	if in.Currency != nil {
		c := normalizeCurrency(*in.Currency)
		in.Currency = &c
		if !currencyPattern.MatchString(c) {
			fields["currency"] = currencyMessage
		}
	}
	if in.Status != nil && !in.Status.Valid() {
		fields["status"] = "must be one of active, paused, archived"
	}
	if len(fields) > 0 {
		return Network{}, apierror.Validation("invalid network", fields)
	}

	return s.withURL(s.repo.Update(ctx, orgID, id, in))
}

func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	return s.repo.Delete(ctx, orgID, id)
}

// Duplicate keeps status as-is, same reasoning as
// trafficsource.Service.Duplicate: no draft-equivalent status exists to
// reset to, and the mock store this replaced never reset it either. Gets
// its own fresh postback secret — never the source network's — so two
// networks never end up sharing one credential; the duplicate's secret
// is not returned here (Network never carries it), matching Update/
// List/Get: an operator who needs it calls RegeneratePostbackSecret.
func (s *Service) Duplicate(ctx context.Context, orgID, id string) (Network, error) {
	source, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Network{}, err
	}

	_, secretHash, err := newPostbackSecret()
	if err != nil {
		return Network{}, err
	}
	created, err := s.repo.Create(ctx, idgen.New(), orgID, CreateInput{
		Name:             source.Name + " (Copy)",
		PostbackURL:      source.PostbackURL,
		AcceptDuplicates: source.AcceptDuplicates,
		Currency:         source.Currency,
		TemplateKey:      source.TemplateKey,
	}, secretHash)
	if err != nil {
		return Network{}, err
	}
	if source.Status != StatusActive {
		status := source.Status
		return s.withURL(s.repo.Update(ctx, orgID, created.ID, UpdateInput{Status: &status}))
	}
	return s.withURL(created, nil)
}

func (s *Service) Pause(ctx context.Context, orgID, id string) (Network, error) {
	n, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Network{}, err
	}
	switch n.Status {
	case StatusPaused:
		return s.withURL(n, nil)
	case StatusActive:
		status := StatusPaused
		return s.withURL(s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status}))
	default:
		return Network{}, apierror.Conflict(fmt.Sprintf("cannot pause a network with status %q", n.Status))
	}
}

func (s *Service) Activate(ctx context.Context, orgID, id string) (Network, error) {
	n, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Network{}, err
	}
	switch n.Status {
	case StatusActive:
		return s.withURL(n, nil)
	case StatusPaused:
		status := StatusActive
		return s.withURL(s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status}))
	default:
		return Network{}, apierror.Conflict(fmt.Sprintf("cannot activate a network with status %q", n.Status))
	}
}

func validateCreate(in CreateInput) map[string]string {
	fields := map[string]string{}
	if len(in.Name) < nameMinLen || len(in.Name) > nameMaxLen {
		fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
	}
	// The outgoing postback URL is optional (audit 2026-10-15, UX-2): many
	// networks need nothing back. Empty means no outgoing postback.
	if in.PostbackURL != "" && urlsafety.ValidateURL(in.PostbackURL) != nil {
		fields["postbackUrl"] = "must be a valid absolute URL"
	}
	if !currencyPattern.MatchString(in.Currency) {
		fields["currency"] = currencyMessage
	}
	return fields
}
