package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const uniqueViolation = "23505"

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

// CreateOrgWithOwner is signup's single transaction: a brand new
// organization, a brand new user, and an Owner membership binding them —
// all three or none, so a partial failure never leaves an org with no
// Owner or a user with no org.
func (r *Repository) CreateOrgWithOwner(ctx context.Context, orgID, orgName, userID, name, email, passwordHash, membershipID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("auth: beginning signup tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, orgID, orgName); err != nil {
		return fmt.Errorf("auth: creating organization: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO users (id, email, name, password_hash) VALUES ($1, $2, $3, $4)`, userID, email, name, passwordHash); err != nil {
		if isUniqueViolation(err) {
			return apierror.Conflict("an account with this email already exists")
		}
		return fmt.Errorf("auth: creating user: %w", err)
	}
	var roleID string
	if err := tx.QueryRow(ctx, `SELECT id FROM roles WHERE key = 'Owner'`).Scan(&roleID); err != nil {
		return fmt.Errorf("auth: looking up Owner role: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO memberships (id, organization_id, user_id, role_id, status) VALUES ($1, $2, $3, $4, 'active')`,
		membershipID, orgID, userID, roleID,
	); err != nil {
		return fmt.Errorf("auth: creating owner membership: %w", err)
	}
	return tx.Commit(ctx)
}

type userAuthRow struct {
	ID           string
	Name         string
	Email        string
	PasswordHash string
}

// findUserByEmail reports found=false (not an error) when no such user
// exists — Login needs to distinguish "no such account" from a real
// lookup failure without leaking which one happened to the client (both
// render as the same generic "invalid email or password").
func (r *Repository) findUserByEmail(ctx context.Context, email string) (row userAuthRow, found bool, err error) {
	err = r.db.QueryRow(ctx, `SELECT id, name, email, password_hash FROM users WHERE lower(email) = lower($1)`, email).
		Scan(&row.ID, &row.Name, &row.Email, &row.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return userAuthRow{}, false, nil
	}
	return row, err == nil, err
}

type activeMembershipRow struct {
	OrganizationID   string
	OrganizationName string
	RoleKey          string
}

func (r *Repository) activeMembershipsByUser(ctx context.Context, userID string) ([]activeMembershipRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT m.organization_id, o.name, r.key
		FROM memberships m
		JOIN organizations o ON o.id = m.organization_id
		JOIN roles r ON r.id = m.role_id
		WHERE m.user_id = $1 AND m.status = 'active'
		ORDER BY o.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("auth: listing active memberships: %w", err)
	}
	defer rows.Close()

	var out []activeMembershipRow
	for rows.Next() {
		var m activeMembershipRow
		if err := rows.Scan(&m.OrganizationID, &m.OrganizationName, &m.RoleKey); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) CreateSession(ctx context.Context, id, userID, orgID, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO sessions (id, user_id, organization_id, token_hash, expires_at) VALUES ($1, $2, $3, $4, $5)`,
		id, userID, orgID, tokenHash, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("auth: creating session: %w", err)
	}
	return nil
}

// ResolveSession validates tokenHash and, in the same round trip, bumps
// the matching membership's last_active_at (the "last active" column the
// Team UI's member list has always expected real data for). Requiring
// status = 'active' in the same query means suspending or removing a
// member takes effect on their very next request — an existing session
// cookie for a suspended/removed membership resolves to zero rows here,
// not a stale success.
func (r *Repository) ResolveSession(ctx context.Context, tokenHash string) (orgID, userID string, err error) {
	err = r.db.QueryRow(ctx, `
		WITH s AS (
			SELECT organization_id, user_id FROM sessions
			WHERE token_hash = $1 AND expires_at > now()
		)
		UPDATE memberships m
		SET last_active_at = now()
		FROM s
		WHERE m.organization_id = s.organization_id AND m.user_id = s.user_id AND m.status = 'active'
		RETURNING s.organization_id, s.user_id`,
		tokenHash,
	).Scan(&orgID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", apierror.Unauthorized("session expired or invalid")
	}
	return orgID, userID, err
}

func (r *Repository) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	if err != nil {
		return fmt.Errorf("auth: deleting session: %w", err)
	}
	return nil
}

// deleteSessionsForMembership revokes every existing session for (userID,
// orgID) immediately — called on suspend/remove so access ends right away
// rather than waiting for ResolveSession's own status='active' check to
// catch it on that user's next request (belt-and-suspenders: the join
// check alone is already correct, this just also cleans up the now-dead
// session rows instead of leaving them to expire naturally).
func (r *Repository) deleteSessionsForMembership(ctx context.Context, userID, orgID string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1 AND organization_id = $2`, userID, orgID)
	if err != nil {
		return fmt.Errorf("auth: revoking sessions: %w", err)
	}
	return nil
}

func (r *Repository) HasPermission(ctx context.Context, userID, orgID, permissionKey string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM memberships m
			JOIN role_permissions rp ON rp.role_id = m.role_id
			JOIN permissions p ON p.id = rp.permission_id
			WHERE m.user_id = $1 AND m.organization_id = $2 AND m.status = 'active' AND p.key = $3
		)`,
		userID, orgID, permissionKey,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("auth: checking permission: %w", err)
	}
	return exists, nil
}

func (r *Repository) PermissionsForRole(ctx context.Context, roleKey string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.key FROM role_permissions rp
		JOIN permissions p ON p.id = rp.permission_id
		JOIN roles r ON r.id = rp.role_id
		WHERE r.key = $1
		ORDER BY p.key`, roleKey)
	if err != nil {
		return nil, fmt.Errorf("auth: listing role permissions: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

type whoAmIRow struct {
	UserID           string
	UserName         string
	UserEmail        string
	OrganizationName string
	RoleKey          string
}

func (r *Repository) WhoAmI(ctx context.Context, userID, orgID string) (whoAmIRow, error) {
	var row whoAmIRow
	err := r.db.QueryRow(ctx, `
		SELECT u.id, u.name, u.email, o.name, r.key
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		JOIN organizations o ON o.id = m.organization_id
		JOIN roles r ON r.id = m.role_id
		WHERE m.user_id = $1 AND m.organization_id = $2 AND m.status = 'active'`,
		userID, orgID,
	).Scan(&row.UserID, &row.UserName, &row.UserEmail, &row.OrganizationName, &row.RoleKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return whoAmIRow{}, apierror.Unauthorized("session expired or invalid")
	}
	return row, err
}

const membershipColumns = `m.id, u.id, u.name, u.email, r.key, m.status, m.invited_at, m.last_active_at`

func scanMembership(row pgx.Row) (Membership, error) {
	var m Membership
	err := row.Scan(&m.ID, &m.UserID, &m.UserName, &m.UserEmail, &m.RoleKey, &m.Status, &m.InvitedAt, &m.LastActiveAt)
	return m, err
}

func (r *Repository) ListMemberships(ctx context.Context, orgID string) ([]Membership, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+membershipColumns+`
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		JOIN roles r ON r.id = m.role_id
		WHERE m.organization_id = $1
		ORDER BY m.invited_at`, orgID)
	if err != nil {
		return nil, fmt.Errorf("auth: listing members: %w", err)
	}
	defer rows.Close()

	var out []Membership
	for rows.Next() {
		m, err := scanMembership(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) GetMembership(ctx context.Context, orgID, membershipID string) (Membership, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+membershipColumns+`
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		JOIN roles r ON r.id = m.role_id
		WHERE m.id = $1 AND m.organization_id = $2`, membershipID, orgID)
	m, err := scanMembership(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Membership{}, apierror.NotFound("member not found")
	}
	return m, err
}

// FindOrCreateShellUser returns an existing user's id by email, or creates
// a new "shell" user — password_hash left at its ” sentinel (migration
// 00020) — if no account with that email exists yet. Shell users can't
// log in until AcceptInvite sets a real password.
func (r *Repository) FindOrCreateShellUser(ctx context.Context, newID, email, name string) (userID string, err error) {
	err = r.db.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = lower($1)`, email).Scan(&userID)
	if err == nil {
		return userID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("auth: looking up user by email: %w", err)
	}

	_, err = r.db.Exec(ctx, `INSERT INTO users (id, email, name, password_hash) VALUES ($1, $2, $3, '')`, newID, email, name)
	if err == nil {
		return newID, nil
	}
	if isUniqueViolation(err) {
		// Lost a race with a concurrent signup/invite using the same
		// email — look it up again rather than fail the whole invite.
		var existingID string
		if lookupErr := r.db.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = lower($1)`, email).Scan(&existingID); lookupErr == nil {
			return existingID, nil
		}
	}
	return "", fmt.Errorf("auth: creating shell user: %w", err)
}

