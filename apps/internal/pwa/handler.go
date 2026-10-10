package pwa

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

type Handler struct {
	svc *Service
	// requireWrite is the RBAC middleware every mutating route below is
	// wrapped in (Register). Read routes carry no permission requirement
	// beyond tenantMiddleware — same split custommetrics/reportpresets/
	// referral already use.
	requireWrite func(http.Handler) http.Handler
	logger       *slog.Logger
}

func NewHandler(svc *Service, requireWrite func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, logger: logger}
}

// Register mounts every /pwas route onto r. The caller applies
// tenant.Middleware first — same contract as every other handler.
func (h *Handler) Register(r chi.Router) {
	r.Get("/", h.list)
	r.With(h.requireWrite).Post("/", h.create)
	r.Get("/{id}", h.get)
	r.With(h.requireWrite).Patch("/{id}", h.update)
	r.With(h.requireWrite).Delete("/{id}", h.delete)
	r.With(h.requireWrite).Post("/{id}/duplicate", h.duplicate)
	r.With(h.requireWrite).Post("/{id}/pause", h.pause)
	r.With(h.requireWrite).Post("/{id}/activate", h.activate)
	r.With(h.requireWrite).Post("/{id}/test-link", h.testLink)
	r.Get("/{id}/comments", h.listComments)
	r.With(h.requireWrite).Put("/{id}/comments", h.replaceComments)
}

