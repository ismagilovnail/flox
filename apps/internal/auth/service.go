package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

const (
	minPasswordLength = 8
	// sessionTTL: no "remember me" toggle exists in the (not-yet-built,
	// Phase 28B) login UI, so one flat lifetime — 30 days, a common
	// default for a first-party session cookie — rather than a shorter
	// expiry that would silently log an operator out mid-workday.
	sessionTTL = 30 * 24 * time.Hour
	// inviteTTL: long enough that an invite sent on a Friday is still
	// good the following Monday.
	inviteTTL = 7 * 24 * time.Hour
	// activityListLimit mirrors the mock TEAM_ACTIVITY feed's own size —
	// this is a recent-activity panel, not a paginated audit log browser
	// (nothing in apps/web asks for pagination here).
	activityListLimit = 50
)

var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// invitableRoles mirrors apps/web/src/features/team/invite-member-sheet.tsx's
// own INVITABLE_ROLES (ROLES minus "Owner") exactly — there is exactly one
// Owner per organization, created at signup, never assigned afterward.
var invitableRoles = map[string]bool{
	"Admin": true, "Manager": true, "Buyer": true, "Analyst": true, "Viewer": true,
}

type Service struct {
	repo   *Repository
	appURL string
}

// appURL is apps/web's own origin (config.AppURL) — an invite link points
// there, at a page Phase 28B builds (see docs/auth.md).
func NewService(repo *Repository, appURL string) *Service {
	return &Service{repo: repo, appURL: appURL}
}

// dummyPasswordHash exists purely so Login always pays bcrypt's comparison
// cost, whether or not the email matched a real account — without it, a
// nonexistent-email login would return measurably faster than a wrong-
// password one, letting an attacker enumerate registered emails by timing
// alone.
var dummyPasswordHash = mustHashOnce("not-a-real-password-used-only-for-constant-time-login")

func mustHashOnce(s string) string {
	h, err := hashPassword(s)
	if err != nil {
		panic("auth: failed to precompute dummy password hash: " + err.Error())
	}
	return h
}

func (s *Service) Signup(ctx context.Context, in SignupInput) (Result, error) {
	in.OrganizationName = strings.TrimSpace(in.OrganizationName)
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))

	fields := map[string]string{}
	if in.OrganizationName == "" {
		fields["organizationName"] = "required"
	}
	if in.Name == "" {
		fields["name"] = "required"
	}
	if !emailPattern.MatchString(in.Email) {
		fields["email"] = "must be a valid email address"
	}
	if len(in.Password) < minPasswordLength {
		fields["password"] = fmt.Sprintf("must be at least %d characters", minPasswordLength)
	}
	if len(fields) > 0 {
		return Result{}, apierror.Validation("invalid signup", fields)
	}

	hash, err := hashPassword(in.Password)
	if err != nil {
		return Result{}, err
	}

	orgID, userID, membershipID := idgen.New(), idgen.New(), idgen.New()
	if err := s.repo.CreateOrgWithOwner(ctx, orgID, in.OrganizationName, userID, in.Name, in.Email, hash, membershipID); err != nil {
		return Result{}, err
	}
	return s.createSession(ctx, userID, orgID, in.Name, in.Email, "Owner", in.OrganizationName)
}

func (s *Service) Login(ctx context.Context, in LoginInput) (Result, error) {
	email := strings.TrimSpace(strings.ToLower(in.Email))

	row, found, err := s.repo.findUserByEmail(ctx, email)
	if err != nil {
		return Result{}, err
	}
	hashToCheck := dummyPasswordHash
	if found {
		hashToCheck = row.PasswordHash
	}
	if !checkPassword(hashToCheck, in.Password) || !found {
		return Result{}, apierror.Unauthorized("invalid email or password")
	}

	memberships, err := s.repo.activeMembershipsByUser(ctx, row.ID)
	if err != nil {
		return Result{}, err
	}
	if len(memberships) == 0 {
		return Result{}, apierror.Unauthorized("no active organization membership")
	}

	var chosen activeMembershipRow
	switch {
	case in.OrganizationID != "":
		match := false
		for _, m := range memberships {
			if m.OrganizationID == in.OrganizationID {
				chosen, match = m, true
				break
			}
		}
		if !match {
			return Result{}, apierror.Unauthorized("not a member of that organization")
		}
	case len(memberships) == 1:
		chosen = memberships[0]
	default:
		return Result{}, apierror.Validation("multiple organizations available, specify organizationId", map[string]string{"organizationId": "required"})
	}

	return s.createSession(ctx, row.ID, chosen.OrganizationID, row.Name, row.Email, chosen.RoleKey, chosen.OrganizationName)
}

