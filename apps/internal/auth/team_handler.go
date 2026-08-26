package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// TeamHandler serves /team/* — listing and managing this org's
// memberships. Every mutating route additionally requires team.write
// (RequirePermission below), read routes require team.read; the caller
// (apps/api/main.go) mounts this under the tenant middleware, same as
// every other domain's routes.
type TeamHandler struct {
	svc    *Service
	logger *slog.Logger
}

func NewTeamHandler(svc *Service, logger *slog.Logger) *TeamHandler {
	return &TeamHandler{svc: svc, logger: logger}
}

func (h *TeamHandler) Register(r chi.Router) {
	requireRead := h.svc.RequirePermission("team.read", h.logger)
	requireWrite := h.svc.RequirePermission("team.write", h.logger)

	r.With(requireRead).Get("/team/members", h.list)
	r.With(requireRead).Get("/team/activity", h.activity)
	r.With(requireWrite).Post("/team/members/invite", h.invite)
	r.With(requireWrite).Patch("/team/members/{id}", h.update)
	r.With(requireWrite).Post("/team/members/{id}/resend-invite", h.resendInvite)
	r.With(requireWrite).Delete("/team/members/{id}", h.remove)
}

type membershipResponse struct {
	ID           string     `json:"id"`
	UserID       string     `json:"userId"`
	Name         string     `json:"name"`
	Email        string     `json:"email"`
	Role         string     `json:"role"`
	Status       string     `json:"status"`
	InvitedAt    time.Time  `json:"invitedAt"`
	LastActiveAt *time.Time `json:"lastActiveAt"`
}

func toMembershipResponse(m Membership) membershipResponse {
	return membershipResponse{
		ID:           m.ID,
		UserID:       m.UserID,
		Name:         m.UserName,
		Email:        m.UserEmail,
		Role:         m.RoleKey,
		Status:       m.Status,
		InvitedAt:    m.InvitedAt,
		LastActiveAt: m.LastActiveAt,
	}
}

func (h *TeamHandler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	members, err := h.svc.ListMembers(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	out := make([]membershipResponse, len(members))
	for i, m := range members {
		out[i] = toMembershipResponse(m)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *TeamHandler) invite(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	actorID, _ := tenant.UserID(r.Context())

	var body struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid request body", nil))
		return
	}

	inviteURL, err := h.svc.Invite(r.Context(), orgID, actorID, InviteInput{Name: body.Name, Email: body.Email, RoleKey: body.Role})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		InviteURL string `json:"inviteUrl"`
	}{inviteURL})
}

func (h *TeamHandler) update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	actorID, _ := tenant.UserID(r.Context())
	membershipID := chi.URLParam(r, "id")

	var body struct {
		Role   *string `json:"role"`
		Status *string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid request body", nil))
		return
	}

	member, err := h.svc.UpdateMembership(r.Context(), orgID, actorID, membershipID, UpdateMembershipInput{RoleKey: body.Role, Status: body.Status})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, toMembershipResponse(member))
}

func (h *TeamHandler) resendInvite(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	actorID, _ := tenant.UserID(r.Context())
	membershipID := chi.URLParam(r, "id")

	inviteURL, err := h.svc.ResendInvite(r.Context(), orgID, actorID, membershipID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		InviteURL string `json:"inviteUrl"`
	}{inviteURL})
}

func (h *TeamHandler) remove(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	actorID, _ := tenant.UserID(r.Context())
	membershipID := chi.URLParam(r, "id")

	if err := h.svc.RemoveMember(r.Context(), orgID, actorID, membershipID); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type activityResponse struct {
	ID         string    `json:"id"`
	Action     string    `json:"action"`
	EntityType string    `json:"entityType"`
	EntityID   string    `json:"entityId"`
	ActorName  *string   `json:"actorName"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (h *TeamHandler) activity(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	entries, err := h.svc.ListActivity(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	out := make([]activityResponse, len(entries))
	for i, e := range entries {
		out[i] = activityResponse{ID: e.ID, Action: e.Action, EntityType: e.EntityType, EntityID: e.EntityID, ActorName: e.ActorName, CreatedAt: e.CreatedAt}
	}
	writeJSON(w, http.StatusOK, out)
}
