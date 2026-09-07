package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
	totpengine "github.com/ismagilovnail/flox/apps/internal/totp"
)

const (
	minPasswordLength = 8
	// pendingTOTPLoginTTL: how long a "password verified, second factor
	// still outstanding" bridge row (pending_totp_logins, migration
	// 00032) stays valid. Short and deliberately not configurable — this
	// is someone actively mid-login, not a long-lived credential; 5
	// minutes covers "grab your phone and read the code off it" with
	// margin, without leaving a half-authenticated token usable for long.
	pendingTOTPLoginTTL = 5 * time.Minute
	// inviteTTL: long enough that an invite sent on a Friday is still
	// good the following Monday.
	inviteTTL = 7 * 24 * time.Hour
	// emailVerificationTTL: shorter than inviteTTL on purpose — this link
	// goes to the email the signer-upper just typed into the form they're
	// still looking at, not one sent days later to a teammate who might
	// be away; 24h covers "check your email after lunch" without leaving
	// a long-lived unauthenticated bearer token sitting in an inbox.
	emailVerificationTTL = 24 * time.Hour
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

// Mailer is the narrow slice of apps/internal/mailer.Client this package
// needs — the same decoupled-interface-per-consumer pattern
// apps/internal/postbacklogs.OutgoingEnqueuer already uses, so this
// package never imports apps/internal/mailer directly (only apps/api's
// wiring does).
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// ReferralRecorder attributes a new organization's signup to whichever
// org's referral code it used, if any — implemented by
// apps/internal/referral.Service and wired in from main.go via
// SetReferralRecorder, not a constructor parameter, so auth never imports
// referral directly and every existing NewService call site (including
// every test) stays unchanged. A nil referrals field is a valid, silent
// no-op — referral bookkeeping is never a prerequisite for signup.
type ReferralRecorder interface {
	RecordSignup(ctx context.Context, code, referredOrgID, name, email string) error
}

type Service struct {
	repo      *Repository
	appURL    string
	mailer    Mailer
	referrals ReferralRecorder
	// totpKey is set via SetTOTPKey, not a NewService parameter — same
	// late-bound-optional-dependency pattern as referrals above, kept
	// consistent even though a missing key is a real misconfiguration
	// (every 2FA method checks for nil and fails loudly) rather than a
	// graceful no-op, so every existing NewService call site (including
	// every test not exercising 2FA) stays unchanged.
	totpKey []byte
}

// appURL is apps/web's own origin (config.AppURL) — an invite link, and
// now the email-verification link Signup sends, both point there.
func NewService(repo *Repository, appURL string, mailer Mailer) *Service {
	return &Service{repo: repo, appURL: appURL, mailer: mailer}
}

func (s *Service) SetReferralRecorder(r ReferralRecorder) {
	s.referrals = r
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

// Signup creates the organization, its Owner, and the Owner's membership —
// all real, immediately — but deliberately does NOT create a session or
// return one: the account can't sign in until it proves it controls the
// email it just typed in, by following the link this sends there. That
// proof is the entire point of email verification; returning a session
// (or the verification link itself) from this same response would let
// anyone who can POST here as anyone@somewhere-they-don't-control finish
// "verifying" without ever touching that inbox. See VerifyEmail.
func (s *Service) Signup(ctx context.Context, in SignupInput) (SignupResult, error) {
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
		return SignupResult{}, apierror.Validation("invalid signup", fields)
	}

	hash, err := hashPassword(in.Password)
	if err != nil {
		return SignupResult{}, err
	}

	token, tokenHash, err := newBearerToken()
	if err != nil {
		return SignupResult{}, err
	}
	expiresAt := time.Now().Add(emailVerificationTTL)

	orgID, userID, membershipID := idgen.New(), idgen.New(), idgen.New()
	if err := s.repo.CreateOrgWithOwner(ctx, orgID, in.OrganizationName, userID, in.Name, in.Email, hash, membershipID, tokenHash, expiresAt); err != nil {
		return SignupResult{}, err
	}

	if s.referrals != nil && in.ReferralCode != "" {
		// Best-effort, deliberately ignored on error: the account above
		// is already real and fully created regardless of whether
		// referral attribution succeeds. An invalid/unknown code is
		// already a silent no-op inside RecordSignup itself; only a
		// genuine infra failure would reach here, and even then this
		// signup must not fail over a secondary bookkeeping system.
		_ = s.referrals.RecordSignup(ctx, in.ReferralCode, orgID, in.Name, in.Email)
	}

	if err := s.sendVerificationEmail(ctx, in.Email, token); err != nil {
		// The account row already exists (unverified) even though this
		// request reports failure — a down/misconfigured relay is an ops
		// problem (apierror.Write logs the wrapped error server-side),
		// not something worth a half-successful response shape for. A
		// retried signup against the same email correctly 409s; recovery
		// from here is ResendVerification once the relay is back.
		return SignupResult{}, fmt.Errorf("auth: signup succeeded but sending the verification email failed: %w", err)
	}

	return SignupResult{
		User:         User{ID: userID, Name: in.Name, Email: in.Email},
		Organization: Organization{ID: orgID, Name: in.OrganizationName},
	}, nil
}

func (s *Service) sendVerificationEmail(ctx context.Context, to, token string) error {
	link := strings.TrimRight(s.appURL, "/") + "/verify-email?token=" + token
	subject := "Confirm your FLOX email"
	body := "Welcome to FLOX!\n\n" +
		"Confirm your email address to activate your workspace:\n" +
		link + "\n\n" +
		"This link expires in 24 hours. If you didn't create a FLOX account, you can ignore this email."
	return s.mailer.Send(ctx, to, subject, body)
}

func (s *Service) Login(ctx context.Context, in LoginInput) (LoginOutcome, error) {
	email := strings.TrimSpace(strings.ToLower(in.Email))

	row, found, err := s.repo.findUserByEmail(ctx, email)
	if err != nil {
		return LoginOutcome{}, err
	}
	hashToCheck := dummyPasswordHash
	if found {
		hashToCheck = row.PasswordHash
	}
	if !checkPassword(hashToCheck, in.Password) || !found {
		return LoginOutcome{}, apierror.Unauthorized("invalid email or password")
	}
	// Checked only after the password is confirmed correct — revealing
	// "unverified" to someone who doesn't already know the password would
	// leak account existence the same way a distinct "no such email"
	// error would (dummyPasswordHash above exists for exactly this
	// reason). Code "email_not_verified", not "unauthorized": apps/web's
	// login form renders a distinct "resend the verification email" CTA
	// for this one, not the generic wrong-password message.
	if row.EmailVerifiedAt == nil {
		return LoginOutcome{}, &apierror.Error{
			Status:  http.StatusForbidden,
			Code:    "email_not_verified",
			Message: "confirm your email before signing in — check your inbox for the verification link",
		}
	}

	memberships, err := s.repo.activeMembershipsByUser(ctx, row.ID)
	if err != nil {
		return LoginOutcome{}, err
	}
	if len(memberships) == 0 {
		return LoginOutcome{}, apierror.Unauthorized("no active organization membership")
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
			return LoginOutcome{}, apierror.Unauthorized("not a member of that organization")
		}
	case len(memberships) == 1:
		chosen = memberships[0]
	default:
		return LoginOutcome{}, apierror.Validation("multiple organizations available, specify organizationId", map[string]string{"organizationId": "required"})
	}

	return s.completeLogin(ctx, row.ID, chosen.OrganizationID, row.Name, row.Email, chosen.RoleKey, chosen.OrganizationName, in.ClientIP)
}