func (r *Repository) CreateInviteMembership(ctx context.Context, id, orgID, userID, roleKey, tokenHash string, expiresAt time.Time) error {
	var roleID string
	if err := r.db.QueryRow(ctx, `SELECT id FROM roles WHERE key = $1`, roleKey).Scan(&roleID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.Validation("invalid role", map[string]string{"role": "unknown role"})
		}
		return fmt.Errorf("auth: looking up role: %w", err)
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO memberships (id, organization_id, user_id, role_id, status, invite_token_hash, invite_token_expires_at)
		VALUES ($1, $2, $3, $4, 'invited', $5, $6)`,
		id, orgID, userID, roleID, tokenHash, expiresAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return apierror.Conflict("this person is already a member of this organization")
		}
		return fmt.Errorf("auth: creating invite: %w", err)
	}
	return nil
}

// SetInviteToken regenerates a pending invite's token and expiry
// ("resend"), also bumping invited_at so the Team list's own "invited"
// timestamp reflects the most recent send, matching the mock UI's
// resendInvite behavior it's replacing.
func (r *Repository) SetInviteToken(ctx context.Context, orgID, membershipID, tokenHash string, expiresAt time.Time) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE memberships SET invite_token_hash = $1, invite_token_expires_at = $2, invited_at = now()
		WHERE id = $3 AND organization_id = $4 AND status = 'invited'`,
		tokenHash, expiresAt, membershipID, orgID,
	)
	if err != nil {
		return fmt.Errorf("auth: resending invite: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("pending invite not found")
	}
	return nil
}

