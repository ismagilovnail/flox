package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/clientip"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// SecurityHandler serves /security — the Settings > Security panel (§30):
// session timeout, IP allowlist, and the org-wide "require 2FA" policy
// toggle (real per-user TOTP enrollment itself is TOTPHandler, in
// totp_handler.go — a different, personal-account concern). Read is open
// to any authenticated member, same reasoning OrganizationHandler already
// documents; writes require settings.write.
type SecurityHandler struct {
	svc      *Service
	logger   *slog.Logger
	clientIP *clientip.Resolver
}

func NewSecurityHandler(svc *Service, logger *slog.Logger, clientIP *clientip.Resolver) *SecurityHandler {
	return &SecurityHandler{svc: svc, logger: logger, clientIP: clientIP}
}

func (h *SecurityHandler) Register(r chi.Router) {
	requireWrite := h.svc.RequirePermission("settings.write", h.logger)
	r.Get("/security", h.get)
	r.With(requireWrite).Patch("/security", h.update)
}

type securitySettingsResponse struct {
	SessionTimeoutMinutes int      `json:"sessionTimeoutMinutes"`
	IPAllowlist           []string `json:"ipAllowlist"`
	TwoFactorRequired     bool     `json:"twoFactorRequired"`
}

func toSecuritySettingsResponse(s SecuritySettings) securitySettingsResponse {
	return securitySettingsResponse{
		SessionTimeoutMinutes: s.SessionTimeoutMinutes,
		IPAllowlist:           s.IPAllowlist,
		TwoFactorRequired:     s.TwoFactorRequired,
	}
}

func (h *SecurityHandler) get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	s, err := h.svc.GetSecuritySettings(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, toSecuritySettingsResponse(s))
}

type updateSecurityRequest struct {
	SessionTimeoutMinutes *int     `json:"sessionTimeoutMinutes"`
	IPAllowlist           []string `json:"ipAllowlist"`
	TwoFactorRequired     *bool    `json:"twoFactorRequired"`
}

func (h *SecurityHandler) update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	var req updateSecurityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	s, err := h.svc.UpdateSecuritySettings(r.Context(), orgID, userID, h.clientIP.String(r), UpdateSecuritySettingsInput{
		SessionTimeoutMinutes: req.SessionTimeoutMinutes,
		IPAllowlist:           req.IPAllowlist,
		TwoFactorRequired:     req.TwoFactorRequired,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, toSecuritySettingsResponse(s))
}
