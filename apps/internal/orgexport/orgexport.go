// Package orgexport hands an organization its data (audit 2026-10-15,
// GDPR-1 part 4; docs/organization-export.md).
//
// An owner or admin requests an export for a period (Service.Request);
// apps/worker builds it (Runner): a ZIP with every tenant Postgres table
// as JSON lines — secret columns and internal tables left out — the team,
// and the ClickHouse events of the period as CSV. The archive goes to
// object storage under a key with 32 random bytes that never leaves the
// server; it is downloaded through the API only (the bucket serves any
// object whose key is known, so the key is the secret) and deleted after
// Retention.
package orgexport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// Retention is how long a finished export can be downloaded.
const Retention = 7 * 24 * time.Hour

// MaxPeriod bounds the events of one export.
const MaxPeriod = 366 * 24 * time.Hour

type Export struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	From        string     `json:"from"`
	To          string     `json:"to"`
	SizeBytes   int64      `json:"sizeBytes"`
	Error       string     `json:"error"`
	CreatedAt   time.Time  `json:"createdAt"`
	FinishedAt  *time.Time `json:"finishedAt"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	RequestedBy string     `json:"requestedBy"`
}

// Opener streams a stored object; *storage.Client is the real one.
type Opener interface {
	Open(ctx context.Context, key string) (io.ReadCloser, error)
}

type Service struct {
	db      *pgxpool.Pool
	objects Opener
	now     func() time.Time
}

func NewService(db *pgxpool.Pool, objects Opener) *Service {
	return &Service{db: db, objects: objects, now: time.Now}
}

// Request queues an export of orgID's data with the events from from to
// to (inclusive dates). One at a time per organization.
func (s *Service) Request(ctx context.Context, orgID, userID, from, to string) (Export, error) {
	f, errF := time.Parse(time.DateOnly, from)
	t, errT := time.Parse(time.DateOnly, to)
	if errF != nil || errT != nil {
		return Export{}, apierror.Validation("from and to must be dates (YYYY-MM-DD)", map[string]string{"from": "date", "to": "date"})
	}
	if t.Before(f) {
		return Export{}, apierror.Validation("to is before from", map[string]string{"to": "before from"})
	}
	if t.Sub(f) > MaxPeriod {
		return Export{}, apierror.Validation("the period is longer than a year", map[string]string{"to": "more than 366 days"})
	}
	if t.After(s.now().UTC().AddDate(0, 0, 1)) {
		return Export{}, apierror.Validation("to is in the future", map[string]string{"to": "future"})
	}
	id := idgen.New()
	tag, err := s.db.Exec(ctx, `
		INSERT INTO organization_exports (id, organization_id, requested_by, from_date, to_date)
		SELECT $1::text, $2::text, $3::text, $4::date, $5::date
		WHERE NOT EXISTS (SELECT 1 FROM organization_exports WHERE organization_id = $2::text AND status IN ('pending', 'running'))`,
		id, orgID, userID, f, t)
	if err != nil {
		return Export{}, fmt.Errorf("orgexport: requesting: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Export{}, apierror.Conflict("an export is already being prepared")
	}
	return s.get(ctx, orgID, id)
}

const exportColumns = `e.id, e.status, e.from_date, e.to_date, e.size_bytes, e.error, e.created_at, e.finished_at, e.expires_at, coalesce(u.name, '')`

func scanExport(row pgx.Row) (Export, error) {
	var e Export
	var from, to time.Time
	err := row.Scan(&e.ID, &e.Status, &from, &to, &e.SizeBytes, &e.Error, &e.CreatedAt, &e.FinishedAt, &e.ExpiresAt, &e.RequestedBy)
	e.From, e.To = from.Format(time.DateOnly), to.Format(time.DateOnly)
	return e, err
}

func (s *Service) get(ctx context.Context, orgID, id string) (Export, error) {
	e, err := scanExport(s.db.QueryRow(ctx, `SELECT `+exportColumns+`
		FROM organization_exports e LEFT JOIN users u ON u.id = e.requested_by
		WHERE e.id = $1 AND e.organization_id = $2`, id, orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Export{}, apierror.NotFound("export not found")
	}
	if err != nil {
		return Export{}, fmt.Errorf("orgexport: reading: %w", err)
	}
	return e, nil
}

// List is orgID's latest exports.
func (s *Service) List(ctx context.Context, orgID string) ([]Export, error) {
	rows, err := s.db.Query(ctx, `SELECT `+exportColumns+`
		FROM organization_exports e LEFT JOIN users u ON u.id = e.requested_by
		WHERE e.organization_id = $1 ORDER BY e.created_at DESC LIMIT 20`, orgID)
	if err != nil {
		return nil, fmt.Errorf("orgexport: listing: %w", err)
	}
	defer rows.Close()
	out := []Export{}
	for rows.Next() {
		e, err := scanExport(rows)
		if err != nil {
			return nil, fmt.Errorf("orgexport: listing: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Open streams a finished, unexpired export of orgID. Anything else is
// not found: an export of another organization looks like none at all.
func (s *Service) Open(ctx context.Context, orgID, id string) (io.ReadCloser, Export, error) {
	var key string
	e, err := scanExport(s.db.QueryRow(ctx, `SELECT `+exportColumns+`
		FROM organization_exports e LEFT JOIN users u ON u.id = e.requested_by
		WHERE e.id = $1 AND e.organization_id = $2 AND e.status = 'done' AND e.expires_at > now()`, id, orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, Export{}, apierror.NotFound("export not found")
	}
	if err != nil {
		return nil, Export{}, fmt.Errorf("orgexport: reading: %w", err)
	}
	if err := s.db.QueryRow(ctx, `SELECT object_key FROM organization_exports WHERE id = $1 AND organization_id = $2`, id, orgID).Scan(&key); err != nil {
		return nil, Export{}, fmt.Errorf("orgexport: reading key: %w", err)
	}
	if s.objects == nil {
		return nil, Export{}, apierror.Unavailable("object storage is not configured")
	}
	r, err := s.objects.Open(ctx, key)
	if err != nil {
		return nil, Export{}, fmt.Errorf("orgexport: opening the archive: %w", err)
	}
	return r, e, nil
}

type Handler struct {
	svc          *Service
	requireWrite func(http.Handler) http.Handler
	logger       *slog.Logger
}

func NewHandler(svc *Service, requireWrite func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, logger: logger}
}

// Register mounts /organization/exports, all behind settings.write: an
// export holds every record of the organization, visitors' IPs included.
func (h *Handler) Register(r chi.Router) {
	r.With(h.requireWrite).Get("/organization/exports", h.list)
	r.With(h.requireWrite).Post("/organization/exports", h.request)
	r.With(h.requireWrite).Get("/organization/exports/{id}/download", h.download)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	out, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"exports": out})
}

func (h *Handler) request(w http.ResponseWriter, r *http.Request) {
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	e, err := h.svc.Request(r.Context(), orgID, userID, req.From, req.To)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusAccepted, e)
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	body, e, err := h.svc.Open(r.Context(), orgID, chi.URLParam(r, "id"))
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="flox-export-`+e.From+`-`+e.To+`.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	if e.SizeBytes > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(e.SizeBytes, 10))
	}
	if _, err := io.Copy(w, body); err != nil {
		h.logger.Warn("export download interrupted", "export_id", e.ID, "error", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