type inviteRow struct {
	MembershipID     string
	OrganizationID   string
	OrganizationName string
	UserID           string
	Email            string
	RoleKey          string
}

func (r *Repository) FindMembershipByInviteToken(ctx context.Context, tokenHash string) (inviteRow, error) {
	var inv inviteRow
	err := r.db.QueryRow(ctx, `
		SELECT m.id, m.organization_id, o.name, u.id, u.email, r.key
		FROM memberships m
		JOIN organizations o ON o.id = m.organization_id
		JOIN users u ON u.id = m.user_id
		JOIN roles r ON r.id = m.role_id
		WHERE m.invite_token_hash = $1 AND m.status = 'invited' AND m.invite_token_expires_at > now()`,
		tokenHash,
	).Scan(&inv.MembershipID, &inv.OrganizationID, &inv.OrganizationName, &inv.UserID, &inv.Email, &inv.RoleKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return inviteRow{}, apierror.NotFound("invite not found or expired")
	}
	return inv, err
}

func (r *Repository) AcceptInvite(ctx context.Context, membershipID, userID, name, passwordHash string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("auth: beginning accept-invite tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $1, name = $2 WHERE id = $3`, passwordHash, name, userID); err != nil {
		return fmt.Errorf("auth: setting password: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE memberships SET status = 'active', invite_token_hash = NULL, invite_token_expires_at = NULL
		WHERE id = $1 AND status = 'invited'`, membershipID)
	if err != nil {
		return fmt.Errorf("auth: activating membership: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("invite not found or already accepted")
	}
	return tx.Commit(ctx)
}

// UpdateMembership refuses to touch a membership whose role is Owner —
// same "no actions for the Owner row" contract apps/web's
// member-row-actions.tsx already renders; enforced here too since
// frontend permissions are only UX (§52).
func (r *Repository) UpdateMembership(ctx context.Context, orgID, membershipID string, in UpdateMembershipInput) (userID string, err error) {
	var currentRoleKey string
	if err := r.db.QueryRow(ctx, `
		SELECT m.user_id, r.key FROM memberships m JOIN roles r ON r.id = m.role_id
		WHERE m.id = $1 AND m.organization_id = $2`, membershipID, orgID,
	).Scan(&userID, &currentRoleKey); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", apierror.NotFound("member not found")
		}
		return "", fmt.Errorf("auth: looking up member: %w", err)
	}
	if currentRoleKey == "Owner" {
		return "", apierror.Validation("cannot modify the organization owner", nil)
	}

	sets := []string{}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if in.RoleKey != nil {
		var roleID string
		if err := r.db.QueryRow(ctx, `SELECT id FROM roles WHERE key = $1`, *in.RoleKey).Scan(&roleID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", apierror.Validation("invalid role", map[string]string{"role": "unknown role"})
			}
			return "", fmt.Errorf("auth: looking up role: %w", err)
		}
		sets = append(sets, "role_id = "+arg(roleID))
	}
	if in.Status != nil {
		sets = append(sets, "status = "+arg(*in.Status))
	}
	if len(sets) == 0 {
		return userID, nil
	}

	args = append(args, membershipID, orgID)
	query := fmt.Sprintf(`UPDATE memberships SET %s WHERE id = $%d AND organization_id = $%d`, strings.Join(sets, ", "), len(args)-1, len(args))
	if _, err := r.db.Exec(ctx, query, args...); err != nil {
		return "", fmt.Errorf("auth: updating membership: %w", err)
	}

	if in.Status != nil && *in.Status == "suspended" {
		if err := r.deleteSessionsForMembership(ctx, userID, orgID); err != nil {
			return "", err
		}
	}
	return userID, nil
}

func (r *Repository) DeleteMembership(ctx context.Context, orgID, membershipID string) error {
	var userID, roleKey string
	if err := r.db.QueryRow(ctx, `
		SELECT m.user_id, r.key FROM memberships m JOIN roles r ON r.id = m.role_id
		WHERE m.id = $1 AND m.organization_id = $2`, membershipID, orgID,
	).Scan(&userID, &roleKey); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.NotFound("member not found")
		}
		return fmt.Errorf("auth: looking up member: %w", err)
	}
	if roleKey == "Owner" {
		return apierror.Validation("cannot remove the organization owner", nil)
	}

	tag, err := r.db.Exec(ctx, `DELETE FROM memberships WHERE id = $1 AND organization_id = $2`, membershipID, orgID)
	if err != nil {
		return fmt.Errorf("auth: removing membership: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("member not found")
	}
	return r.deleteSessionsForMembership(ctx, userID, orgID)
}

func (r *Repository) RecordActivity(ctx context.Context, id, orgID string, actorUserID *string, action, entityType, entityID string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO audit_logs (id, organization_id, actor_user_id, action, entity_type, entity_id) VALUES ($1, $2, $3, $4, $5, $6)`,
		id, orgID, actorUserID, action, entityType, entityID,
	)
	if err != nil {
		return fmt.Errorf("auth: recording activity: %w", err)
	}
	return nil
}

func (r *Repository) ListActivity(ctx context.Context, orgID string, limit int) ([]ActivityEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT a.id, a.action, a.entity_type, a.entity_id, u.name, a.created_at
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.actor_user_id
		WHERE a.organization_id = $1
		ORDER BY a.created_at DESC
		LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, fmt.Errorf("auth: listing activity: %w", err)
	}
	defer rows.Close()

	var out []ActivityEntry
	for rows.Next() {
		var e ActivityEntry
		if err := rows.Scan(&e.ID, &e.Action, &e.EntityType, &e.EntityID, &e.ActorName, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
