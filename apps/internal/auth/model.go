// Package auth is authentication, sessions and role-based authorization
// for organizations and their memberships (§52); handler → service →
// repository like every domain.
//
//   - Sessions are server-side, in an HTTP-only cookie, not JWTs: revocable
//     with no rotation or denylist machinery.
//   - An invite is a link the admin copies and passes on; sign-up
//     verification is a sent email (Mailer).
//   - RequirePermission gates routes by role. Tenant isolation (§36) is
//     separate: org-scoped repositories, whatever the role.
package auth

import "time"

// User is a person who can sign in. Password hash is deliberately absent
// from this struct — same "never let the credential leave the repository
// layer as a Go value a handler could accidentally serialize" precedent as
// adaccount.Connection excluding AccessToken.
type User struct {
	ID        string
	Email     string
	Name      string
	CreatedAt time.Time
}

type Organization struct {
	ID   string
	Name string
}

// OrganizationSettings is the fuller shape the Settings > Organization
// panel reads/writes (§30) — kept separate from Organization (the
// minimal {id, name} embedded in every auth response) rather than
// widening that shared shape for one feature's needs. Timezone/
// Currency already exist as real organizations columns (migration
// 00001) — this is the first real reader/writer of them.
type OrganizationSettings struct {
	ID       string
	Name     string
	Timezone string
	Currency string
}

type UpdateOrganizationSettingsInput struct {
	Name     *string
	Timezone *string
	Currency *string
}

// SecuritySettings backs the Settings > Security panel (§30; the spec has
// no design for it, the UI's shape is the design). It lives on the
// organizations row like OrganizationSettings (migration 00032) but is its
// own type and handler: another panel, other validation, and the lockout
// risk an IP allowlist brings.
type SecuritySettings struct {
	SessionTimeoutMinutes int
	IPAllowlist           []string
	TwoFactorRequired     bool
}

type UpdateSecuritySettingsInput struct {
	SessionTimeoutMinutes *int
	IPAllowlist           []string
	TwoFactorRequired     *bool
}

// TOTPEnrollment is what EnrollTOTP hands back — shown to the user
// exactly once. QRCodeDataURI is a ready-to-render `data:image/png;
// base64,...` string (apps/internal/totp renders the PNG server-side —
// the secret never needs to leave this process to reach a third-party
// QR-rendering service).
type TOTPEnrollment struct {
	Secret        string
	QRCodeDataURI string
	BackupCodes   []string
}

// Membership is one user's standing within one org — the join row
// memberships/roles/users produce, denormalized for the team list and
// activity UI.
type Membership struct {
	ID           string
	UserID       string
	UserName     string
	UserEmail    string
	RoleKey      string
	Status       string // active | invited | suspended
	InvitedAt    time.Time
	LastActiveAt *time.Time
}

// ReferralCode is optional and best-effort — an invalid/unknown code never
// blocks or fails a real account signup (see Service.Signup and the
// ReferralRecorder interface below).
type SignupInput struct {
	OrganizationName string
	Name             string
	Email            string
	Password         string
	ReferralCode     string
}

// SignupResult is what Signup returns — deliberately NOT Result: no
// session token, because no session exists yet. The account can't sign in
// until VerifyEmail consumes the link Signup emails to Email — see
// service.go's own comment on Signup for why (the whole point of this is
// proving the signer-upper controls that inbox; handing back a session,
// or the verification link itself, on this same response would defeat
// that entirely).
type SignupResult struct {
	User         User
	Organization Organization
}

// LoginInput.OrganizationID is optional: required only to disambiguate a
// user who holds an active membership in more than one org (rare —
// nothing in apps/web exposes multi-org membership yet, but memberships
// are many-to-many by schema, so login must handle it rather than pick
// arbitrarily). ClientIP feeds the org's IP allowlist check (§30
// Security), from clientip.Resolver like the rate limits: a spoofable
// address would make the allowlist decorative.
type LoginInput struct {
	Email          string
	Password       string
	OrganizationID string
	ClientIP       string
}

type InviteInput struct {
	Email   string
	Name    string
	RoleKey string
}

// UpdateMembershipInput fields are pointers so PATCH's "only touch what
// was sent" semantics (this project's existing convention, e.g.
// campaign.UpdateInput) apply here too — a role-only change must not
// require also re-sending status, and vice versa.
type UpdateMembershipInput struct {
	RoleKey *string
	Status  *string
}

type AcceptInviteInput struct {
	Token    string
	Name     string
	Password string
	ClientIP string
}

// InvitePreview is what apps/web's accept-invite page shows before the
// invitee sets a password — deliberately returned by an
// unauthenticated, token-guarded endpoint, so that page can render "You've
// been invited to join {OrganizationName} as {RoleKey}" without the
// invitee needing to be signed in yet (they can't be — they have no
// password).
type InvitePreview struct {
	OrganizationName string
	Email            string
	RoleKey          string
}

// Result is what signup/login/accept-invite all return: enough for the
// handler to set the session cookie (Token/ExpiresAt) and enough for the
// client to render "who am I, in which org, as what role" without a
// second round trip.
type Result struct {
	Token        string
	ExpiresAt    time.Time
	User         User
	Organization Organization
	RoleKey      string
}

// LoginOutcome is what every account-entry path that can grant a session
// (Login, VerifyEmail's auto-login, AcceptInvite) actually resolves to
// once §30 Security is in play — a real session is only one of three
// possibilities now:
//   - Session set: no second factor needed.
//   - RequiresTOTP: this user already has TOTP enabled — PendingToken
//     must be exchanged at POST /auth/login/totp along with a live code
//     or a backup code.
//   - RequiresTOTPEnrollment: the org's own two_factor_required policy
//     is on and this user hasn't set up TOTP yet — PendingToken must be
//     used with POST /auth/2fa/enroll then /auth/2fa/confirm before a
//     session is granted.
//
// PendingToken rows live in `pending_totp_logins`, a table completely
// separate from `sessions` — see migration 00032's own doc comment for
// why that separation is load-bearing, not incidental.
type LoginOutcome struct {
	Session                *Result
	PendingToken           string
	RequiresTOTP           bool
	RequiresTOTPEnrollment bool
}

// ActivityEntry is one audit_logs row (migration 00010) — populated by
// this package's own membership actions only (invited/role changed/
// removed/suspended/reactivated), not a cross-domain audit trail. Sweeping
// audit-log writes into every other domain's write path is out of scope
// here for the same reason the RBAC sweep is (see package doc comment).
type ActivityEntry struct {
	ID         string
	Action     string
	EntityType string
	EntityID   string
	ActorName  *string
	CreatedAt  time.Time
}
