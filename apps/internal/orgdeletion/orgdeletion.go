// Package orgdeletion deletes an organization and everything it owns, on
// its owner's request (audit 2026-10-15, GDPR-1 part 2;
// docs/organization-deletion.md).
//
// An owner schedules the deletion with the organization's name and their
// password (Service.Schedule); it happens GracePeriod later and can be
// cancelled until then. apps/worker then runs Runner: in one Postgres
// transaction it deletes the organizations row — every tenant table
// cascades from it — and the users who belonged to it and to no other
// organization, leaving a tombstone in organization_deletions. PurgeDelay
// later, once events still in flight have landed, it deletes the
// organization's rows from every ClickHouse table that has an
// organization_id, its files from object storage and its Redis keys, and
// marks the tombstone purged.
package orgdeletion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/auth"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// GracePeriod is how long a scheduled deletion can be cancelled.
const GracePeriod = 7 * 24 * time.Hour

// ownerRole is roles.key of the role that may delete the organization.
const ownerRole = "Owner"

// Mailer sends the owners a notice; *mailer.Client is the real one.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// Status is a scheduled deletion; ScheduledAt nil means none.
type Status struct {
	ScheduledAt *time.Time `json:"scheduledAt"`
	RequestedAt *time.Time `json:"requestedAt"`
	RequestedBy string     `json:"requestedBy"`
}

type Service struct {
	db     *pgxpool.Pool
	mail   Mailer
	appURL string
	logger *slog.Logger
	now    func() time.Time
}

func NewService(db *pgxpool.Pool, mail Mailer, appURL string, logger *slog.Logger) *Service {
	return &Service{db: db, mail: mail, appURL: appURL, logger: logger, now: time.Now}
}

// Get is orgID's scheduled deletion, if any.
func (s *Service) Get(ctx context.Context, orgID string) (Status, error) {
	var st Status
	var by *string
	err := s.db.QueryRow(ctx, `
		SELECT o.deletion_scheduled_at, o.deletion_requested_at, u.name
		FROM organizations o LEFT JOIN users u ON u.id = o.deletion_requested_by
		WHERE o.id = $1`, orgID).Scan(&st.ScheduledAt, &st.RequestedAt, &by)
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{}, apierror.NotFound("organization not found")
	}
	if err != nil {
		return Status{}, fmt.Errorf("orgdeletion: reading status: %w", err)
	}
	if by != nil && st.ScheduledAt != nil {
		st.RequestedBy = *by
	}
	return st, nil
}

// Schedule schedules orgID's deletion GracePeriod from now. Only an
// active owner may, confirming with the organization's exact name and
// their own password; a deletion already scheduled is a conflict.
func (s *Service) Schedule(ctx context.Context, orgID, userID, confirmName, password string) (Status, error) {
	var name, hash, role string
	var scheduled *time.Time
	err := s.db.QueryRow(ctx, `
		SELECT o.name, u.password_hash, r.key, o.deletion_scheduled_at
		FROM organizations o
		JOIN memberships m ON m.organization_id = o.id AND m.user_id = $2 AND m.status = 'active'
		JOIN users u ON u.id = m.user_id
		JOIN roles r ON r.id = m.role_id
		WHERE o.id = $1`, orgID, userID).Scan(&name, &hash, &role, &scheduled)
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{}, apierror.Forbidden("only an owner of the organization can delete it")
	}
	if err != nil {
		return Status{}, fmt.Errorf("orgdeletion: reading organization: %w", err)
	}
	if role != ownerRole {
		return Status{}, apierror.Forbidden("only an owner of the organization can delete it")
	}
	if scheduled != nil {
		return Status{}, apierror.Conflict("the organization is already scheduled for deletion")
	}
	if strings.TrimSpace(confirmName) != name {
		return Status{}, apierror.Validation("type the organization's name exactly to confirm", map[string]string{"confirmName": "does not match"})
	}
	if !auth.PasswordMatches(hash, password) {
		return Status{}, apierror.Validation("wrong password", map[string]string{"password": "wrong"})
	}
	now := s.now().UTC()
	at := now.Add(GracePeriod)
	if _, err := s.db.Exec(ctx, `
		UPDATE organizations
		SET deletion_scheduled_at = $2, deletion_requested_at = $3, deletion_requested_by = $4
		WHERE id = $1 AND deletion_scheduled_at IS NULL`, orgID, at, now, userID); err != nil {
		return Status{}, fmt.Errorf("orgdeletion: scheduling: %w", err)
	}
	s.notifyOwners(ctx, orgID, name, fmt.Sprintf("FLOX: %s will be deleted on %s", name, at.Format("2 Jan 2006 15:04 MST")),
		fmt.Sprintf("Deletion of the organization %q was requested. On %s it and all its data — campaigns, clicks, conversions, reports, files — will be deleted for good.\n\nAn owner can cancel until then: %s/settings\n",
			name, at.Format("2 Jan 2006 15:04 MST"), s.appURL))
	return s.Get(ctx, orgID)
}

