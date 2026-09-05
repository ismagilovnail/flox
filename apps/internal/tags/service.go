package tags

import (
	"context"
	"fmt"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

const (
	nameMinLen = 1
	nameMaxLen = 40
	// §30.6's bulk apply is a multi-select over one already-loaded list
	// page — capped generously rather than left unbounded, same call
	// made for the Content Gallery's own bulk endpoints.
	bulkMax = 500
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, orgID string) ([]Tag, error) {
	return s.repo.List(ctx, orgID)
}

func validateNameColor(name, color string) (string, error) {
	name = strings.TrimSpace(name)
	if len(name) < nameMinLen {
		return "", apierror.Validation("name is required", map[string]string{"name": "required"})
	}
	if len(name) > nameMaxLen {
		return "", apierror.Validation(fmt.Sprintf("name must be at most %d characters", nameMaxLen), map[string]string{"name": "too long"})
	}
	if !Colors[color] {
		return "", apierror.Validation("invalid color", map[string]string{"color": "invalid"})
	}
	return name, nil
}

func (s *Service) Create(ctx context.Context, orgID string, in CreateTagInput) (Tag, error) {
	name, err := validateNameColor(in.Name, in.Color)
	if err != nil {
		return Tag{}, err
	}
	return s.repo.Create(ctx, orgID, CreateTagInput{Name: name, Color: in.Color})
}

func (s *Service) Update(ctx context.Context, orgID, id string, in UpdateTagInput) (Tag, error) {
	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		if len(trimmed) < nameMinLen {
			return Tag{}, apierror.Validation("name is required", map[string]string{"name": "required"})
		}
		if len(trimmed) > nameMaxLen {
			return Tag{}, apierror.Validation(fmt.Sprintf("name must be at most %d characters", nameMaxLen), map[string]string{"name": "too long"})
		}
		in.Name = &trimmed
	}
	if in.Color != nil && !Colors[*in.Color] {
		return Tag{}, apierror.Validation("invalid color", map[string]string{"color": "invalid"})
	}
	return s.repo.Update(ctx, orgID, id, in)
}

func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	return s.repo.Delete(ctx, orgID, id)
}

func (s *Service) ListAssignments(ctx context.Context, orgID string, entityType EntityType) ([]Assignment, error) {
	if !entityType.valid() {
		return nil, apierror.Validation("invalid entityType", map[string]string{"entityType": "invalid"})
	}
	return s.repo.ListAssignments(ctx, orgID, entityType)
}

// validateOwnedTags rejects a request naming a tag_id that isn't a real
// tag owned by orgID — see Repository.ExistingTagIDs' own doc comment
// for why this matters even though it isn't a data-leak risk.
func (s *Service) validateOwnedTags(ctx context.Context, orgID string, tagIDs []string) error {
	if len(tagIDs) == 0 {
		return nil
	}
	owned, err := s.repo.ExistingTagIDs(ctx, orgID, tagIDs)
	if err != nil {
		return err
	}
	for _, id := range tagIDs {
		if !owned[id] {
			return apierror.Validation("unknown tag id", map[string]string{"tagIds": id})
		}
	}
	return nil
}

func (s *Service) SetEntityTags(ctx context.Context, orgID string, in SetEntityTagsInput) error {
	if !in.EntityType.valid() {
		return apierror.Validation("invalid entityType", map[string]string{"entityType": "invalid"})
	}
	if strings.TrimSpace(in.EntityID) == "" {
		return apierror.Validation("entityId is required", map[string]string{"entityId": "required"})
	}
	exists, err := s.repo.EntityExists(ctx, orgID, in.EntityType, in.EntityID)
	if err != nil {
		return err
	}
	if !exists {
		return apierror.NotFound("entity not found")
	}
	if err := s.validateOwnedTags(ctx, orgID, in.TagIDs); err != nil {
		return err
	}
	return s.repo.SetEntityTags(ctx, orgID, in.EntityType, in.EntityID, in.TagIDs)
}

func (s *Service) BulkEdit(ctx context.Context, orgID string, in BulkEditInput) error {
	if !in.EntityType.valid() {
		return apierror.Validation("invalid entityType", map[string]string{"entityType": "invalid"})
	}
	if len(in.EntityIDs) == 0 {
		return apierror.Validation("entityIds is required", map[string]string{"entityIds": "required"})
	}
	if len(in.EntityIDs) > bulkMax {
		return apierror.Validation(fmt.Sprintf("at most %d entities per bulk edit", bulkMax), map[string]string{"entityIds": "too many"})
	}
	if len(in.ToAdd) == 0 && len(in.ToRemove) == 0 {
		return nil
	}
	if err := s.validateOwnedTags(ctx, orgID, in.ToAdd); err != nil {
		return err
	}
	if err := s.validateOwnedTags(ctx, orgID, in.ToRemove); err != nil {
		return err
	}
	for _, entityID := range in.EntityIDs {
		exists, err := s.repo.EntityExists(ctx, orgID, in.EntityType, entityID)
		if err != nil {
			return err
		}
		if !exists {
			return apierror.NotFound("entity not found: " + entityID)
		}
	}
	return s.repo.BulkEdit(ctx, orgID, in.EntityType, in.EntityIDs, in.ToAdd, in.ToRemove)
}
