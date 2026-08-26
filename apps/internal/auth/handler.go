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

// Handler serves the unauthenticated /auth/* routes (signup, login,
// accept-invite, invite preview) plus the two authenticated ones that
// don't belong to the team domain (logout, me). Team/membership endpoints
// are TeamHandler, in team_handler.go — a separate router group requiring
// team.read/team.write, mounted under the tenant middleware.
type Handler struct {
	svc           *Service
	logger        *slog.Logger
	secureCookies bool
}

// secureCookies should be true in any environment served over HTTPS.
// False in local dev: a Secure cookie is dropped by browsers over plain
// http, and this project's dev stack runs apps/api on http://localhost
// with no TLS termination in front of it.
func NewHandler(svc *Service, logger *slog.Logger, secureCookies bool) *Handler {
	return &Handler{svc: svc, logger: logger, secureCookies: secureCookies}
}

// RegisterPublic mounts the routes that must NOT sit behind the tenant
// (session-required) middleware — there is no session yet when a client
// calls any of these.
func (h *Handler) RegisterPublic(r chi.Router) {
	r.Post("/auth/signup", h.signup)
	r.Post("/auth/login", h.login)
	r.Get("/auth/invites/{token}", h.previewInvite)
	r.Post("/auth/accept-invite", h.acceptInvite)
}

// RegisterAuthenticated mounts routes that need a valid session but aren't
// team-permission-gated — caller must mount r under the tenant middleware.
func (h *Handler) RegisterAuthenticated(r chi.Router) {
	r.Post("/auth/logout", h.logout)
	r.Get("/auth/me", h.me)
}

type userResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type organizationResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type authResponse struct {
	User         userResponse         `json:"user"`
	Organization organizationResponse `json:"organization"`
	Role         string               `json:"role"`
}

func toAuthResponse(res Result) authResponse {
	return authResponse{
		User:         userResponse{ID: res.User.ID, Name: res.User.Name, Email: res.User.Email},
		Organization: organizationResponse{ID: res.Organization.ID, Name: res.Organization.Name},
		Role:         res.RoleKey,
	}
}

func (h *Handler) setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     tenant.CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   h.secureCookies,
		// Lax, not None: apps/web and apps/api are different origins but
		// the same "site" (SameSite is defined by scheme+registrable
		// domain, not port) in both local dev (localhost:3000/:8080) and
		// any same-domain production deployment — Lax already covers
		// that. A genuinely cross-site deployment (different registrable
		// domains) would need SameSite=None+Secure instead; noted as a
		// known limitation rather than built speculatively.
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     tenant.CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) signup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrganizationName string `json:"organizationName"`
		Name             string `json:"name"`
		Email            string `json:"email"`
		Password         string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid request body", nil))
		return
	}

	res, err := h.svc.Signup(r.Context(), SignupInput{
		OrganizationName: body.OrganizationName,
		Name:             body.Name,
		Email:            body.Email,
		Password:         body.Password,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}

	h.setSessionCookie(w, res.Token, res.ExpiresAt)
	writeJSON(w, http.StatusCreated, toAuthResponse(res))
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email          string `json:"email"`
		Password       string `json:"password"`
		OrganizationID string `json:"organizationId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid request body", nil))
		return
	}

	res, err := h.svc.Login(r.Context(), LoginInput{
		Email:          body.Email,
		Password:       body.Password,
		OrganizationID: body.OrganizationID,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}

	h.setSessionCookie(w, res.Token, res.ExpiresAt)
	writeJSON(w, http.StatusOK, toAuthResponse(res))
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(tenant.CookieName); err == nil && cookie.Value != "" {
		if err := h.svc.Logout(r.Context(), cookie.Value); err != nil {
			apierror.Write(w, h.logger, err)
			return
		}
	}
	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())

	who, err := h.svc.Me(r.Context(), userID, orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		User         userResponse         `json:"user"`
		Organization organizationResponse `json:"organization"`
		Role         string               `json:"role"`
		Permissions  []string             `json:"permissions"`
	}{
		User:         userResponse{ID: who.User.ID, Name: who.User.Name, Email: who.User.Email},
		Organization: organizationResponse{ID: who.Organization.ID, Name: who.Organization.Name},
		Role:         who.RoleKey,
		Permissions:  who.Permissions,
	})
}

func (h *Handler) previewInvite(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	preview, err := h.svc.PreviewInvite(r.Context(), token)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		OrganizationName string `json:"organizationName"`
		Email            string `json:"email"`
		Role             string `json:"role"`
	}{preview.OrganizationName, preview.Email, preview.RoleKey})
}

func (h *Handler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid request body", nil))
		return
	}

	res, err := h.svc.AcceptInvite(r.Context(), AcceptInviteInput{
		Token:    body.Token,
		Name:     body.Name,
		Password: body.Password,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}

	h.setSessionCookie(w, res.Token, res.ExpiresAt)
	writeJSON(w, http.StatusOK, toAuthResponse(res))
}