// Cancel cancels orgID's scheduled deletion. Only an active owner may.
func (s *Service) Cancel(ctx context.Context, orgID, userID string) (Status, error) {
	var name, role string
	err := s.db.QueryRow(ctx, `
		SELECT o.name, r.key FROM organizations o
		JOIN memberships m ON m.organization_id = o.id AND m.user_id = $2 AND m.status = 'active'
		JOIN roles r ON r.id = m.role_id
		WHERE o.id = $1`, orgID, userID).Scan(&name, &role)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && role != ownerRole) {
		return Status{}, apierror.Forbidden("only an owner of the organization can cancel its deletion")
	}
	if err != nil {
		return Status{}, fmt.Errorf("orgdeletion: reading organization: %w", err)
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE organizations
		SET deletion_scheduled_at = NULL, deletion_requested_at = NULL, deletion_requested_by = NULL
		WHERE id = $1 AND deletion_scheduled_at IS NOT NULL`, orgID)
	if err != nil {
		return Status{}, fmt.Errorf("orgdeletion: cancelling: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Status{}, apierror.Conflict("the organization is not scheduled for deletion")
	}
	s.notifyOwners(ctx, orgID, name, fmt.Sprintf("FLOX: deletion of %s cancelled", name),
		fmt.Sprintf("The scheduled deletion of the organization %q was cancelled. Nothing was deleted.\n", name))
	return s.Get(ctx, orgID)
}

// notifyOwners mails every active owner. A failed send is logged, not
// returned: the request itself succeeded, and the banner in the app says
// the same.
func (s *Service) notifyOwners(ctx context.Context, orgID, orgName, subject, body string) {
	if s.mail == nil {
		return
	}
	rows, err := s.db.Query(ctx, `
		SELECT u.email FROM memberships m
		JOIN users u ON u.id = m.user_id
		JOIN roles r ON r.id = m.role_id
		WHERE m.organization_id = $1 AND m.status = 'active' AND r.key = $2`, orgID, ownerRole)
	if err != nil {
		s.logger.Warn("organization deletion: listing owners to notify", "organization_id", orgID, "error", err)
		return
	}
	var emails []string
	for rows.Next() {
		var e string
		if rows.Scan(&e) == nil {
			emails = append(emails, e)
		}
	}
	rows.Close()
	for _, e := range emails {
		if err := s.mail.Send(ctx, e, subject, body); err != nil {
			s.logger.Warn("organization deletion: notifying an owner", "organization_id", orgID, "organization", orgName, "error", err)
		}
	}
}

type Handler struct {
	svc *Service
	// requireWrite is settings.write — every owner has it; the service
	// then checks the owner role itself.
	requireWrite func(http.Handler) http.Handler
	logger       *slog.Logger
}

func NewHandler(svc *Service, requireWrite func(http.Handler) http.Handler, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireWrite: requireWrite, logger: logger}
}

// Register mounts /organization/deletion. The caller applies
// tenant.Middleware. Reading is open to every member (the app-wide banner);
// scheduling and cancelling need settings.write, then the owner role (the
// service checks it), and refuse an API key: deleting an organization is
// a person's decision.
func (h *Handler) Register(r chi.Router) {
	r.Get("/organization/deletion", h.get)
	r.With(h.requireWrite).Post("/organization/deletion", h.schedule)
	r.With(h.requireWrite).Delete("/organization/deletion", h.cancel)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	st, err := h.svc.Get(r.Context(), orgID)
	h.write(w, st, err)
}

type scheduleRequest struct {
	ConfirmName string `json:"confirmName"`
	Password    string `json:"password"`
}

func (h *Handler) schedule(w http.ResponseWriter, r *http.Request) {
	if !h.person(w, r) {
		return
	}
	var req scheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	st, err := h.svc.Schedule(r.Context(), orgID, userID, req.ConfirmName, req.Password)
	h.write(w, st, err)
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	if !h.person(w, r) {
		return
	}
	orgID, _ := tenant.OrgID(r.Context())
	userID, _ := tenant.UserID(r.Context())
	st, err := h.svc.Cancel(r.Context(), orgID, userID)
	h.write(w, st, err)
}

// person refuses API-key requests.
func (h *Handler) person(w http.ResponseWriter, r *http.Request) bool {
	if scope, ok := tenant.APIKeyScope(r.Context()); ok && scope != "" {
		apierror.Write(w, h.logger, apierror.Forbidden("an API key cannot delete an organization"))
		return false
	}
	return true
}

func (h *Handler) write(w http.ResponseWriter, st Status, err error) {
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}
