package network

import (
	"context"
	"fmt"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/urlsafety"
)

const (
	nameMinLen = 2
	nameMaxLen = 80
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, orgID string) ([]Network, error) {
	return s.repo.List(ctx, orgID)
}

func (s *Service) Get(ctx context.Context, orgID, id string) (Network, error) {
	return s.repo.GetByID(ctx, orgID, id)
}

func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (CreateResult, error) {
	in.Name = strings.TrimSpace(in.Name)
	if fields := validateCreate(in); len(fields) > 0 {
		return CreateResult{}, apierror.Validation("invalid network", fields)
	}
	secret, secretHash, err := newPostbackSecret()
	if err != nil {
		return CreateResult{}, err
	}
	n, err := s.repo.Create(ctx, idgen.New(), orgID, in, secretHash)
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
	if in.PostbackURL != nil && urlsafety.ValidateURL(*in.PostbackURL) != nil {
		fields["postbackUrl"] = "must be a valid absolute URL"
	}
	if in.Status != nil && !in.Status.Valid() {
		fields["status"] = "must be one of active, paused, archived"
	}
	if len(fields) > 0 {
		return Network{}, apierror.Validation("invalid network", fields)
	}

	return s.repo.Update(ctx, orgID, id, in)
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
	}, secretHash)
	if err != nil {
		return Network{}, err
	}
	if source.Status != StatusActive {
		status := source.Status
		return s.repo.Update(ctx, orgID, created.ID, UpdateInput{Status: &status})
	}
	return created, nil
}

func (s *Service) Pause(ctx context.Context, orgID, id string) (Network, error) {
	n, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Network{}, err
	}
	switch n.Status {
	case StatusPaused:
		return n, nil
	case StatusActive:
		status := StatusPaused
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
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
		return n, nil
	case StatusPaused:
		status := StatusActive
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Network{}, apierror.Conflict(fmt.Sprintf("cannot activate a network with status %q", n.Status))
	}
}

func validateCreate(in CreateInput) map[string]string {
	fields := map[string]string{}
	if len(in.Name) < nameMinLen || len(in.Name) > nameMaxLen {
		fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
	}
	if urlsafety.ValidateURL(in.PostbackURL) != nil {
		fields["postbackUrl"] = "must be a valid absolute URL"
	}
	return fields
}
