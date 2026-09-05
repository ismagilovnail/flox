package apikey

import (
	"context"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

const (
	nameMinLen = 2
	nameMaxLen = 60
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, orgID string) ([]ApiKey, error) {
	return s.repo.List(ctx, orgID)
}

func (s *Service) Create(ctx context.Context, orgID, createdByUserID string, in CreateInput) (CreateResult, error) {
	name := strings.TrimSpace(in.Name)
	if len(name) < nameMinLen {
		return CreateResult{}, apierror.Validation("name must be at least 2 characters", map[string]string{"name": "too short"})
	}
	if len(name) > nameMaxLen {
		return CreateResult{}, apierror.Validation("name is too long", map[string]string{"name": "too long"})
	}
	if !in.Scope.valid() {
		return CreateResult{}, apierror.Validation("invalid scope", map[string]string{"scope": "invalid"})
	}

	fullKey, prefix, hash, err := newKey()
	if err != nil {
		return CreateResult{}, err
	}
	created, err := s.repo.Create(ctx, orgID, createdByUserID, prefix, hash, CreateInput{Name: name, Scope: in.Scope})
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{ApiKey: created, FullKey: fullKey}, nil
}

func (s *Service) Revoke(ctx context.Context, orgID, id string) error {
	return s.repo.Revoke(ctx, orgID, id)
}