func (s *Service) createSession(ctx context.Context, userID, orgID, userName, userEmail, roleKey, orgName string) (Result, error) {
	token, tokenHash, err := newBearerToken()
	if err != nil {
		return Result{}, err
	}
	expiresAt := time.Now().Add(sessionTTL)
	if err := s.repo.CreateSession(ctx, idgen.New(), userID, orgID, tokenHash, expiresAt); err != nil {
		return Result{}, err
	}
	return Result{
		Token:        token,
		ExpiresAt:    expiresAt,
		User:         User{ID: userID, Name: userName, Email: userEmail},
		Organization: Organization{ID: orgID, Name: orgName},
		RoleKey:      roleKey,
	}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.repo.DeleteSession(ctx, hashToken(token))
}

// ResolveSession implements tenant.SessionResolver — this is the one
// method tenant.NewMiddleware calls on every authenticated request.
func (s *Service) ResolveSession(ctx context.Context, token string) (orgID, userID string, err error) {
	return s.repo.ResolveSession(ctx, hashToken(token))
}

type WhoAmI struct {
	User         User
	Organization Organization
	RoleKey      string
	Permissions  []string
}

func (s *Service) Me(ctx context.Context, userID, orgID string) (WhoAmI, error) {
	row, err := s.repo.WhoAmI(ctx, userID, orgID)
	if err != nil {
		return WhoAmI{}, err
	}
	perms, err := s.repo.PermissionsForRole(ctx, row.RoleKey)
	if err != nil {
		return WhoAmI{}, err
	}
	return WhoAmI{
		User:         User{ID: row.UserID, Name: row.UserName, Email: row.UserEmail},
		Organization: Organization{ID: orgID, Name: row.OrganizationName},
		RoleKey:      row.RoleKey,
		Permissions:  perms,
	}, nil
}