// createSession is the one place a real session row is ever written —
// timeoutMinutes comes from the org's own configurable §30 Security
// setting (organizations.session_timeout_minutes), not a flat package
// constant anymore; migration 00032's own DEFAULT 43200 (30 days)
// preserves every existing org's real behavior on day one.
func (s *Service) createSession(ctx context.Context, userID, orgID, userName, userEmail, roleKey, orgName string, timeoutMinutes int) (Result, error) {
	token, tokenHash, err := newBearerToken()
	if err != nil {
		return Result{}, err
	}
	expiresAt := time.Now().Add(time.Duration(timeoutMinutes) * time.Minute)
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

// VerifyEmail consumes a signup's emailed link — same one-time-token
// shape as AcceptInvite (find by hash, must be unexpired), but there's no
// separate "preview" step first (nothing to show before consuming: no
// name/role choice the way an invite has). Clicking the link IS proof of
// inbox access, exactly as strong as AcceptInvite's own proof (typing a
// fresh password into that same request) — so this auto-logs in the same
// way AcceptInvite does, not a separate "verified, now go log in by hand"
// step for the common case. A LoginOutcome with everything zeroed out
// (no Session, no PendingToken) means "verified, but auto-login wasn't
// attempted" — the account had zero or multiple active memberships at
// verification time (e.g. also separately accepted an invite in the
// interim); apps/web sends that case to /login instead of into the app.
func (s *Service) VerifyEmail(ctx context.Context, token, clientIP string) (LoginOutcome, error) {
	userID, name, email, err := s.repo.FindUserByVerificationToken(ctx, hashToken(token))
	if err != nil {
		return LoginOutcome{}, err
	}
	if err := s.repo.MarkEmailVerified(ctx, userID); err != nil {
		return LoginOutcome{}, err
	}

	memberships, err := s.repo.activeMembershipsByUser(ctx, userID)
	if err != nil {
		return LoginOutcome{}, err
	}
	if len(memberships) != 1 {
		return LoginOutcome{}, nil
	}

	return s.completeLogin(ctx, userID, memberships[0].OrganizationID, name, email, memberships[0].RoleKey, memberships[0].OrganizationName, clientIP)
}

// completeLogin is the one real chokepoint every account-entry path
// (Login, VerifyEmail, AcceptInvite) funnels through once password/
// invite-token/email-link proof has already succeeded — §30 Security's
// IP allowlist and 2FA policy are enforced exactly once, here, rather
// than duplicated per caller.
func (s *Service) completeLogin(ctx context.Context, userID, orgID, userName, userEmail, roleKey, orgName, clientIP string) (LoginOutcome, error) {
	sec, userHasTOTP, err := s.repo.LoginSecurityContext(ctx, userID, orgID)
	if err != nil {
		return LoginOutcome{}, err
	}

	if !ipAllowed(sec.IPAllowlist, clientIP) {
		return LoginOutcome{}, &apierror.Error{
			Status:  http.StatusForbidden,
			Code:    "ip_not_allowed",
			Message: "sign-in is not allowed from this network",
		}
	}

	if userHasTOTP || sec.TwoFactorRequired {
		token, tokenHash, err := newBearerToken()
		if err != nil {
			return LoginOutcome{}, err
		}
		expiresAt := time.Now().Add(pendingTOTPLoginTTL)
		requiresEnrollment := !userHasTOTP
		if err := s.repo.CreatePendingTOTPLogin(ctx, idgen.New(), userID, orgID, tokenHash, requiresEnrollment, expiresAt); err != nil {
			return LoginOutcome{}, err
		}
		return LoginOutcome{
			PendingToken:           token,
			RequiresTOTP:           userHasTOTP,
			RequiresTOTPEnrollment: requiresEnrollment,
		}, nil
	}

	res, err := s.createSession(ctx, userID, orgID, userName, userEmail, roleKey, orgName, sec.SessionTimeoutMinutes)
	if err != nil {
		return LoginOutcome{}, err
	}
	return LoginOutcome{Session: &res}, nil
}

// ipAllowed reports whether clientIP satisfies allowlist — an empty
// allowlist means "no restriction" (the only sane default for a feature
// nobody has configured). A clientIP that fails to parse is rejected
// once the list is non-empty (fail closed: a spoofed/malformed address
// must never bypass a restriction the org explicitly turned on), but
// never rejected while the list is empty (an unparsed IP must not break
// login for every org that hasn't touched this feature).
func ipAllowed(allowlist []string, clientIP string) bool {
	if len(allowlist) == 0 {
		return true
	}
	parsed := net.ParseIP(clientIP)
	if parsed == nil {
		return false
	}
	for _, entry := range allowlist {
		if strings.Contains(entry, "/") {
			_, ipnet, err := net.ParseCIDR(entry)
			if err == nil && ipnet.Contains(parsed) {
				return true
			}
			continue
		}
		if candidate := net.ParseIP(entry); candidate != nil && candidate.Equal(parsed) {
			return true
		}
	}
	return false
}

// ResendVerification issues a fresh verification token and re-sends the
// email — always nil-error and silent about whether the account exists
// or is already verified, the same anti-enumeration stance
// dummyPasswordHash gives Login: a caller who doesn't already know
// exactly which of "no such account" / "already verified" / "email just
// sent" happened can't use this endpoint to probe which emails are
// registered.
func (s *Service) ResendVerification(ctx context.Context, email string) error {
	email = strings.TrimSpace(strings.ToLower(email))

	userID, found, err := s.repo.FindUnverifiedUserByEmail(ctx, email)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	token, tokenHash, err := newBearerToken()
	if err != nil {
		return err
	}
	if err := s.repo.SetEmailVerificationToken(ctx, userID, tokenHash, time.Now().Add(emailVerificationTTL)); err != nil {
		return err
	}
	return s.sendVerificationEmail(ctx, email, token)
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

func (s *Service) AcceptInvite(ctx context.Context, in AcceptInviteInput) (LoginOutcome, error) {
	in.Name = strings.TrimSpace(in.Name)

	fields := map[string]string{}
	if in.Name == "" {
		fields["name"] = "required"
	}
	if len(in.Password) < minPasswordLength {
		fields["password"] = fmt.Sprintf("must be at least %d characters", minPasswordLength)
	}
	if len(fields) > 0 {
		return LoginOutcome{}, apierror.Validation("invalid", fields)
	}

	inv, err := s.repo.FindMembershipByInviteToken(ctx, hashToken(in.Token))
	if err != nil {
		return LoginOutcome{}, err
	}

	hash, err := hashPassword(in.Password)
	if err != nil {
		return LoginOutcome{}, err
	}
	if err := s.repo.AcceptInvite(ctx, inv.MembershipID, inv.UserID, in.Name, hash); err != nil {
		return LoginOutcome{}, err
	}

	actor := inv.UserID
	_ = s.repo.RecordActivity(ctx, idgen.New(), inv.OrganizationID, &actor, "team.invite_accepted", "membership", inv.MembershipID)
	return s.completeLogin(ctx, inv.UserID, inv.OrganizationID, in.Name, inv.Email, inv.RoleKey, inv.OrganizationName, in.ClientIP)
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

const (
	orgNameMinLen = 2
	orgNameMaxLen = 80
)

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

func (s *Service) GetOrganizationSettings(ctx context.Context, orgID string) (OrganizationSettings, error) {
	return s.repo.GetOrganizationSettings(ctx, orgID)
}

// UpdateOrganizationSettings validates timezone against Go's own IANA
// tzdata (broader and more honest than the frontend's own short
// dropdown of common zones — any real timezone an operator's browser
// reports should be acceptable, not just the eight the UI happens to
// list) and currency as a plausible ISO 4217-shaped code (3 uppercase
// letters) rather than a fixed list — apps/internal/conversion.
// PostgresFX already degrades gracefully (stores the original amount,
// no USD value) when no fx_rates row exists for a currency/date, so
// this never needs to double as "is this currency actually tradeable
// today."
func (s *Service) UpdateOrganizationSettings(ctx context.Context, orgID, actorUserID string, in UpdateOrganizationSettingsInput) (OrganizationSettings, error) {
	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		if len(trimmed) < orgNameMinLen {
			return OrganizationSettings{}, apierror.Validation("name must be at least 2 characters", map[string]string{"name": "too short"})
		}
		if len(trimmed) > orgNameMaxLen {
			return OrganizationSettings{}, apierror.Validation("name is too long", map[string]string{"name": "too long"})
		}
		in.Name = &trimmed
	}
	if in.Timezone != nil {
		if _, err := time.LoadLocation(*in.Timezone); err != nil {
			return OrganizationSettings{}, apierror.Validation("unknown timezone", map[string]string{"timezone": "invalid"})
		}
	}
	if in.Currency != nil {
		upper := strings.ToUpper(strings.TrimSpace(*in.Currency))
		if !currencyPattern.MatchString(upper) {
			return OrganizationSettings{}, apierror.Validation("currency must be a 3-letter code", map[string]string{"currency": "invalid"})
		}
		in.Currency = &upper
	}

	updated, err := s.repo.UpdateOrganizationSettings(ctx, orgID, in)
	if err != nil {
		return OrganizationSettings{}, err
	}
	actor := actorUserID
	_ = s.repo.RecordActivity(ctx, idgen.New(), orgID, &actor, "settings.updated", "organization", orgID)
	return updated, nil
}

func (s *Service) GetSecuritySettings(ctx context.Context, orgID string) (SecuritySettings, error) {
	return s.repo.GetSecuritySettings(ctx, orgID)
}

// UpdateSecuritySettings validates session_timeout_minutes against the
// same 15-86400 bound migration 00032's own CHECK constraint enforces (a
// clean 422 here beats a raw constraint-violation error reaching the
// client) — deliberately wider than the frontend panel's own curated
// dropdown range, same "backend validates broadly, frontend curates a
// shortlist" precedent Organization settings' timezone/currency already
// established: the real default (43200, matching the pre-existing
// hardcoded 30-day session TTL) must itself remain a valid value to set
// back to, not just a valid value to have inherited. Every IPAllowlist
// entry is checked as a real IP or CIDR (net.ParseIP/net.ParseCIDR — not
// a regex, so "999.999.999.999"-shaped garbage that merely looks right
// is still rejected).
//
// The one safeguard against the feature's own real lockout risk: a
// non-empty new allowlist must cover actingClientIP, the IP the request
// making THIS change is coming from — reject the update otherwise. This
// only protects the person saving the list from immediately locking out
// their own current login; it deliberately does not (and structurally
// cannot) guarantee no OTHER admin on a different network gets excluded
// — that remains a real, visible consequence of the policy an Owner is
// choosing to set, the same way a cloud security-group console warns
// but still allows a narrow rule.
func (s *Service) UpdateSecuritySettings(ctx context.Context, orgID, actorUserID, actingClientIP string, in UpdateSecuritySettingsInput) (SecuritySettings, error) {
	if in.SessionTimeoutMinutes != nil {
		if *in.SessionTimeoutMinutes < 15 || *in.SessionTimeoutMinutes > 86400 {
			return SecuritySettings{}, apierror.Validation("session timeout must be 15-86400 minutes", map[string]string{"sessionTimeoutMinutes": "out of range"})
		}
	}
	if in.IPAllowlist != nil {
		for _, entry := range in.IPAllowlist {
			if !validIPOrCIDR(entry) {
				return SecuritySettings{}, apierror.Validation("invalid IP or CIDR", map[string]string{"ipAllowlist": entry})
			}
		}
		if len(in.IPAllowlist) > 0 && !ipAllowed(in.IPAllowlist, actingClientIP) {
			return SecuritySettings{}, apierror.Validation(
				"this list would lock out your own current connection — add your own IP first",
				map[string]string{"ipAllowlist": "excludes your current IP"},
			)
		}
	}

	updated, err := s.repo.UpdateSecuritySettings(ctx, orgID, in)
	if err != nil {
		return SecuritySettings{}, err
	}
	actor := actorUserID
	_ = s.repo.RecordActivity(ctx, idgen.New(), orgID, &actor, "settings.updated", "organization", orgID)
	return updated, nil
}

func validIPOrCIDR(entry string) bool {
	if strings.Contains(entry, "/") {
		_, _, err := net.ParseCIDR(entry)
		return err == nil
	}
	return net.ParseIP(entry) != nil
}

// --- §30 Security: real per-user TOTP two-factor authentication ---

func (s *Service) SetTOTPKey(key []byte) {
	s.totpKey = key
}

func (s *Service) TOTPStatus(ctx context.Context, userID string) (bool, error) {
	_, enabled, err := s.repo.GetUserTOTPSecret(ctx, userID)
	return enabled, err
}

// EnrollTOTP generates a brand-new secret + QR code + backup codes,
// encrypts and persists the secret as UNCONFIRMED (totp_enabled_at left
// NULL — see repository.SetPendingUserTOTPSecret's own comment), and
// replaces any previous backup codes immediately (a retried enrollment
// invalidates whatever the last attempt generated, confirmed or not —
// there's never a moment where two generations' worth of codes are both
// valid).
func (s *Service) EnrollTOTP(ctx context.Context, userID, accountEmail string) (TOTPEnrollment, error) {
	if s.totpKey == nil {
		return TOTPEnrollment{}, fmt.Errorf("auth: TOTP encryption key not configured")
	}
	enrollment, err := totpengine.GenerateEnrollment("FLOX", accountEmail)
	if err != nil {
		return TOTPEnrollment{}, err
	}
	encrypted, err := totpengine.EncryptSecret(s.totpKey, enrollment.Secret)
	if err != nil {
		return TOTPEnrollment{}, err
	}
	if err := s.repo.SetPendingUserTOTPSecret(ctx, userID, encrypted); err != nil {
		return TOTPEnrollment{}, err
	}

	hashed := make([]string, len(enrollment.BackupCodes))
	for i, code := range enrollment.BackupCodes {
		hashed[i] = totpengine.HashBackupCode(code)
	}
	if err := s.repo.ReplaceBackupCodes(ctx, userID, hashed); err != nil {
		return TOTPEnrollment{}, err
	}

	return TOTPEnrollment{
		Secret:        enrollment.Secret,
		QRCodeDataURI: "data:image/png;base64," + base64.StdEncoding.EncodeToString(enrollment.QRCodePNG),
		BackupCodes:   enrollment.BackupCodes,
	}, nil
}

// ConfirmTOTP verifies code against the pending (unconfirmed) secret
// EnrollTOTP just stored and, on success, marks it enabled. Returns
// apierror.Validation on a wrong code — the caller (handler) decides
// whether that's a self-serve retry or a forced-enrollment-mid-login
// retry; either way the pending secret is left in place so retrying
// costs nothing.
func (s *Service) ConfirmTOTP(ctx context.Context, userID, code string) error {
	if s.totpKey == nil {
		return fmt.Errorf("auth: TOTP encryption key not configured")
	}
	encrypted, enabled, err := s.repo.GetUserTOTPSecret(ctx, userID)
	if err != nil {
		return err
	}
	if encrypted == "" || enabled {
		return apierror.Validation("no pending TOTP enrollment", map[string]string{"code": "no enrollment in progress"})
	}
	secret, err := totpengine.DecryptSecret(s.totpKey, encrypted)
	if err != nil {
		return err
	}
	if !totpengine.ValidateCode(secret, code) {
		return apierror.Validation("incorrect code", map[string]string{"code": "incorrect"})
	}
	return s.repo.ConfirmUserTOTP(ctx, userID)
}

// DisableTOTP requires the account's own current password (a
// destructive-enough action — losing 2FA protection — that re-proving
// identity is warranted even for an already-authenticated session) and
// refuses outright while the org's own two_factor_required policy is on:
// an individual member cannot opt out of a workspace-wide requirement.
func (s *Service) DisableTOTP(ctx context.Context, userID, orgID, password string) error {
	sec, err := s.repo.GetSecuritySettings(ctx, orgID)
	if err != nil {
		return err
	}
	if sec.TwoFactorRequired {
		return apierror.Validation("two-factor authentication is required by your organization and can't be disabled", nil)
	}

	row, found, err := s.repo.findUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if !found || !checkPassword(row.PasswordHash, password) {
		return apierror.Unauthorized("incorrect password")
	}

	return s.repo.DisableUserTOTP(ctx, userID)
}

// VerifyPendingLogin exchanges a PendingToken (from Login/VerifyEmail/
// AcceptInvite's LoginOutcome) plus a live TOTP code or a backup code for
// a real session — the only place completeLogin's "second factor still
// outstanding" bridge ever resolves into an actual session.
func (s *Service) VerifyPendingLogin(ctx context.Context, pendingToken, code string) (Result, error) {
	if s.totpKey == nil {
		return Result{}, fmt.Errorf("auth: TOTP encryption key not configured")
	}
	pending, err := s.repo.FindPendingTOTPLogin(ctx, hashToken(pendingToken))
	if err != nil {
		return Result{}, err
	}
	if pending.RequiresEnrollment {
		return Result{}, apierror.Validation("complete enrollment first", map[string]string{"code": "enrollment required"})
	}

	encrypted, enabled, err := s.repo.GetUserTOTPSecret(ctx, pending.UserID)
	if err != nil {
		return Result{}, err
	}
	if !enabled {
		return Result{}, apierror.Validation("no confirmed TOTP enrollment", nil)
	}
	secret, err := totpengine.DecryptSecret(s.totpKey, encrypted)
	if err != nil {
		return Result{}, err
	}

	valid := totpengine.ValidateCode(secret, code)
	if !valid {
		if consumed, err := s.repo.ConsumeBackupCode(ctx, pending.UserID, totpengine.HashBackupCode(code)); err != nil {
			return Result{}, err
		} else {
			valid = consumed
		}
	}
	if !valid {
		// Invalidate on the first wrong code rather than leaving the
		// window open for repeated guesses: the pending row is the only
		// thing standing between a 6-digit TOTP code and a session, and
		// its 5-minute TTL (pendingTOTPLoginTTL) is otherwise long enough
		// for an unlimited number of attempts against one token (security
		// audit finding #3). The cost is a genuine typo forcing a fresh
		// login, same trade CLAUDE.md already makes for VerifyEmail/invite
		// tokens elsewhere in this file.
		_ = s.repo.DeletePendingTOTPLogin(ctx, pending.ID)
		return Result{}, apierror.Unauthorized("incorrect code")
	}

	who, err := s.repo.WhoAmI(ctx, pending.UserID, pending.OrganizationID)
	if err != nil {
		return Result{}, err
	}
	sec, err := s.repo.GetSecuritySettings(ctx, pending.OrganizationID)
	if err != nil {
		return Result{}, err
	}
	_ = s.repo.DeletePendingTOTPLogin(ctx, pending.ID)
	return s.createSession(ctx, who.UserID, pending.OrganizationID, who.UserName, who.UserEmail, who.RoleKey, who.OrganizationName, sec.SessionTimeoutMinutes)
}

// ResolvePendingLoginSubject identifies who a pendingToken belongs to
// without consuming it — TOTPHandler's dual-context enroll/confirm
// endpoints need to know "who is this for" before deciding what to do,
// separately from actually completing the login.
func (s *Service) ResolvePendingLoginSubject(ctx context.Context, pendingToken string) (userID, orgID string, err error) {
	pending, err := s.repo.FindPendingTOTPLogin(ctx, hashToken(pendingToken))
	if err != nil {
		return "", "", err
	}
	return pending.UserID, pending.OrganizationID, nil
}

// CompleteEnrollmentLogin is ConfirmTOTP's forced-enrollment-mid-login
// sibling: verifies the code against the pending secret exactly like
// ConfirmTOTP, but on success also completes the login that triggered
// enrollment in the first place (the org's two_factor_required policy),
// deleting the pending row and returning a real session in one step —
// the frontend never gets a "confirmed, now please log in again" dead
// end.
func (s *Service) CompleteEnrollmentLogin(ctx context.Context, pendingToken, code string) (Result, error) {
	pending, err := s.repo.FindPendingTOTPLogin(ctx, hashToken(pendingToken))
	if err != nil {
		return Result{}, err
	}
	if !pending.RequiresEnrollment {
		return Result{}, apierror.Validation("already enrolled — enter your code instead", nil)
	}
	if err := s.ConfirmTOTP(ctx, pending.UserID, code); err != nil {
		return Result{}, err
	}

	who, err := s.repo.WhoAmI(ctx, pending.UserID, pending.OrganizationID)
	if err != nil {
		return Result{}, err
	}
	sec, err := s.repo.GetSecuritySettings(ctx, pending.OrganizationID)
	if err != nil {
		return Result{}, err
	}
	_ = s.repo.DeletePendingTOTPLogin(ctx, pending.ID)
	return s.createSession(ctx, who.UserID, pending.OrganizationID, who.UserName, who.UserEmail, who.RoleKey, who.OrganizationName, sec.SessionTimeoutMinutes)
}
