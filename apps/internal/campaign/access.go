package campaign

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// ManageAllPermission lets a member change and reassign every campaign;
// without it campaign.write covers only the campaigns they own (audit
// TEAM-1, migration 00097).
const ManageAllPermission = "campaign.manage_all"

// PermissionChecker is auth.Service.HasPermission.
type PermissionChecker func(ctx context.Context, userID, orgID, permission string) (bool, error)

// Access guards writes to one campaign by its owner.
type Access struct {
	repo   *Repository
	can    PermissionChecker
	logger *slog.Logger
}

func NewAccess(repo *Repository, can PermissionChecker, logger *slog.Logger) *Access {
	return &Access{repo: repo, can: can, logger: logger}
}

// RequireOwned wraps the mutating routes of one campaign — the campaign
// itself, its stream sets, tracking links and spend — after their
// campaign.write check. param names the URL parameter holding the
// campaign's id; a route without it (creating a campaign) passes.
//
// It passes an API key (keys act for the whole organization; only Owners
// and Admins make them), a member with campaign.manage_all, and the
// campaign's owner. Anyone else gets 403 — an unowned campaign included.
// An unknown campaign passes on, for the handler to answer 404 as usual.
func (a *Access) RequireOwned(param string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			id := chi.URLParam(r, param)
			if _, isKey := tenant.APIKeyScope(ctx); isKey || id == "" {
				next.ServeHTTP(w, r)
				return
			}
			orgID, _ := tenant.OrgID(ctx)
			userID, _ := tenant.UserID(ctx)
			all, err := a.can(ctx, userID, orgID, ManageAllPermission)
			if err != nil {
				apierror.Write(w, a.logger, err)
				return
			}
			if all {
				next.ServeHTTP(w, r)
				return
			}
			owner, found, err := a.repo.OwnerOf(ctx, orgID, id)
			if err != nil {
				apierror.Write(w, a.logger, err)
				return
			}
			if found && owner != userID {
				apierror.Write(w, a.logger, apierror.Forbidden("this campaign belongs to another buyer"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
