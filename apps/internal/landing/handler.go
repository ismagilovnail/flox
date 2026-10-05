package landing

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
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

// Register mounts every /landings route onto r. The caller applies
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

	r.With(h.requireWrite).Post("/upload", h.upload)
	r.With(h.requireWrite).Post("/{id}/archive", h.replaceArchive)
	// Preview saves nothing, but it's a POST and only the template editor
	// (content.write) ever calls it — gated like every other mutating-
	// method route (apps/api's RBAC coverage test enforces this).
	r.With(h.requireWrite).Post("/{id}/preview", h.preview)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	landings, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"landings": landings})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	l, err := h.svc.Get(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

type createRequest struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	LogoURL string `json:"logoUrl"`
	Type    Type   `json:"type"`
	URL     string `json:"url"`
	Status  Status `json:"status"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	l, err := h.svc.Create(r.Context(), orgID, CreateInput{
		Name:    req.Name,
		Title:   req.Title,
		LogoURL: req.LogoURL,
		Type:    req.Type,
		URL:     req.URL,
		Status:  req.Status,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

type updateRequest struct {
	Name    *string `json:"name"`
	Title   *string `json:"title"`
	LogoURL *string `json:"logoUrl"`
	Type    *Type   `json:"type"`
	URL     *string `json:"url"`
	Content *string `json:"content"`
	Status  *Status `json:"status"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}

	l, err := h.svc.Update(r.Context(), orgID, chi.URLParam(r, "id"), UpdateInput{
		Name:    req.Name,
		Title:   req.Title,
		LogoURL: req.LogoURL,
		Type:    req.Type,
		URL:     req.URL,
		Content: req.Content,
		Status:  req.Status,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
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
	l, err := h.svc.Duplicate(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (h *Handler) pause(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	l, err := h.svc.Pause(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (h *Handler) activate(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	l, err := h.svc.Activate(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

// maxUploadFormBytes leaves room above maxArchiveBytes for the form's
// other fields, so an archive within the real limit is never rejected by
// this outer cap first (parseBundle reports the precise limit).
const maxUploadFormBytes = maxArchiveBytes + 1<<20

// readArchive parses the multipart body and returns its "archive" file.
// The returned closer must be called once the archive has been consumed.
func readArchive(w http.ResponseWriter, r *http.Request) (Archive, func(), error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadFormBytes)
	if err := r.ParseMultipartForm(maxUploadFormBytes); err != nil {
		return Archive{}, nil, apierror.Validation("invalid archive", map[string]string{
			"archive": fmt.Sprintf("the archive exceeds the %dMB limit", maxArchiveBytes>>20),
		})
	}
	file, header, err := r.FormFile("archive")
	if err != nil {
		_ = r.MultipartForm.RemoveAll()
		return Archive{}, nil, apierror.Validation("invalid archive", map[string]string{"archive": "attach a .zip archive"})
	}
	closer := func() {
		file.Close()
		_ = r.MultipartForm.RemoveAll()
	}
	return Archive{ReaderAt: file, Size: header.Size}, closer, nil
}

// upload answers POST /landings/upload (multipart: name, title, logoUrl,
// status, archive) — creates an internal landing from a ZIP.
func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	archive, closeArchive, err := readArchive(w, r)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	defer closeArchive()

	res, err := h.svc.CreateFromArchive(r.Context(), orgID, ArchiveCreateInput{
		Name:    r.FormValue("name"),
		Title:   r.FormValue("title"),
		LogoURL: r.FormValue("logoUrl"),
		Status:  Status(r.FormValue("status")),
		Archive: archive,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

// replaceArchive answers POST /landings/{id}/archive (multipart: archive)
// — re-uploads an internal landing's bundle, keeping its ID.
func (h *Handler) replaceArchive(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	archive, closeArchive, err := readArchive(w, r)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	defer closeArchive()

	res, err := h.svc.ReplaceArchive(r.Context(), orgID, chi.URLParam(r, "id"), archive)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type previewRequest struct {
	Content *string `json:"content"`
}

// preview answers POST /landings/{id}/preview — the rendered page as
// JSON ({"html": ...}), never as text/html: user HTML must only ever run
// inside the editor's sandboxed iframe, not on the API's own origin.
func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req previewRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxEntryBytes+64<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	out, err := h.svc.Preview(r.Context(), orgID, chi.URLParam(r, "id"), req.Content)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"html": out})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