func (h *Handler) listComments(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	comments, err := h.svc.ListComments(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": comments})
}

// replaceComments answers PUT /pwas/{id}/comments — the whole list is
// swapped, which is what the constructor form sends on save.
func (h *Handler) replaceComments(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req struct {
		Comments []CommentInput `json:"comments"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	comments, err := h.svc.ReplaceComments(r.Context(), orgID, chi.URLParam(r, "id"), req.Comments)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": comments})
}

// testLink answers POST /pwas/{id}/test-link — {"url": ...}, a signed
// apps/tracker install-page link for the constructor's "open test page"
// (Service.TestLink). A POST because each call mints a new link.
func (h *Handler) testLink(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	link, err := h.svc.TestLink(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": link})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	page, err := pagination.FromRequest(r)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	pwas, total, err := h.svc.List(r.Context(), orgID, page)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pwas": pwas, "total": total})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	p, err := h.svc.Get(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type createRequest struct {
	Name                string   `json:"name"`
	ShortName           string   `json:"shortName"`
	ThemeColor          string   `json:"themeColor"`
	BackgroundColor     string   `json:"backgroundColor"`
	ThemeAuto           bool     `json:"themeAuto"`
	ShowHeader          *bool    `json:"showHeader"`
	ShowBottomNav       *bool    `json:"showBottomNav"`
	HeaderTitle         string   `json:"headerTitle"`
	HeaderLogoURL       string   `json:"headerLogoUrl"`
	IconURL             string   `json:"iconUrl"`
	StartURL            string   `json:"startUrl"`
	BounceInAppWebview  bool     `json:"bounceInAppWebview"`
	Developer           string   `json:"developer"`
	Description         string   `json:"description"`
	Rating              *float64 `json:"rating"`
	RatingCount         *int     `json:"ratingCount"`
	DownloadsLabel      string   `json:"downloadsLabel"`
	SizeMB              *float64 `json:"sizeMb"`
	Screenshots         []string `json:"screenshots"`
	WhatsNew            string   `json:"whatsNew"`
	RatingStars5        *float64 `json:"ratingStars5"`
	RatingStars4        *float64 `json:"ratingStars4"`
	RatingStars3        *float64 `json:"ratingStars3"`
	RatingStars2        *float64 `json:"ratingStars2"`
	RatingStars1        *float64 `json:"ratingStars1"`
	Version             string   `json:"version"`
	AgeRating           string   `json:"ageRating"`
	Category            string   `json:"category"`
	InteractiveElements string   `json:"interactiveElements"`
	ContainsAds         bool     `json:"containsAds"`
	InAppPurchases      bool     `json:"inAppPurchases"`
	UpdatedDate         *string  `json:"updatedDate"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	p, err := h.svc.Create(r.Context(), orgID, CreateInput{
		Name:                req.Name,
		ShortName:           req.ShortName,
		ThemeColor:          req.ThemeColor,
		BackgroundColor:     req.BackgroundColor,
		ThemeAuto:           req.ThemeAuto,
		ShowHeader:          defaultOn(req.ShowHeader),
		ShowBottomNav:       defaultOn(req.ShowBottomNav),
		HeaderTitle:         req.HeaderTitle,
		HeaderLogoURL:       req.HeaderLogoURL,
		IconURL:             req.IconURL,
		StartURL:            req.StartURL,
		BounceInAppWebview:  req.BounceInAppWebview,
		Developer:           req.Developer,
		Description:         req.Description,
		Rating:              req.Rating,
		RatingCount:         req.RatingCount,
		DownloadsLabel:      req.DownloadsLabel,
		SizeMB:              req.SizeMB,
		Screenshots:         req.Screenshots,
		WhatsNew:            req.WhatsNew,
		RatingStars5:        req.RatingStars5,
		RatingStars4:        req.RatingStars4,
		RatingStars3:        req.RatingStars3,
		RatingStars2:        req.RatingStars2,
		RatingStars1:        req.RatingStars1,
		Version:             req.Version,
		AgeRating:           req.AgeRating,
		Category:            req.Category,
		InteractiveElements: req.InteractiveElements,
		ContainsAds:         req.ContainsAds,
		InAppPurchases:      req.InAppPurchases,
		UpdatedDate:         req.UpdatedDate,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

type updateRequest struct {
	Name                *string   `json:"name"`
	ShortName           *string   `json:"shortName"`
	ThemeColor          *string   `json:"themeColor"`
	BackgroundColor     *string   `json:"backgroundColor"`
	ThemeAuto           *bool     `json:"themeAuto"`
	ShowHeader          *bool     `json:"showHeader"`
	ShowBottomNav       *bool     `json:"showBottomNav"`
	HeaderTitle         *string   `json:"headerTitle"`
	HeaderLogoURL       *string   `json:"headerLogoUrl"`
	IconURL             *string   `json:"iconUrl"`
	StartURL            *string   `json:"startUrl"`
	BounceInAppWebview  *bool     `json:"bounceInAppWebview"`
	Developer           *string   `json:"developer"`
	Description         *string   `json:"description"`
	Rating              *float64  `json:"rating"`
	RatingCount         *int      `json:"ratingCount"`
	DownloadsLabel      *string   `json:"downloadsLabel"`
	SizeMB              *float64  `json:"sizeMb"`
	Screenshots         *[]string `json:"screenshots"`
	WhatsNew            *string   `json:"whatsNew"`
	RatingStars5        *float64  `json:"ratingStars5"`
	RatingStars4        *float64  `json:"ratingStars4"`
	RatingStars3        *float64  `json:"ratingStars3"`
	RatingStars2        *float64  `json:"ratingStars2"`
	RatingStars1        *float64  `json:"ratingStars1"`
	Version             *string   `json:"version"`
	AgeRating           *string   `json:"ageRating"`
	Category            *string   `json:"category"`
	InteractiveElements *string   `json:"interactiveElements"`
	ContainsAds         *bool     `json:"containsAds"`
	InAppPurchases      *bool     `json:"inAppPurchases"`
	UpdatedDate         *string   `json:"updatedDate"`
	Status              *Status   `json:"status"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	p, err := h.svc.Update(r.Context(), orgID, chi.URLParam(r, "id"), UpdateInput{
		Name:                req.Name,
		ShortName:           req.ShortName,
		ThemeColor:          req.ThemeColor,
		BackgroundColor:     req.BackgroundColor,
		ThemeAuto:           req.ThemeAuto,
		ShowHeader:          req.ShowHeader,
		ShowBottomNav:       req.ShowBottomNav,
		HeaderTitle:         req.HeaderTitle,
		HeaderLogoURL:       req.HeaderLogoURL,
		IconURL:             req.IconURL,
		StartURL:            req.StartURL,
		BounceInAppWebview:  req.BounceInAppWebview,
		Developer:           req.Developer,
		Description:         req.Description,
		Rating:              req.Rating,
		RatingCount:         req.RatingCount,
		DownloadsLabel:      req.DownloadsLabel,
		SizeMB:              req.SizeMB,
		Screenshots:         req.Screenshots,
		WhatsNew:            req.WhatsNew,
		RatingStars5:        req.RatingStars5,
		RatingStars4:        req.RatingStars4,
		RatingStars3:        req.RatingStars3,
		RatingStars2:        req.RatingStars2,
		RatingStars1:        req.RatingStars1,
		Version:             req.Version,
		AgeRating:           req.AgeRating,
		Category:            req.Category,
		InteractiveElements: req.InteractiveElements,
		ContainsAds:         req.ContainsAds,
		InAppPurchases:      req.InAppPurchases,
		UpdatedDate:         req.UpdatedDate,
		Status:              req.Status,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	if err := h.svc.Delete(r.Context(), orgID, chi.URLParam(r, "id")); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) duplicate(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	p, err := h.svc.Duplicate(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handler) pause(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	p, err := h.svc.Pause(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) activate(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	p, err := h.svc.Activate(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// defaultOn defaults a create request that omits a show_* toggle to on, so
// clients that predate the toggle keep today's page.
func defaultOn(v *bool) bool { return v == nil || *v }