// RequirePermission is a chi-compatible middleware factory — mount it
// after the tenant middleware on any route this org's role-based access
// control should gate. Confirmed via AskUserQuestion: this phase wires it
// onto this package's own team endpoints only; applying it across every
// pre-existing domain route is an explicit, separate follow-up (see
// package doc comment).
func (s *Service) RequirePermission(permissionKey string, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			orgID, _ := tenant.OrgID(r.Context())
			userID, _ := tenant.UserID(r.Context())

			ok, err := s.repo.HasPermission(r.Context(), userID, orgID, permissionKey)
			if err != nil {
				apierror.Write(w, logger, err)
				return
			}
			if !ok {
				apierror.Write(w, logger, apierror.Forbidden("missing permission: "+permissionKey))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Service) ListMembers(ctx context.Context, orgID string) ([]Membership, error) {
	return s.repo.ListMemberships(ctx, orgID)
}

func (s *Service) Invite(ctx context.Context, orgID, actorUserID string, in InviteInput) (inviteURL string, err error) {
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	in.Name = strings.TrimSpace(in.Name)

	fields := map[string]string{}
	if in.Name == "" {
		fields["name"] = "required"
	}
	if !emailPattern.MatchString(in.Email) {
		fields["email"] = "must be a valid email address"
	}
	if in.RoleKey == "Owner" {
		fields["role"] = "there is already exactly one Owner; invite as another role"
	} else if !invitableRoles[in.RoleKey] {
		fields["role"] = "unknown role"
	}
	if len(fields) > 0 {
		return "", apierror.Validation("invalid invite", fields)
	}

	shellUserID, err := s.repo.FindOrCreateShellUser(ctx, idgen.New(), in.Email, in.Name)
	if err != nil {
		return "", err
	}

	token, tokenHash, err := newBearerToken()
	if err != nil {
		return "", err
	}
	membershipID := idgen.New()
	expiresAt := time.Now().Add(inviteTTL)
	if err := s.repo.CreateInviteMembership(ctx, membershipID, orgID, shellUserID, in.RoleKey, tokenHash, expiresAt); err != nil {
		return "", err
	}

	actor := actorUserID
	_ = s.repo.RecordActivity(ctx, idgen.New(), orgID, &actor, "team.invited", "membership", membershipID)
	return s.inviteURL(token), nil
}

func (s *Service) ResendInvite(ctx context.Context, orgID, actorUserID, membershipID string) (inviteURL string, err error) {
	token, tokenHash, err := newBearerToken()
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().Add(inviteTTL)
	if err := s.repo.SetInviteToken(ctx, orgID, membershipID, tokenHash, expiresAt); err != nil {
		return "", err
	}

	actor := actorUserID
	_ = s.repo.RecordActivity(ctx, idgen.New(), orgID, &actor, "team.invite_resent", "membership", membershipID)
	return s.inviteURL(token), nil
}

func (s *Service) inviteURL(token string) string {
	return strings.TrimRight(s.appURL, "/") + "/accept-invite?token=" + token
}

func (s *Service) PreviewInvite(ctx context.Context, token string) (InvitePreview, error) {
	inv, err := s.repo.FindMembershipByInviteToken(ctx, hashToken(token))
	if err != nil {
		return InvitePreview{}, err
	}
	return InvitePreview{OrganizationName: inv.OrganizationName, Email: inv.Email, RoleKey: inv.RoleKey}, nil
}

func (s *Service) AcceptInvite(ctx context.Context, in AcceptInviteInput) (Result, error) {
	in.Name = strings.TrimSpace(in.Name)

	fields := map[string]string{}
	if in.Name == "" {
		fields["name"] = "required"
	}
	if len(in.Password) < minPasswordLength {
		fields["password"] = fmt.Sprintf("must be at least %d characters", minPasswordLength)
	}
	if len(fields) > 0 {
		return Result{}, apierror.Validation("invalid", fields)
	}

	inv, err := s.repo.FindMembershipByInviteToken(ctx, hashToken(in.Token))
	if err != nil {
		return Result{}, err
	}

	hash, err := hashPassword(in.Password)
	if err != nil {
		return Result{}, err
	}
	if err := s.repo.AcceptInvite(ctx, inv.MembershipID, inv.UserID, in.Name, hash); err != nil {
		return Result{}, err
	}

	actor := inv.UserID
	_ = s.repo.RecordActivity(ctx, idgen.New(), inv.OrganizationID, &actor, "team.invite_accepted", "membership", inv.MembershipID)
	return s.createSession(ctx, inv.UserID, inv.OrganizationID, in.Name, inv.Email, inv.RoleKey, inv.OrganizationName)
}

func (s *Service) UpdateMembership(ctx context.Context, orgID, actorUserID, membershipID string, in UpdateMembershipInput) (Membership, error) {
	if in.RoleKey != nil {
		if *in.RoleKey == "Owner" || !invitableRoles[*in.RoleKey] {
			return Membership{}, apierror.Validation("invalid role", map[string]string{"role": "unknown or unassignable role"})
		}
	}
	if in.Status != nil && *in.Status != "active" && *in.Status != "suspended" {
		return Membership{}, apierror.Validation("invalid status", map[string]string{"status": "must be active or suspended"})
	}

	if _, err := s.repo.UpdateMembership(ctx, orgID, membershipID, in); err != nil {
		return Membership{}, err
	}

	actor := actorUserID
	if in.RoleKey != nil {
		_ = s.repo.RecordActivity(ctx, idgen.New(), orgID, &actor, "team.role_changed", "membership", membershipID)
	}
	if in.Status != nil {
		action := "team.suspended"
		if *in.Status == "active" {
			action = "team.reactivated"
		}
		_ = s.repo.RecordActivity(ctx, idgen.New(), orgID, &actor, action, "membership", membershipID)
	}
	return s.repo.GetMembership(ctx, orgID, membershipID)
}

func (s *Service) RemoveMember(ctx context.Context, orgID, actorUserID, membershipID string) error {
	if err := s.repo.DeleteMembership(ctx, orgID, membershipID); err != nil {
		return err
	}
	actor := actorUserID
	_ = s.repo.RecordActivity(ctx, idgen.New(), orgID, &actor, "team.removed", "membership", membershipID)
	return nil
}

func (s *Service) ListActivity(ctx context.Context, orgID string) ([]ActivityEntry, error) {
	return s.repo.ListActivity(ctx, orgID, activityListLimit)
}
