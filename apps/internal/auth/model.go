// Package auth implements §52/Phase 28 — authentication, sessions, and
// role-based authorization for organizations and their memberships.
// handler → service → repository, same layering as every other domain.
//
// Scope confirmed via AskUserQuestion before this phase started:
//   - Server-side sessions in an HTTP-only cookie, not JWTs — trivially
//     revocable, no rotation/denylist machinery (see migration 00020's own
//     comment on the sessions table).
//   - Inviting a team member generates a shareable accept-invite link
//     (a token embedded in a URL apps/web will render/copy in Phase 28B),
//     not a real sent email — this project has no SMTP/email-provider
//     integration anywhere, the same situation Ad Account Connections hit
//     with OAuth (no registered app to redirect through) and resolved the
//     same way: build the real mechanism up to the boundary that requires
//     infrastructure this environment doesn't have.
//   - RBAC permission enforcement (RequirePermission below) is wired onto
//     this package's own team/membership endpoints only. Retrofitting it
//     onto every pre-existing domain route (campaigns, offers, sources,
//     ...) was confirmed via AskUserQuestion as an explicit, separate
//     follow-up — a wide, mechanical sweep across ~15 already-shipped
//     packages that deserves its own reviewable phase, not a silent
//     expansion of this one. Tenant isolation (§36-TENANCY: no org ever
//     sees another org's data) is unaffected either way — it was already
//     enforced everywhere via org-scoped repositories and is orthogonal to
//     role-based permission checks within one org.
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

// Membership is one user's standing within one org — the join row
// memberships/roles/users produce, denormalized for the team list/activity
// UI Phase 28B wires this to.
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

type SignupInput struct {
	OrganizationName string
	Name             string
	Email            string
	Password         string
}

// LoginInput.OrganizationID is optional: required only to disambiguate a
// user who holds an active membership in more than one org (rare —
// nothing in apps/web exposes multi-org membership yet, but memberships
// are many-to-many by schema, so login must handle it rather than pick
// arbitrarily).
type LoginInput struct {
	Email          string
	Password       string
	OrganizationID string
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
}

// InvitePreview is what apps/web's future accept-invite page (Phase 28B)
// shows before the invitee sets a password — deliberately returned by an
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
