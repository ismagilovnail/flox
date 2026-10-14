package apikey

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

const (
	nameMinLen = 2
	nameMaxLen = 60

	// RotationGrace is how long a rotated key keeps working, so an
	// integration can switch to the new secret without downtime (§54).
	RotationGrace = 24 * time.Hour
)

type Service struct {
	repo   *Repository
	logger *slog.Logger
}

func NewService(repo *Repository, logger *slog.Logger) *Service {
	return &Service{repo: repo, logger: logger}
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

// Rotate issues a new secret for key id: a new key with the same name and
// scope, its full secret returned once, as on create. The old key keeps
// authenticating for RotationGrace, then stops (see Repository.Rotate).
func (s *Service) Rotate(ctx context.Context, orgID, id, userID string) (CreateResult, error) {
	fullKey, prefix, hash, err := newKey()
	if err != nil {
		return CreateResult{}, err
	}
	created, err := s.repo.Rotate(ctx, orgID, id, userID, prefix, hash, RotationGrace)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{ApiKey: created, FullKey: fullKey}, nil
}

// Authenticate validates a raw API key (an Authorization: Bearer token)
// and returns the organization it belongs to, its creator, and its scope
// — implements tenant.APIKeyAuthenticator, called from tenant.NewMiddleware
// on every request carrying a Bearer token instead of a session cookie.
//
// userID is the key's creator (created_by_user_id), returned only for
// audit attribution (e.g. a campaign's created_by_user_id when created
// through this key) — NOT re-checked against the org's memberships the
// way a session's user_id implicitly is on every RequirePermission call.
// A key deliberately outlives its creator's own membership: an
// integration set up by someone who has since left the org shouldn't
// silently break, the same way a Stripe or GitHub API key isn't tied to
// its creator's continued employment. Explicit revocation (this
// package's own Revoke) is the intended control, not membership churn.
// Authorization itself is entirely a function of the key's scope — see
// apps/internal/auth.Service.RequirePermission's scope-tier check.
func (s *Service) Authenticate(ctx context.Context, rawKey string) (tenant.APIKeyIdentity, error) {
	if !strings.HasPrefix(rawKey, keyPrefix) {
		return tenant.APIKeyIdentity{}, apierror.Unauthorized("invalid api key")
	}
	key, err := s.repo.FindByHash(ctx, hashKey(rawKey))
	if err != nil {
		return tenant.APIKeyIdentity{}, err
	}
	if key.RevokedAt != nil {
		return tenant.APIKeyIdentity{}, apierror.Unauthorized("api key revoked")
	}
	if !key.usable(time.Now()) {
		return tenant.APIKeyIdentity{}, apierror.Unauthorized("api key expired: it was rotated, use its replacement")
	}
	// Best-effort: a failed bookkeeping write must never turn into a
	// rejected request over an otherwise-valid key, matching this
	// codebase's existing "a down/failed optional write degrades that
	// feature, not the request" stance (e.g. ratelimit's own fail-open).
	if err := s.repo.UpdateLastUsed(ctx, key.ID); err != nil {
		s.logger.Warn("api key authenticated but last_used_at update failed", "error", err, "key_id", key.ID)
	}
	return tenant.APIKeyIdentity{KeyID: key.ID, OrganizationID: key.OrganizationID, UserID: key.CreatedByUserID, Scope: string(key.Scope)}, nil
}
