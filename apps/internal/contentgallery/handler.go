package contentgallery

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"

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

// Register mounts every /content-gallery route onto r. The caller applies
// tenant.Middleware first — same contract as every other handler.
func (h *Handler) Register(r chi.Router) {
	r.Get("/folders", h.listFolders)
	r.With(h.requireWrite).Post("/folders", h.createFolder)
	r.With(h.requireWrite).Patch("/folders/{id}", h.renameFolder)
	r.With(h.requireWrite).Delete("/folders/{id}", h.deleteFolder)

	r.Get("/files", h.listFiles)
	r.With(h.requireWrite).Post("/files", h.uploadFile)
	r.With(h.requireWrite).Post("/files/move", h.moveFiles)
	r.With(h.requireWrite).Post("/files/archive", h.archiveFiles)
	r.With(h.requireWrite).Post("/files/restore", h.restoreFiles)
	r.With(h.requireWrite).Post("/files/delete", h.deleteFiles)

	// System content (system.go): read like the library, used like an upload.
	r.Get("/system", h.listSystem)
	r.Get("/system/{itemId}/preview", h.previewSystem)
	r.With(h.requireWrite).Post("/system/{itemId}/use", h.useSystem)
}

type systemItemResponse struct {
	ID          string            `json:"id"`
	Kind        SystemKind        `json:"kind"`
	Name        map[string]string `json:"name"`
	Description map[string]string `json:"description"`
	Tags        []string          `json:"tags"`
	// PreviewPath is the card image, relative to the API's origin.
	PreviewPath string `json:"previewPath"`
	// AssetBasePath, for a landing template, is where its CSS and images
	// are served — what the preview needs as its assetBase.
	AssetBasePath string `json:"assetBasePath,omitempty"`
}

func (h *Handler) listSystem(w http.ResponseWriter, _ *http.Request) {
	items := h.svc.SystemItems()
	out := make([]systemItemResponse, len(items))
	for i, it := range items {
		out[i] = systemItemResponse{
			ID: it.ID, Kind: it.Kind, Name: it.Name, Description: it.Description, Tags: it.Tags,
			PreviewPath: SystemAssetBasePath + it.Preview,
		}
		if it.Kind == SystemLanding {
			out[i].AssetBasePath = SystemAssetBasePath + it.Dir + "/"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// previewSystem answers {"html": …} for a landing template; JSON, never
// text/html — the page must not run on the API's origin.
func (h *Handler) previewSystem(w http.ResponseWriter, r *http.Request) {
	html, err := h.svc.SystemLandingPreview(chi.URLParam(r, "itemId"), r.URL.Query().Get("assetBase"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"html": html})
}

func (h *Handler) useSystem(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	var req struct {
		Name string `json:"name"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
			return
		}
	}
	res, err := h.svc.UseSystemItem(r.Context(), orgID, userID, chi.URLParam(r, "itemId"), SystemUseInput{Name: req.Name})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (h *Handler) listFolders(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	folders, err := h.svc.ListFolders(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"folders": folders})
}

type folderRequest struct {
	Name string `json:"name"`
}

func (h *Handler) createFolder(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req folderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	f, err := h.svc.CreateFolder(r.Context(), orgID, req.Name)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (h *Handler) renameFolder(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req folderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	f, err := h.svc.RenameFolder(r.Context(), orgID, chi.URLParam(r, "id"), req.Name)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *Handler) deleteFolder(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	if err := h.svc.DeleteFolder(r.Context(), orgID, chi.URLParam(r, "id")); err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listFiles answers GET /content-gallery/files?folderId=&type=&status=
// &uploadedBy=&limit=&offset= — folderId="root" means "unfiled only"
// (ListFilesParams.FolderID's own doc explains the tri-state), matching
// the reference's own "toggle a folder filter" UX.
func (h *Handler) listFiles(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	q := r.URL.Query()

	params := ListFilesParams{
		FileType:         FileType(q.Get("type")),
		Status:           FileStatus(q.Get("status")),
		UploadedByUserID: q.Get("uploadedBy"),
	}
	if v := q.Get("folderId"); v != "" {
		if v == "root" {
			root := ""
			params.FolderID = &root
		} else {
			params.FolderID = &v
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			params.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			params.Offset = n
		}
	}

	result, err := h.svc.ListFiles(r.Context(), orgID, params)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// maxUploadFormBytes caps the whole multipart body a little above
// maxUploadBytes (service.go) to leave room for the form's own field
// overhead, so a file that's actually within the real limit is never
// rejected by this outer cap first.
const maxUploadFormBytes = maxUploadBytes + 1<<20

func (h *Handler) uploadFile(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadFormBytes)
	if err := r.ParseMultipartForm(maxUploadFormBytes); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("file exceeds the upload size limit", nil))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		apierror.Write(w, h.logger, apierror.Validation("missing file", nil))
		return
	}
	defer file.Close()

	// Sniffed from the file's own bytes, not trusted from the client-
	// supplied Content-Type header — see allowedImageExt's own doc
	// comment in service.go.
	peek := make([]byte, 512)
	n, _ := io.ReadFull(file, peek)
	peek = peek[:n]
	contentType := http.DetectContentType(peek)
	reader := io.MultiReader(bytes.NewReader(peek), file)

	var folderID *string
	if v := r.FormValue("folderId"); v != "" {
		folderID = &v
	}

	f, err := h.svc.UploadFile(r.Context(), orgID, userID, UploadInput{
		FolderID:    folderID,
		Filename:    header.Filename,
		ContentType: contentType,
		Size:        header.Size,
		Reader:      reader,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

type fileIDsRequest struct {
	IDs      []string `json:"ids"`
	FolderID *string  `json:"folderId"`
}

func (h *Handler) moveFiles(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req fileIDsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	count, err := h.svc.MoveFiles(r.Context(), orgID, req.IDs, req.FolderID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"moved": count})
}

func (h *Handler) archiveFiles(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req fileIDsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	count, err := h.svc.ArchiveFiles(r.Context(), orgID, req.IDs)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"archived": count})
}

func (h *Handler) restoreFiles(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req fileIDsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	count, err := h.svc.RestoreFiles(r.Context(), orgID, req.IDs)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"restored": count})
}

func (h *Handler) deleteFiles(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req fileIDsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	count, err := h.svc.DeleteFiles(r.Context(), orgID, req.IDs)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": count})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
