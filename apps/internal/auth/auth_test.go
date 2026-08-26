package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/auth"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/logging"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

func mustPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connecting to database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newService(pool *pgxpool.Pool) *auth.Service {
	return auth.NewService(auth.NewRepository(pool), "http://localhost:3000")
}

// signupOrg is the one way this package's tests create an org — no
// separate SQL seeder, because Signup itself is the code under test for
// org creation, and every other test needs a real signed-up owner anyway.
// Registers cleanup deleting the organization (cascades to its users/
// memberships/sessions, migration 00001/00020's ON DELETE CASCADE) so
// repeated test runs against a real, persistent dev Postgres don't leave
// permanent rows behind — every other package's own seedOrg helper
// (adaccount_test.go, costsync_test.go, ...) does the same.
func signupOrg(t *testing.T, ctx context.Context, pool *pgxpool.Pool, svc *auth.Service, orgName, ownerEmail string) auth.Result {
	t.Helper()
	res, err := svc.Signup(ctx, auth.SignupInput{
		OrganizationName: orgName,
		Name:             "Test Owner",
		Email:            ownerEmail,
		Password:         "correct-horse-battery-staple",
	})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, res.Organization.ID)
	})
	return res
}

// uniqueEmail is deliberately all-lowercase (idgen.New()'s Crockford
// base32 output is uppercase) so tests can compare a returned email
// against this exact string without separately accounting for Signup/
// Invite's own lowercase normalization — TestLoginSucceedsAndFailsCorrectly's
// "case-insensitive email" case constructs its own uppercased variant
// specifically to exercise that normalization.
//
// Every real user account this test file ever creates (a signupOrg
// Owner, or an Invite's shell user) traces back to one of these calls —
// signupOrg's own cleanup only cascades an org's memberships/sessions
// away (organizations has no FK from users, so a user row survives its
// org being deleted), so this is the one place that also deletes the
// underlying users row, covering both cases with no separate tracking.
func uniqueEmail(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	email := "test-" + lower(idgen.New()) + "@example.com"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE lower(email) = lower($1)`, email)
	})
	return email
}

func lower(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + 32
		}
	}
	return string(out)
}

func TestSignupCreatesOrgOwnerAndSession(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()

	res := signupOrg(t, ctx, pool, svc, "Acme Traffic", uniqueEmail(t, pool))

	if res.RoleKey != "Owner" {
		t.Fatalf("RoleKey = %q, want Owner", res.RoleKey)
	}
	if res.Token == "" {
		t.Fatal("Signup returned an empty session token")
	}
	if res.Organization.Name != "Acme Traffic" {
		t.Fatalf("Organization.Name = %q, want Acme Traffic", res.Organization.Name)
	}

	orgID, userID, err := svc.ResolveSession(ctx, res.Token)
	if err != nil {
		t.Fatalf("ResolveSession on a freshly-issued token: %v", err)
	}
	if orgID != res.Organization.ID || userID != res.User.ID {
		t.Fatalf("ResolveSession = (%s, %s), want (%s, %s)", orgID, userID, res.Organization.ID, res.User.ID)
	}
}

func TestSignupRejectsDuplicateEmail(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	email := uniqueEmail(t, pool)

	signupOrg(t, ctx, pool, svc, "First Org", email)

	if _, err := svc.Signup(ctx, auth.SignupInput{OrganizationName: "Second Org", Name: "Someone Else", Email: email, Password: "another-long-password"}); err == nil {
		t.Fatal("Signup with an already-registered email succeeded, want a conflict error")
	}
}

func TestSignupValidatesInput(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()

	cases := []auth.SignupInput{
		{OrganizationName: "", Name: "A", Email: uniqueEmail(t, pool), Password: "longenough1"},
		{OrganizationName: "Org", Name: "", Email: uniqueEmail(t, pool), Password: "longenough1"},
		{OrganizationName: "Org", Name: "A", Email: "not-an-email", Password: "longenough1"},
		{OrganizationName: "Org", Name: "A", Email: uniqueEmail(t, pool), Password: "short"},
	}
	for i, in := range cases {
		if _, err := svc.Signup(ctx, in); err == nil {
			t.Fatalf("case %d: Signup with invalid input succeeded, want a validation error", i)
		}
	}
}

func TestLoginSucceedsAndFailsCorrectly(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	email := uniqueEmail(t, pool)
	signupOrg(t, ctx, pool, svc, "Login Org", email)

	t.Run("correct password", func(t *testing.T) {
		res, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: "correct-horse-battery-staple"})
		if err != nil {
			t.Fatalf("Login: %v", err)
		}
		if res.RoleKey != "Owner" {
			t.Fatalf("RoleKey = %q, want Owner", res.RoleKey)
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		if _, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: "totally-wrong-password"}); err == nil {
			t.Fatal("Login with the wrong password succeeded, want an error")
		}
	})

	t.Run("unknown email", func(t *testing.T) {
		if _, err := svc.Login(ctx, auth.LoginInput{Email: uniqueEmail(t, pool), Password: "whatever-password"}); err == nil {
			t.Fatal("Login with an unregistered email succeeded, want an error")
		}
	})

	t.Run("case-insensitive email", func(t *testing.T) {
		res, err := svc.Login(ctx, auth.LoginInput{Email: "  " + upper(email) + "  ", Password: "correct-horse-battery-staple"})
		if err != nil {
			t.Fatalf("Login with a differently-cased, whitespace-padded email: %v", err)
		}
		if res.User.Email != email {
			t.Fatalf("User.Email = %q, want %q", res.User.Email, email)
		}
	})
}

func upper(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'a' && r <= 'z' {
			out[i] = r - 32
		}
	}
	return string(out)
}

func TestLogout(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	res := signupOrg(t, ctx, pool, svc, "Logout Org", uniqueEmail(t, pool))

	if err := svc.Logout(ctx, res.Token); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, _, err := svc.ResolveSession(ctx, res.Token); err == nil {
		t.Fatal("ResolveSession succeeded after Logout, want an error")
	}
}

func TestResolveSessionRejectsGarbageToken(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()

	if _, _, err := svc.ResolveSession(ctx, "not-a-real-token"); err == nil {
		t.Fatal("ResolveSession accepted a token that was never issued")
	}
}

func TestMeReturnsRoleAndPermissions(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	res := signupOrg(t, ctx, pool, svc, "Me Org", uniqueEmail(t, pool))

	who, err := svc.Me(ctx, res.User.ID, res.Organization.ID)
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if who.RoleKey != "Owner" {
		t.Fatalf("RoleKey = %q, want Owner", who.RoleKey)
	}
	found := map[string]bool{}
	for _, p := range who.Permissions {
		found[p] = true
	}
	for _, want := range []string{"campaign.write", "team.write", "settings.write"} {
		if !found[want] {
			t.Fatalf("Owner's permissions %v missing %q", who.Permissions, want)
		}
	}
}

func TestInviteAcceptAndPermissions(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	owner := signupOrg(t, ctx, pool, svc, "Invite Org", uniqueEmail(t, pool))
	invitedEmail := uniqueEmail(t, pool)

	inviteURL, err := svc.Invite(ctx, owner.Organization.ID, owner.User.ID, auth.InviteInput{Name: "Invited Analyst", Email: invitedEmail, RoleKey: "Analyst"})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	token := tokenFromURL(t, inviteURL)

	preview, err := svc.PreviewInvite(ctx, token)
	if err != nil {
		t.Fatalf("PreviewInvite: %v", err)
	}
	if preview.Email != invitedEmail || preview.RoleKey != "Analyst" || preview.OrganizationName != "Invite Org" {
		t.Fatalf("PreviewInvite = %+v, want matching email/role/org", preview)
	}

	members, err := svc.ListMembers(ctx, owner.Organization.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("ListMembers = %d members, want 2 (owner + invited)", len(members))
	}
	var invited *auth.Membership
	for i := range members {
		if members[i].UserEmail == invitedEmail {
			invited = &members[i]
		}
	}
	if invited == nil || invited.Status != "invited" {
		t.Fatalf("invited member not found or wrong status: %+v", members)
	}

	res, err := svc.AcceptInvite(ctx, auth.AcceptInviteInput{Token: token, Name: "Invited Analyst", Password: "another-long-password"})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if res.RoleKey != "Analyst" || res.Organization.ID != owner.Organization.ID {
		t.Fatalf("AcceptInvite result = %+v, want role Analyst in org %s", res, owner.Organization.ID)
	}

	// The invited user can now log in with the password they just set.
	if _, err := svc.Login(ctx, auth.LoginInput{Email: invitedEmail, Password: "another-long-password"}); err != nil {
		t.Fatalf("Login after accepting invite: %v", err)
	}

	// Analyst has analytics.read but not campaign.write or team.write.
	who, err := svc.Me(ctx, res.User.ID, res.Organization.ID)
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	perms := map[string]bool{}
	for _, p := range who.Permissions {
		perms[p] = true
	}
	if !perms["analytics.read"] {
		t.Fatalf("Analyst missing analytics.read: %v", who.Permissions)
	}
	if perms["campaign.write"] || perms["team.write"] {
		t.Fatalf("Analyst unexpectedly has a write permission: %v", who.Permissions)
	}

	// The token cannot be replayed — accepting twice fails.
	if _, err := svc.AcceptInvite(ctx, auth.AcceptInviteInput{Token: token, Name: "Invited Analyst", Password: "another-long-password"}); err == nil {
		t.Fatal("AcceptInvite succeeded twice with the same token, want an error the second time")
	}
}

func TestInviteRejectsOwnerRoleAndDuplicateMember(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	owner := signupOrg(t, ctx, pool, svc, "Dup Org", uniqueEmail(t, pool))

	if _, err := svc.Invite(ctx, owner.Organization.ID, owner.User.ID, auth.InviteInput{Name: "X", Email: uniqueEmail(t, pool), RoleKey: "Owner"}); err == nil {
		t.Fatal("Invite with role=Owner succeeded, want a validation error")
	}

	email := uniqueEmail(t, pool)
	if _, err := svc.Invite(ctx, owner.Organization.ID, owner.User.ID, auth.InviteInput{Name: "First", Email: email, RoleKey: "Viewer"}); err != nil {
		t.Fatalf("first Invite: %v", err)
	}
	if _, err := svc.Invite(ctx, owner.Organization.ID, owner.User.ID, auth.InviteInput{Name: "First Again", Email: email, RoleKey: "Buyer"}); err == nil {
		t.Fatal("inviting the same email twice to the same org succeeded, want a conflict")
	}
}

func TestResendInviteInvalidatesThePreviousToken(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	owner := signupOrg(t, ctx, pool, svc, "Resend Org", uniqueEmail(t, pool))

	inviteURL, err := svc.Invite(ctx, owner.Organization.ID, owner.User.ID, auth.InviteInput{Name: "Bob", Email: uniqueEmail(t, pool), RoleKey: "Viewer"})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	oldToken := tokenFromURL(t, inviteURL)

	members, err := svc.ListMembers(ctx, owner.Organization.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	var membershipID string
	for _, m := range members {
		if m.Status == "invited" {
			membershipID = m.ID
		}
	}
	if membershipID == "" {
		t.Fatal("no invited membership found")
	}

	newURL, err := svc.ResendInvite(ctx, owner.Organization.ID, owner.User.ID, membershipID)
	if err != nil {
		t.Fatalf("ResendInvite: %v", err)
	}
	newToken := tokenFromURL(t, newURL)

	if _, err := svc.PreviewInvite(ctx, oldToken); err == nil {
		t.Fatal("the old invite token still resolves after ResendInvite, want it invalidated")
	}
	if _, err := svc.PreviewInvite(ctx, newToken); err != nil {
		t.Fatalf("PreviewInvite with the freshly-resent token: %v", err)
	}
}

func TestUpdateMembershipRoleAndSuspend(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	owner := signupOrg(t, ctx, pool, svc, "Update Org", uniqueEmail(t, pool))

	inviteURL, err := svc.Invite(ctx, owner.Organization.ID, owner.User.ID, auth.InviteInput{Name: "Carol", Email: uniqueEmail(t, pool), RoleKey: "Viewer"})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	accepted, err := svc.AcceptInvite(ctx, auth.AcceptInviteInput{Token: tokenFromURL(t, inviteURL), Name: "Carol", Password: "carols-long-password"})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}

	members, err := svc.ListMembers(ctx, owner.Organization.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	var membershipID string
	for _, m := range members {
		if m.UserID == accepted.User.ID {
			membershipID = m.ID
		}
	}

	newRole := "Manager"
	updated, err := svc.UpdateMembership(ctx, owner.Organization.ID, owner.User.ID, membershipID, auth.UpdateMembershipInput{RoleKey: &newRole})
	if err != nil {
		t.Fatalf("UpdateMembership role change: %v", err)
	}
	if updated.RoleKey != "Manager" {
		t.Fatalf("RoleKey after update = %q, want Manager", updated.RoleKey)
	}

	suspended := "suspended"
	if _, err := svc.UpdateMembership(ctx, owner.Organization.ID, owner.User.ID, membershipID, auth.UpdateMembershipInput{Status: &suspended}); err != nil {
		t.Fatalf("UpdateMembership suspend: %v", err)
	}

	// Suspension revokes the existing session immediately.
	if _, _, err := svc.ResolveSession(ctx, accepted.Token); err == nil {
		t.Fatal("a suspended member's session still resolves, want it revoked")
	}

	activity, err := svc.ListActivity(ctx, owner.Organization.ID)
	if err != nil {
		t.Fatalf("ListActivity: %v", err)
	}
	actions := map[string]bool{}
	for _, a := range activity {
		actions[a.Action] = true
	}
	for _, want := range []string{"team.invited", "team.invite_accepted", "team.role_changed", "team.suspended"} {
		if !actions[want] {
			t.Fatalf("activity log missing action %q, got %+v", want, activity)
		}
	}
}

func TestUpdateMembershipRejectsAssigningOwnerRole(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	owner := signupOrg(t, ctx, pool, svc, "Owner Role Org", uniqueEmail(t, pool))

	inviteURL, _ := svc.Invite(ctx, owner.Organization.ID, owner.User.ID, auth.InviteInput{Name: "Dave", Email: uniqueEmail(t, pool), RoleKey: "Viewer"})
	accepted, err := svc.AcceptInvite(ctx, auth.AcceptInviteInput{Token: tokenFromURL(t, inviteURL), Name: "Dave", Password: "daves-long-password"})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	members, _ := svc.ListMembers(ctx, owner.Organization.ID)
	var membershipID string
	for _, m := range members {
		if m.UserID == accepted.User.ID {
			membershipID = m.ID
		}
	}

	ownerRole := "Owner"
	if _, err := svc.UpdateMembership(ctx, owner.Organization.ID, owner.User.ID, membershipID, auth.UpdateMembershipInput{RoleKey: &ownerRole}); err == nil {
		t.Fatal("UpdateMembership assigned the Owner role, want a validation error")
	}
}

func TestCannotModifyOrRemoveTheOwner(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	owner := signupOrg(t, ctx, pool, svc, "Protect Owner Org", uniqueEmail(t, pool))

	members, err := svc.ListMembers(ctx, owner.Organization.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("ListMembers = %d, want 1 (just the Owner)", len(members))
	}
	ownerMembershipID := members[0].ID

	newRole := "Admin"
	if _, err := svc.UpdateMembership(ctx, owner.Organization.ID, owner.User.ID, ownerMembershipID, auth.UpdateMembershipInput{RoleKey: &newRole}); err == nil {
		t.Fatal("changed the Owner's own role, want a validation error")
	}
	if err := svc.RemoveMember(ctx, owner.Organization.ID, owner.User.ID, ownerMembershipID); err == nil {
		t.Fatal("removed the Owner's own membership, want a validation error")
	}
}

func TestRemoveMemberRevokesAccess(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	owner := signupOrg(t, ctx, pool, svc, "Remove Org", uniqueEmail(t, pool))

	inviteURL, _ := svc.Invite(ctx, owner.Organization.ID, owner.User.ID, auth.InviteInput{Name: "Eve", Email: uniqueEmail(t, pool), RoleKey: "Viewer"})
	accepted, err := svc.AcceptInvite(ctx, auth.AcceptInviteInput{Token: tokenFromURL(t, inviteURL), Name: "Eve", Password: "eves-long-password"})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	members, _ := svc.ListMembers(ctx, owner.Organization.ID)
	var membershipID string
	for _, m := range members {
		if m.UserID == accepted.User.ID {
			membershipID = m.ID
		}
	}

	if err := svc.RemoveMember(ctx, owner.Organization.ID, owner.User.ID, membershipID); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if _, _, err := svc.ResolveSession(ctx, accepted.Token); err == nil {
		t.Fatal("a removed member's session still resolves, want it revoked")
	}
	if _, err := svc.UpdateMembership(ctx, owner.Organization.ID, owner.User.ID, membershipID, auth.UpdateMembershipInput{}); err == nil {
		t.Fatal("UpdateMembership on an already-removed membership succeeded, want not-found")
	}
}

func TestLoginRequiresOrganizationIDWhenMultipleActiveMemberships(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	email := uniqueEmail(t, pool)

	orgA := signupOrg(t, ctx, pool, svc, "Multi Org A", email)

	// Invite the same email into a second org and accept — this user now
	// holds two active memberships.
	ownerB := signupOrg(t, ctx, pool, svc, "Multi Org B", uniqueEmail(t, pool))
	inviteURL, err := svc.Invite(ctx, ownerB.Organization.ID, ownerB.User.ID, auth.InviteInput{Name: "Multi User", Email: email, RoleKey: "Viewer"})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	// AcceptInvite would try to overwrite the password, which is fine —
	// same user, same credentials either way — but skip Name mismatch
	// concerns by reusing the original password.
	if _, err := svc.AcceptInvite(ctx, auth.AcceptInviteInput{Token: tokenFromURL(t, inviteURL), Name: "Multi User", Password: "correct-horse-battery-staple"}); err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}

	if _, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: "correct-horse-battery-staple"}); err == nil {
		t.Fatal("Login with no organizationId succeeded despite two active memberships, want a validation error")
	}

	res, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: "correct-horse-battery-staple", OrganizationID: orgA.Organization.ID})
	if err != nil {
		t.Fatalf("Login with an explicit organizationId: %v", err)
	}
	if res.Organization.ID != orgA.Organization.ID {
		t.Fatalf("logged into org %s, want %s", res.Organization.ID, orgA.Organization.ID)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	orgA := signupOrg(t, ctx, pool, svc, "Tenant A", uniqueEmail(t, pool))
	orgB := signupOrg(t, ctx, pool, svc, "Tenant B", uniqueEmail(t, pool))

	membersA, err := svc.ListMembers(ctx, orgA.Organization.ID)
	if err != nil || len(membersA) != 1 {
		t.Fatalf("ListMembers for org A: %v, %+v", err, membersA)
	}
	ownerAMembershipID := membersA[0].ID

	t.Run("update", func(t *testing.T) {
		role := "Admin"
		if _, err := svc.UpdateMembership(ctx, orgB.Organization.ID, orgB.User.ID, ownerAMembershipID, auth.UpdateMembershipInput{RoleKey: &role}); err == nil {
			t.Fatal("org B updated org A's membership, want not-found")
		}
	})
	t.Run("remove", func(t *testing.T) {
		if err := svc.RemoveMember(ctx, orgB.Organization.ID, orgB.User.ID, ownerAMembershipID); err == nil {
			t.Fatal("org B removed org A's membership, want not-found")
		}
	})
	t.Run("list never mixes orgs", func(t *testing.T) {
		membersB, err := svc.ListMembers(ctx, orgB.Organization.ID)
		if err != nil {
			t.Fatalf("ListMembers for org B: %v", err)
		}
		for _, m := range membersB {
			if m.ID == ownerAMembershipID {
				t.Fatal("org B's member list contains org A's membership")
			}
		}
	})
	t.Run("activity never mixes orgs", func(t *testing.T) {
		_, _ = svc.Invite(ctx, orgA.Organization.ID, orgA.User.ID, auth.InviteInput{Name: "X", Email: uniqueEmail(t, pool), RoleKey: "Viewer"})
		activityB, err := svc.ListActivity(ctx, orgB.Organization.ID)
		if err != nil {
			t.Fatalf("ListActivity for org B: %v", err)
		}
		if len(activityB) != 0 {
			t.Fatalf("org B's activity log contains org A's invite: %+v", activityB)
		}
	})
}

// fixedResolver implements tenant.SessionResolver by ignoring the token
// entirely and always resolving to one fixed identity — this test only
// needs a real request context carrying (orgID, userID) the way
// tenant.NewMiddleware would actually produce it, not a real session
// lookup (that round trip is covered by TestSignupCreatesOrgOwnerAndSession
// and friends already exercising ResolveSession itself).
type fixedResolver struct{ orgID, userID string }

func (f fixedResolver) ResolveSession(ctx context.Context, token string) (string, string, error) {
	return f.orgID, f.userID, nil
}

func TestRequirePermissionMiddleware(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	owner := signupOrg(t, ctx, pool, svc, "Middleware Org", uniqueEmail(t, pool))

	inviteURL, _ := svc.Invite(ctx, owner.Organization.ID, owner.User.ID, auth.InviteInput{Name: "Viewer Person", Email: uniqueEmail(t, pool), RoleKey: "Viewer"})
	viewer, err := svc.AcceptInvite(ctx, auth.AcceptInviteInput{Token: tokenFromURL(t, inviteURL), Name: "Viewer Person", Password: "viewers-long-password"})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}

	logger := logging.New("error")
	requireWrite := svc.RequirePermission("team.write", logger)

	handlerFor := func(userID, orgID string) http.Handler {
		protected := requireWrite(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
		return tenant.NewMiddleware(fixedResolver{orgID: orgID, userID: userID}, logger)(protected)
	}

	newRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/team/members/invite", nil)
		req.AddCookie(&http.Cookie{Name: tenant.CookieName, Value: "irrelevant-fixedResolver-ignores-it"})
		return req
	}

	t.Run("owner has team.write", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handlerFor(owner.User.ID, owner.Organization.ID).ServeHTTP(rec, newRequest())
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 for Owner", rec.Code)
		}
	})

	t.Run("viewer lacks team.write", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handlerFor(viewer.User.ID, viewer.Organization.ID).ServeHTTP(rec, newRequest())
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 for Viewer", rec.Code)
		}
	})
}

func tokenFromURL(t *testing.T, rawURL string) string {
	t.Helper()
	const marker = "token="
	i := indexOf(rawURL, marker)
	if i < 0 {
		t.Fatalf("invite URL %q has no token query param", rawURL)
	}
	return rawURL[i+len(marker):]
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
