package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// TOTPHandler serves real per-user two-factor authentication (§30
// Security). Enroll/Confirm are mounted PUBLIC (RegisterPublic, no
// tenantMiddleware) on purpose: a brand-new invitee joining an org whose
// two_factor_required policy is already on must be able to complete
// enrollment before a session exists at all — resolveSubject below
// handles both that case (a pendingToken from LoginOutcome) and the
// ordinary self-serve case (an existing session cookie) itself, so
// callers never need two different endpoints for what is, from the
// user's perspective, "set up my 2FA."
type TOTPHandler struct {
	svc           *Service
	logger        *slog.Logger
	secureCookies bool
}

func NewTOTPHandler(svc *Service, logger *slog.Logger, secureCookies bool) *TOTPHandler {
	return &TOTPHandler{svc: svc, logger: logger, secureCookies: secureCookies}
}

// RegisterPublic mounts /auth/login/totp (exchanging a pending login for
// a real session) and /auth/2fa/{enroll,confirm} (dual-context — see
// resolveSubject).
func (h *TOTPHandler) RegisterPublic(r chi.Router) {
	r.Post("/auth/login/totp", h.loginTOTP)
	r.Post("/auth/2fa/enroll", h.enroll)
	r.Post("/auth/2fa/confirm", h.confirm)
}

// RegisterAuthenticated mounts the two actions that only ever make sense
// for an already-signed-in user — caller must mount r under the tenant
// middleware.
func (h *TOTPHandler) RegisterAuthenticated(r chi.Router) {
	r.Get("/auth/2fa/status", h.status)
	r.Post("/auth/2fa/disable", h.disable)
}

// resolveSubject identifies who a dual-context 2FA request is for: an
// existing session cookie (self-serve opt-in) takes priority; absent
// that, pendingToken (from a LoginOutcome) resolves the still-anonymous
// user mid-login.
func (h *TOTPHandler) resolveSubject(r *http.Request, pendingToken string) (userID, orgID string, err error) {
	if cookie, cerr := r.Cookie(tenant.CookieName); cerr == nil && cookie.Value != "" {
		orgID, userID, err = h.svc.ResolveSession(r.Context(), cookie.Value)
		return userID, orgID, err
	}
	if pendingToken == "" {
		return "", "", apierror.Unauthorized("sign in first, or provide a pendingToken")
	}
	return h.svc.ResolvePendingLoginSubject(r.Context(), pendingToken)
}

type enrollTOTPRequest struct {
	PendingToken string `json:"pendingToken"`
}

type enrollTOTPResponse struct {
	Secret        string   `json:"secret"`
	QRCodeDataURI string   `json:"qrCodeDataUri"`
	BackupCodes   []string `json:"backupCodes"`
}

func (h *TOTPHandler) enroll(w http.ResponseWriter, r *http.Request) {
	var body enrollTOTPRequest
	_ = json.NewDecoder(r.Body).Decode(&body) // optional body — the self-serve path sends none

	userID, orgID, err := h.resolveSubject(r, body.PendingToken)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	who, err := h.svc.Me(r.Context(), userID, orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	enrollment, err := h.svc.EnrollTOTP(r.Context(), userID, who.User.Email)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, enrollTOTPResponse{
		Secret:        enrollment.Secret,
		QRCodeDataURI: enrollment.QRCodeDataURI,
		BackupCodes:   enrollment.BackupCodes,
	})
}

type confirmTOTPRequest struct {
	PendingToken string `json:"pendingToken"`
	Code         string `json:"code"`
}

// confirm has two real outcomes depending on how it was reached:
//   - Via pendingToken from a forced-enrollment LoginOutcome
//     (RequiresTOTPEnrollment): confirming ALSO completes the login that
//     triggered enrollment — the response is a real session, exactly
//     like /auth/login itself, so the frontend never hits a "confirmed,
//     now please log in again" dead end.
//   - Via an existing session (self-serve opt-in): confirming just
//     enables 2FA for the already-logged-in user — the response is a
//     plain {enabled: true}.
func (h *TOTPHandler) confirm(w http.ResponseWriter, r *http.Request) {
	var body confirmTOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	if body.PendingToken != "" {
		res, err := h.svc.CompleteEnrollmentLogin(r.Context(), body.PendingToken, body.Code)
		if err != nil {
			apierror.Write(w, h.logger, err)
			return
		}
		setSessionCookie(w, h.secureCookies, res.Token, res.ExpiresAt)
		writeJSON(w, http.StatusOK, toAuthResponse(res))
		return
	}

	cookie, cerr := r.Cookie(tenant.CookieName)
	if cerr != nil || cookie.Value == "" {
		apierror.Write(w, h.logger, apierror.Unauthorized("sign in first, or provide a pendingToken"))
		return
	}
	_, userID, err := h.svc.ResolveSession(r.Context(), cookie.Value)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	if err := h.svc.ConfirmTOTP(r.Context(), userID, body.Code); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true})
}

type loginTOTPRequest struct {
	PendingToken string `json:"pendingToken"`
	Code         string `json:"code"`
}

func (h *TOTPHandler) loginTOTP(w http.ResponseWriter, r *http.Request) {
	var body loginTOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	res, err := h.svc.VerifyPendingLogin(r.Context(), body.PendingToken, body.Code)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	setSessionCookie(w, h.secureCookies, res.Token, res.ExpiresAt)
	writeJSON(w, http.StatusOK, toAuthResponse(res))
}

func (h *TOTPHandler) status(w http.ResponseWriter, r *http.Request) {
	userID, _ := tenant.UserID(r.Context())
	enabled, err := h.svc.TOTPStatus(r.Context(), userID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled})
}

type disableTOTPRequest struct {
	Password string `json:"password"`
}

func (h *TOTPHandler) disable(w http.ResponseWriter, r *http.Request) {
	userID, _ := tenant.UserID(r.Context())
	orgID, _ := tenant.OrgID(r.Context())
	var body disableTOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	if err := h.svc.DisableTOTP(r.Context(), userID, orgID, body.Password); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
