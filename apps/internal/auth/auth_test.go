package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	stdtotp "github.com/pquerna/otp/totp"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/auth"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/logging"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
	"github.com/ismagilovnail/flox/apps/internal/totp"
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

// testMailer captures the most recently "sent" email instead of actually
// sending one — package-level and shared across every newService(pool)
// call in this file rather than threaded through every call site's
// signature, which is safe because nothing in this file runs its
// subtests in parallel (no t.Parallel() calls) — signupOrg below reads
// lastBody immediately after the one Signup call that wrote it, with no
// other test's Signup/ResendVerification able to interleave.
type testMailer struct {
	lastTo, lastBody string
}

func (m *testMailer) Send(_ context.Context, to, _, body string) error {
	m.lastTo, m.lastBody = to, body
	return nil
}

var sharedTestMailer = &testMailer{}

// testTOTPKey is a fixed, throwaway 32-byte key — fine for tests, never
// reused as a real deployment's TOTP_ENCRYPTION_KEY (see .env.example's
// own warning against exactly that).
const testTOTPKey = "IXkldrBTvgfDhIxzUBn3MrRAJq7Gh7z8mQ5UhBC4DRo="

func newService(pool *pgxpool.Pool) *auth.Service {
	svc := auth.NewService(auth.NewRepository(pool), "http://localhost:3000", sharedTestMailer)
	key, err := totp.ParseKey(testTOTPKey)
	if err != nil {
		panic(err)
	}
	svc.SetTOTPKey(key)
	return svc
}

// signupOrg is the one way this package's tests create an org — no
// separate SQL seeder, because Signup itself is the code under test for
// org creation, and every other test needs a real signed-up owner anyway.
// Registers cleanup deleting the organization (cascades to its users/
// memberships/sessions, migration 00001/00020's ON DELETE CASCADE) so
// repeated test runs against a real, persistent dev Postgres don't leave
// permanent rows behind — every other package's own seedOrg helper
// (adaccount_test.go, costsync_test.go, ...) does the same.
// signupOrg drives the real Signup -> (click the emailed link) ->
// VerifyEmail sequence and hands back the session VerifyEmail produces —
// same auth.Result shape and same call-site contract every test in this
// file already expects, so adding email verification ahead of login
// didn't require touching any of them. "Clicking the link" is reading
// sharedTestMailer.lastBody (populated by Signup's call to Send,
// intercepted above instead of actually going out over SMTP) and pulling
// the token out of the verification URL inside it, exactly like
// tokenFromURL does for an invite URL below.
func signupOrg(t *testing.T, ctx context.Context, pool *pgxpool.Pool, svc *auth.Service, orgName, ownerEmail string) auth.Result {
	t.Helper()
	signupRes, err := svc.Signup(ctx, auth.SignupInput{
		OrganizationName: orgName,
		Name:             "Test Owner",
		Email:            ownerEmail,
		Password:         "correct-horse-battery-staple",
	})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, signupRes.Organization.ID)
	})

	if sharedTestMailer.lastTo != ownerEmail {
		t.Fatalf("verification email was sent to %q, want %q", sharedTestMailer.lastTo, ownerEmail)
	}
	verifyRes, err := svc.VerifyEmail(ctx, tokenFromBody(t, sharedTestMailer.lastBody), "")
	if err != nil {
		t.Fatalf("verify-email: %v", err)
	}
	if verifyRes.Session == nil {
		t.Fatal("VerifyEmail returned no session for a fresh signup's own single active membership")
	}
	return *verifyRes.Session
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

func TestLoginBlockedBeforeEmailVerification(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	email := uniqueEmail(t, pool)

	signupRes, err := svc.Signup(ctx, auth.SignupInput{
		OrganizationName: "Unverified Org", Name: "Not Yet Verified", Email: email, Password: "correct-horse-battery-staple",
	})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, signupRes.Organization.ID)
	})

	_, err = svc.Login(ctx, auth.LoginInput{Email: email, Password: "correct-horse-battery-staple"})
	if err == nil {
		t.Fatal("Login before email verification succeeded, want an error")
	}
	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "email_not_verified" {
		t.Fatalf("Login error = %v, want code email_not_verified", err)
	}
}

func TestVerifyEmailLogsInAndConsumesTheToken(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	email := uniqueEmail(t, pool)

	signupRes, err := svc.Signup(ctx, auth.SignupInput{
		OrganizationName: "Verify Org", Name: "Verifier", Email: email, Password: "correct-horse-battery-staple",
	})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, signupRes.Organization.ID)
	})
	token := tokenFromBody(t, sharedTestMailer.lastBody)

	verifyRes, err := svc.VerifyEmail(ctx, token, "")
	if err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}
	if verifyRes.Session == nil || verifyRes.Session.RoleKey != "Owner" {
		t.Fatalf("VerifyEmail session = %+v, want a session with role Owner", verifyRes.Session)
	}

	// Now that it's verified, Login succeeds.
	if _, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: "correct-horse-battery-staple"}); err != nil {
		t.Fatalf("Login after verification: %v", err)
	}

	// The token cannot be replayed.
	if _, err := svc.VerifyEmail(ctx, token, ""); err == nil {
		t.Fatal("VerifyEmail succeeded twice with the same token, want an error the second time")
	}
}

func TestVerifyEmailRejectsInvalidToken(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()

	if _, err := svc.VerifyEmail(ctx, "not-a-real-token", ""); err == nil {
		t.Fatal("VerifyEmail accepted a token that was never issued")
	}
}

func TestResendVerificationIssuesAFreshTokenAndInvalidatesTheOld(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	email := uniqueEmail(t, pool)

	signupRes, err := svc.Signup(ctx, auth.SignupInput{
		OrganizationName: "Resend Verify Org", Name: "Resender", Email: email, Password: "correct-horse-battery-staple",
	})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, signupRes.Organization.ID)
	})
	oldToken := tokenFromBody(t, sharedTestMailer.lastBody)

	if err := svc.ResendVerification(ctx, email); err != nil {
		t.Fatalf("ResendVerification: %v", err)
	}
	newToken := tokenFromBody(t, sharedTestMailer.lastBody)

	if _, err := svc.VerifyEmail(ctx, oldToken, ""); err == nil {
		t.Fatal("the pre-resend token still verifies, want it invalidated")
	}
	if _, err := svc.VerifyEmail(ctx, newToken, ""); err != nil {
		t.Fatalf("VerifyEmail with the freshly-resent token: %v", err)
	}
}

func TestResendVerificationIsSilentForUnknownOrAlreadyVerifiedEmail(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()

	// Anti-enumeration: neither case is distinguishable from the caller's
	// side, both just report success.
	if err := svc.ResendVerification(ctx, uniqueEmail(t, pool)); err != nil {
		t.Fatalf("ResendVerification for an unregistered email: %v", err)
	}

	verified := signupOrg(t, ctx, pool, svc, "Already Verified Org", uniqueEmail(t, pool))
	if err := svc.ResendVerification(ctx, verified.User.Email); err != nil {
		t.Fatalf("ResendVerification for an already-verified email: %v", err)
	}
}

func TestLoginSucceedsAndFailsCorrectly(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	email := uniqueEmail(t, pool)
	signupOrg(t, ctx, pool, svc, "Login Org", email)

	t.Run("correct password", func(t *testing.T) {
		outcome, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: "correct-horse-battery-staple"})
		if err != nil {
			t.Fatalf("Login: %v", err)
		}
		if outcome.Session == nil || outcome.Session.RoleKey != "Owner" {
			t.Fatalf("Login session = %+v, want a session with role Owner", outcome.Session)
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
		outcome, err := svc.Login(ctx, auth.LoginInput{Email: "  " + upper(email) + "  ", Password: "correct-horse-battery-staple"})
		if err != nil {
			t.Fatalf("Login with a differently-cased, whitespace-padded email: %v", err)
		}
		if outcome.Session == nil || outcome.Session.User.Email != email {
			t.Fatalf("Login session = %+v, want User.Email %q", outcome.Session, email)
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

	outcome, err := svc.AcceptInvite(ctx, auth.AcceptInviteInput{Token: token, Name: "Invited Analyst", Password: "another-long-password"})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if outcome.Session == nil || outcome.Session.RoleKey != "Analyst" || outcome.Session.Organization.ID != owner.Organization.ID {
		t.Fatalf("AcceptInvite result = %+v, want role Analyst in org %s", outcome.Session, owner.Organization.ID)
	}
	res := *outcome.Session

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
	acceptedOutcome, err := svc.AcceptInvite(ctx, auth.AcceptInviteInput{Token: tokenFromURL(t, inviteURL), Name: "Carol", Password: "carols-long-password"})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if acceptedOutcome.Session == nil {
		t.Fatal("AcceptInvite returned no session")
	}
	accepted := *acceptedOutcome.Session

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
	acceptedOutcome, err := svc.AcceptInvite(ctx, auth.AcceptInviteInput{Token: tokenFromURL(t, inviteURL), Name: "Dave", Password: "daves-long-password"})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if acceptedOutcome.Session == nil {
		t.Fatal("AcceptInvite returned no session")
	}
	accepted := *acceptedOutcome.Session
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
	acceptedOutcome, err := svc.AcceptInvite(ctx, auth.AcceptInviteInput{Token: tokenFromURL(t, inviteURL), Name: "Eve", Password: "eves-long-password"})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if acceptedOutcome.Session == nil {
		t.Fatal("AcceptInvite returned no session")
	}
	accepted := *acceptedOutcome.Session
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

	outcome, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: "correct-horse-battery-staple", OrganizationID: orgA.Organization.ID})
	if err != nil {
		t.Fatalf("Login with an explicit organizationId: %v", err)
	}
	if outcome.Session == nil || outcome.Session.Organization.ID != orgA.Organization.ID {
		t.Fatalf("Login session = %+v, want org %s", outcome.Session, orgA.Organization.ID)
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

// nilAPIKeyAuthenticator implements tenant.APIKeyAuthenticator by always
// rejecting — every cookie-based subtest never sends an Authorization
// header, so this is never actually invoked; it exists only to satisfy
// tenant.NewMiddleware's signature.
type nilAPIKeyAuthenticator struct{}

func (nilAPIKeyAuthenticator) Authenticate(ctx context.Context, rawKey string) (string, string, string, error) {
	return "", "", "", apierror.Unauthorized("invalid api key")
}

// fixedAPIKeyAuthenticator implements tenant.APIKeyAuthenticator by
// ignoring the raw key and always resolving to one fixed identity/scope —
// same rationale as fixedResolver above, for TestRequirePermissionAPIKeyScope.
type fixedAPIKeyAuthenticator struct{ orgID, userID, scope string }

func (f fixedAPIKeyAuthenticator) Authenticate(ctx context.Context, rawKey string) (string, string, string, error) {
	return f.orgID, f.userID, f.scope, nil
}

func TestRequirePermissionMiddleware(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	owner := signupOrg(t, ctx, pool, svc, "Middleware Org", uniqueEmail(t, pool))

	inviteURL, _ := svc.Invite(ctx, owner.Organization.ID, owner.User.ID, auth.InviteInput{Name: "Viewer Person", Email: uniqueEmail(t, pool), RoleKey: "Viewer"})
	viewerOutcome, err := svc.AcceptInvite(ctx, auth.AcceptInviteInput{Token: tokenFromURL(t, inviteURL), Name: "Viewer Person", Password: "viewers-long-password"})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if viewerOutcome.Session == nil {
		t.Fatal("AcceptInvite returned no session")
	}
	viewer := *viewerOutcome.Session

	logger := logging.New("error")
	requireWrite := svc.RequirePermission("team.write", logger)

	handlerFor := func(userID, orgID string) http.Handler {
		protected := requireWrite(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
		return tenant.NewMiddleware(fixedResolver{orgID: orgID, userID: userID}, nilAPIKeyAuthenticator{}, logger, false)(protected)
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

// TestRequirePermissionAPIKeyScope exercises RequirePermission's other
// branch — no membership row, no DB query, purely apiKeyScopeAllows's
// suffix rule (audit finding #4's real-auth follow-up). team.read/
// team.write are real, live-mounted permission keys (team_handler.go) —
// using them here means this test is checking the exact same keys a real
// request would, not a synthetic one that happens to never be wired to a
// route.
func TestRequirePermissionAPIKeyScope(t *testing.T) {
	logger := logging.New("error")
	svc := newService(mustPool(t))
	requireRead := svc.RequirePermission("team.read", logger)
	requireWrite := svc.RequirePermission("team.write", logger)

	build := func(scope string, requirePerm func(http.Handler) http.Handler) http.Handler {
		protected := requirePerm(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
		return tenant.NewMiddleware(fixedResolver{}, fixedAPIKeyAuthenticator{orgID: "org1", userID: "creator1", scope: scope}, logger, false)(protected)
	}

	newRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/team/members/invite", nil)
		req.Header.Set("Authorization", "Bearer flx_live_irrelevant-fixedAPIKeyAuthenticator-ignores-it")
		return req
	}

	cases := []struct {
		name        string
		scope       string
		requirePerm func(http.Handler) http.Handler
		wantStatus  int
	}{
		{"read scope allows team.read", "read", requireRead, http.StatusOK},
		{"read scope denies team.write", "read", requireWrite, http.StatusForbidden},
		{"write scope allows team.read", "write", requireRead, http.StatusOK},
		{"write scope denies team.write (excluded even at write tier)", "write", requireWrite, http.StatusForbidden},
		{"admin scope allows team.read", "admin", requireRead, http.StatusOK},
		{"admin scope allows team.write", "admin", requireWrite, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			build(tc.scope, tc.requirePerm).ServeHTTP(rec, newRequest())
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
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

// tokenFromBody is tokenFromURL's counterpart for a token embedded inside
// a longer plain-text email body (auth.Service.sendVerificationEmail's
// link isn't the last thing in the message the way an invite URL is on
// its own) — reads up to the next whitespace instead of to the end of
// the string.
func tokenFromBody(t *testing.T, body string) string {
	t.Helper()
	const marker = "token="
	i := indexOf(body, marker)
	if i < 0 {
		t.Fatalf("email body %q has no token query param", body)
	}
	rest := body[i+len(marker):]
	for j, r := range rest {
		if r == '\n' || r == '\r' || r == ' ' {
			return rest[:j]
		}
	}
	return rest
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestGetOrganizationSettingsReturnsRealDefaults(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()

	owner := signupOrg(t, ctx, pool, svc, "Org Settings Defaults", uniqueEmail(t, pool))

	got, err := svc.GetOrganizationSettings(ctx, owner.Organization.ID)
	if err != nil {
		t.Fatalf("GetOrganizationSettings: %v", err)
	}
	if got.Name != "Org Settings Defaults" {
		t.Fatalf("Name = %q, want %q", got.Name, "Org Settings Defaults")
	}
	// migration 00001's own column defaults — real, not fabricated.
	if got.Timezone != "UTC" || got.Currency != "USD" {
		t.Fatalf("defaults = %+v, want UTC/USD", got)
	}
}

func TestUpdateOrganizationSettingsPersists(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()

	owner := signupOrg(t, ctx, pool, svc, "Org Settings Update", uniqueEmail(t, pool))

	name := "Renamed Org"
	tz := "Europe/Berlin"
	currency := "eur" // lowercase in, uppercase out
	updated, err := svc.UpdateOrganizationSettings(ctx, owner.Organization.ID, owner.User.ID, auth.UpdateOrganizationSettingsInput{
		Name: &name, Timezone: &tz, Currency: &currency,
	})
	if err != nil {
		t.Fatalf("UpdateOrganizationSettings: %v", err)
	}
	if updated.Name != name || updated.Timezone != tz || updated.Currency != "EUR" {
		t.Fatalf("updated = %+v, want Renamed Org/Europe/Berlin/EUR", updated)
	}

	got, err := svc.GetOrganizationSettings(ctx, owner.Organization.ID)
	if err != nil {
		t.Fatalf("GetOrganizationSettings after update: %v", err)
	}
	if got != updated {
		t.Fatalf("Get after Update = %+v, want %+v (real persistence, not just the returned value)", got, updated)
	}

	activity, err := svc.ListActivity(ctx, owner.Organization.ID)
	if err != nil {
		t.Fatalf("ListActivity: %v", err)
	}
	if len(activity) == 0 || activity[0].Action != "settings.updated" {
		t.Fatalf("activity = %+v, want a leading settings.updated entry", activity)
	}
}

func TestUpdateOrganizationSettingsValidatesInput(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()

	owner := signupOrg(t, ctx, pool, svc, "Org Settings Validation", uniqueEmail(t, pool))

	badTZ := "Not/A_Real_Zone"
	if _, err := svc.UpdateOrganizationSettings(ctx, owner.Organization.ID, owner.User.ID, auth.UpdateOrganizationSettingsInput{Timezone: &badTZ}); err == nil {
		t.Error("invalid timezone accepted")
	}

	badCurrency := "US"
	if _, err := svc.UpdateOrganizationSettings(ctx, owner.Organization.ID, owner.User.ID, auth.UpdateOrganizationSettingsInput{Currency: &badCurrency}); err == nil {
		t.Error("invalid currency accepted")
	}

	shortName := "A"
	if _, err := svc.UpdateOrganizationSettings(ctx, owner.Organization.ID, owner.User.ID, auth.UpdateOrganizationSettingsInput{Name: &shortName}); err == nil {
		t.Error("too-short name accepted")
	}
}

func TestGetSecuritySettingsReturnsRealDefaults(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()

	owner := signupOrg(t, ctx, pool, svc, "Security Defaults Org", uniqueEmail(t, pool))

	got, err := svc.GetSecuritySettings(ctx, owner.Organization.ID)
	if err != nil {
		t.Fatalf("GetSecuritySettings: %v", err)
	}
	// migration 00032's own column defaults — real, not fabricated;
	// 43200 (30 days) preserves the pre-existing hardcoded sessionTTL.
	if got.SessionTimeoutMinutes != 43200 || len(got.IPAllowlist) != 0 || got.TwoFactorRequired {
		t.Fatalf("defaults = %+v, want 43200/empty/false", got)
	}
}

func TestUpdateSecuritySettingsPersistsAndValidates(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()

	owner := signupOrg(t, ctx, pool, svc, "Security Update Org", uniqueEmail(t, pool))

	timeout := 60
	required := true
	updated, err := svc.UpdateSecuritySettings(ctx, owner.Organization.ID, owner.User.ID, "203.0.113.5", auth.UpdateSecuritySettingsInput{
		SessionTimeoutMinutes: &timeout,
		IPAllowlist:           []string{"203.0.113.0/24"},
		TwoFactorRequired:     &required,
	})
	if err != nil {
		t.Fatalf("UpdateSecuritySettings: %v", err)
	}
	if updated.SessionTimeoutMinutes != 60 || len(updated.IPAllowlist) != 1 || !updated.TwoFactorRequired {
		t.Fatalf("updated = %+v, want 60/[203.0.113.0/24]/true", updated)
	}

	got, err := svc.GetSecuritySettings(ctx, owner.Organization.ID)
	if err != nil {
		t.Fatalf("GetSecuritySettings after update: %v", err)
	}
	if !reflect.DeepEqual(got, updated) {
		t.Fatalf("Get after Update = %+v, want %+v (real persistence)", got, updated)
	}

	// Out-of-range timeout.
	bad := 5
	if _, err := svc.UpdateSecuritySettings(ctx, owner.Organization.ID, owner.User.ID, "203.0.113.5", auth.UpdateSecuritySettingsInput{SessionTimeoutMinutes: &bad}); err == nil {
		t.Error("out-of-range session timeout accepted")
	}

	// Garbage IP/CIDR.
	if _, err := svc.UpdateSecuritySettings(ctx, owner.Organization.ID, owner.User.ID, "203.0.113.5", auth.UpdateSecuritySettingsInput{IPAllowlist: []string{"999.999.999.999"}}); err == nil {
		t.Error("invalid IP accepted")
	}

	// The real lockout safeguard: a new list that excludes the acting
	// request's own current IP is rejected.
	if _, err := svc.UpdateSecuritySettings(ctx, owner.Organization.ID, owner.User.ID, "198.51.100.9", auth.UpdateSecuritySettingsInput{IPAllowlist: []string{"203.0.113.0/24"}}); err == nil {
		t.Error("a list excluding the acting IP was accepted, want rejected (self-lockout guard)")
	}
}

func TestLoginRespectsIPAllowlist(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	email := uniqueEmail(t, pool)
	owner := signupOrg(t, ctx, pool, svc, "IP Allowlist Org", email)

	if _, err := svc.UpdateSecuritySettings(ctx, owner.Organization.ID, owner.User.ID, "203.0.113.5", auth.UpdateSecuritySettingsInput{
		IPAllowlist: []string{"203.0.113.0/24"},
	}); err != nil {
		t.Fatalf("UpdateSecuritySettings: %v", err)
	}

	if _, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: "correct-horse-battery-staple", ClientIP: "198.51.100.9"}); err == nil {
		t.Fatal("Login from a disallowed IP succeeded, want rejected")
	} else {
		var apiErr *apierror.Error
		if !errors.As(err, &apiErr) || apiErr.Code != "ip_not_allowed" {
			t.Fatalf("Login error = %v, want code ip_not_allowed", err)
		}
	}

	outcome, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: "correct-horse-battery-staple", ClientIP: "203.0.113.5"})
	if err != nil {
		t.Fatalf("Login from an allowed IP: %v", err)
	}
	if outcome.Session == nil {
		t.Fatal("Login from an allowed IP produced no session")
	}
}

func TestSelfServeTOTPEnrollmentAndLogin(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	email := uniqueEmail(t, pool)
	owner := signupOrg(t, ctx, pool, svc, "Self-Serve TOTP Org", email)

	if enabled, err := svc.TOTPStatus(ctx, owner.User.ID); err != nil || enabled {
		t.Fatalf("TOTPStatus before enrollment = (%v, %v), want (false, nil)", enabled, err)
	}

	enrollment, err := svc.EnrollTOTP(ctx, owner.User.ID, email)
	if err != nil {
		t.Fatalf("EnrollTOTP: %v", err)
	}
	if enrollment.Secret == "" || enrollment.QRCodeDataURI == "" || len(enrollment.BackupCodes) != 10 {
		t.Fatalf("Enrollment = %+v, want all fields populated", enrollment)
	}

	if err := svc.ConfirmTOTP(ctx, owner.User.ID, "000000"); err == nil {
		t.Fatal("ConfirmTOTP accepted an arbitrary wrong code")
	}

	code, err := stdtotp.GenerateCode(enrollment.Secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if err := svc.ConfirmTOTP(ctx, owner.User.ID, code); err != nil {
		t.Fatalf("ConfirmTOTP with a real code: %v", err)
	}

	if enabled, err := svc.TOTPStatus(ctx, owner.User.ID); err != nil || !enabled {
		t.Fatalf("TOTPStatus after confirmation = (%v, %v), want (true, nil)", enabled, err)
	}

	// Login now requires the second factor instead of granting a session.
	outcome, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: "correct-horse-battery-staple"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if outcome.Session != nil || !outcome.RequiresTOTP || outcome.RequiresTOTPEnrollment || outcome.PendingToken == "" {
		t.Fatalf("Login outcome = %+v, want RequiresTOTP with a pending token and no session", outcome)
	}

	// A wrong code invalidates the pending login immediately (audit
	// finding #3) rather than leaving the 5-minute window open to
	// repeated guesses — so the very same token must now be rejected
	// even with the real code afterward.
	if _, err := svc.VerifyPendingLogin(ctx, outcome.PendingToken, "000000"); err == nil {
		t.Fatal("VerifyPendingLogin accepted an arbitrary wrong code")
	}
	loginCode, err := stdtotp.GenerateCode(enrollment.Secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if _, err := svc.VerifyPendingLogin(ctx, outcome.PendingToken, loginCode); err == nil {
		t.Fatal("VerifyPendingLogin accepted the real code after a prior wrong guess invalidated the token")
	}

	// A fresh login issues a fresh pending token, which the real code
	// still works against.
	outcome2, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: "correct-horse-battery-staple"})
	if err != nil {
		t.Fatalf("second Login: %v", err)
	}
	loginCode2, err := stdtotp.GenerateCode(enrollment.Secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	res, err := svc.VerifyPendingLogin(ctx, outcome2.PendingToken, loginCode2)
	if err != nil {
		t.Fatalf("VerifyPendingLogin with a real code: %v", err)
	}
	if res.RoleKey != "Owner" {
		t.Fatalf("RoleKey = %q, want Owner", res.RoleKey)
	}

	// The pending token is single-use.
	if _, err := svc.VerifyPendingLogin(ctx, outcome2.PendingToken, loginCode2); err == nil {
		t.Fatal("VerifyPendingLogin succeeded twice with the same pending token")
	}
}

func TestBackupCodeConsumesOnce(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	email := uniqueEmail(t, pool)
	owner := signupOrg(t, ctx, pool, svc, "Backup Code Org", email)

	enrollment, err := svc.EnrollTOTP(ctx, owner.User.ID, email)
	if err != nil {
		t.Fatalf("EnrollTOTP: %v", err)
	}
	code, err := stdtotp.GenerateCode(enrollment.Secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if err := svc.ConfirmTOTP(ctx, owner.User.ID, code); err != nil {
		t.Fatalf("ConfirmTOTP: %v", err)
	}

	outcome, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: "correct-horse-battery-staple"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	backupCode := enrollment.BackupCodes[0]

	if _, err := svc.VerifyPendingLogin(ctx, outcome.PendingToken, backupCode); err != nil {
		t.Fatalf("VerifyPendingLogin with a real backup code: %v", err)
	}

	// A second pending login, then the SAME backup code must be rejected.
	outcome2, err := svc.Login(ctx, auth.LoginInput{Email: email, Password: "correct-horse-battery-staple"})
	if err != nil {
		t.Fatalf("second Login: %v", err)
	}
	if _, err := svc.VerifyPendingLogin(ctx, outcome2.PendingToken, backupCode); err == nil {
		t.Fatal("an already-used backup code was accepted a second time")
	}
}

func TestOrgRequiredTOTPForcesEnrollmentOnLogin(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	ownerEmail := uniqueEmail(t, pool)
	owner := signupOrg(t, ctx, pool, svc, "Forced Enrollment Org", ownerEmail)

	invitedEmail := uniqueEmail(t, pool)
	inviteURL, err := svc.Invite(ctx, owner.Organization.ID, owner.User.ID, auth.InviteInput{Name: "Needs 2FA", Email: invitedEmail, RoleKey: "Analyst"})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	acceptOutcome, err := svc.AcceptInvite(ctx, auth.AcceptInviteInput{Token: tokenFromURL(t, inviteURL), Name: "Needs 2FA", Password: "needs-2fa-password"})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if acceptOutcome.Session == nil {
		t.Fatal("AcceptInvite (before org requires 2FA) returned no session")
	}

	required := true
	if _, err := svc.UpdateSecuritySettings(ctx, owner.Organization.ID, owner.User.ID, "203.0.113.5", auth.UpdateSecuritySettingsInput{TwoFactorRequired: &required}); err != nil {
		t.Fatalf("UpdateSecuritySettings: %v", err)
	}

	outcome, err := svc.Login(ctx, auth.LoginInput{Email: invitedEmail, Password: "needs-2fa-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if outcome.Session != nil || !outcome.RequiresTOTPEnrollment || outcome.RequiresTOTP || outcome.PendingToken == "" {
		t.Fatalf("Login outcome = %+v, want RequiresTOTPEnrollment with a pending token", outcome)
	}

	enrollment, err := svc.EnrollTOTP(ctx, acceptOutcome.Session.User.ID, invitedEmail)
	if err != nil {
		t.Fatalf("EnrollTOTP: %v", err)
	}
	code, err := stdtotp.GenerateCode(enrollment.Secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}

	// CompleteEnrollmentLogin both confirms AND completes the login in
	// one step — no separate "now please log in again" round trip.
	res, err := svc.CompleteEnrollmentLogin(ctx, outcome.PendingToken, code)
	if err != nil {
		t.Fatalf("CompleteEnrollmentLogin: %v", err)
	}
	if res.User.Email != invitedEmail {
		t.Fatalf("User.Email = %q, want %q", res.User.Email, invitedEmail)
	}

	if enabled, err := svc.TOTPStatus(ctx, acceptOutcome.Session.User.ID); err != nil || !enabled {
		t.Fatalf("TOTPStatus after forced enrollment = (%v, %v), want (true, nil)", enabled, err)
	}

	// A second login now goes through the ordinary "already enrolled"
	// RequiresTOTP path, not enrollment again.
	outcome2, err := svc.Login(ctx, auth.LoginInput{Email: invitedEmail, Password: "needs-2fa-password"})
	if err != nil {
		t.Fatalf("second Login: %v", err)
	}
	if !outcome2.RequiresTOTP || outcome2.RequiresTOTPEnrollment {
		t.Fatalf("second Login outcome = %+v, want plain RequiresTOTP", outcome2)
	}
}

func TestDisableTOTPRequiresPasswordAndRespectsOrgPolicy(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	ctx := context.Background()
	email := uniqueEmail(t, pool)
	owner := signupOrg(t, ctx, pool, svc, "Disable TOTP Org", email)

	enrollment, err := svc.EnrollTOTP(ctx, owner.User.ID, email)
	if err != nil {
		t.Fatalf("EnrollTOTP: %v", err)
	}
	code, err := stdtotp.GenerateCode(enrollment.Secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if err := svc.ConfirmTOTP(ctx, owner.User.ID, code); err != nil {
		t.Fatalf("ConfirmTOTP: %v", err)
	}

	if err := svc.DisableTOTP(ctx, owner.User.ID, owner.Organization.ID, "wrong-password"); err == nil {
		t.Fatal("DisableTOTP accepted the wrong password")
	}

	required := true
	if _, err := svc.UpdateSecuritySettings(ctx, owner.Organization.ID, owner.User.ID, "203.0.113.5", auth.UpdateSecuritySettingsInput{TwoFactorRequired: &required}); err != nil {
		t.Fatalf("UpdateSecuritySettings: %v", err)
	}
	if err := svc.DisableTOTP(ctx, owner.User.ID, owner.Organization.ID, "correct-horse-battery-staple"); err == nil {
		t.Fatal("DisableTOTP succeeded while the org requires 2FA, want rejected")
	}

	notRequired := false
	if _, err := svc.UpdateSecuritySettings(ctx, owner.Organization.ID, owner.User.ID, "203.0.113.5", auth.UpdateSecuritySettingsInput{TwoFactorRequired: &notRequired}); err != nil {
		t.Fatalf("UpdateSecuritySettings: %v", err)
	}
	if err := svc.DisableTOTP(ctx, owner.User.ID, owner.Organization.ID, "correct-horse-battery-staple"); err != nil {
		t.Fatalf("DisableTOTP with the correct password: %v", err)
	}
	if enabled, err := svc.TOTPStatus(ctx, owner.User.ID); err != nil || enabled {
		t.Fatalf("TOTPStatus after disable = (%v, %v), want (false, nil)", enabled, err)
	}
}

// TestPermissionGrantsMatchTheSeededPolicy pins the full role -> permission
// matrix against the database, per role, rather than spot-checking a few
// keys the way TestMeReturnsRoleAndPermissions does.
//
// It exists because the 2026-09-06 audit found this policy real in the
// database and unenforced almost everywhere in the API — the fix mounts
// auth.Service.RequirePermission across every mutating route, which makes
// these grants load-bearing for the first time. A silently wrong grant
// (a role that keeps a write key it shouldn't, or loses a read key it
// needs) is now an access-control bug rather than a cosmetic one, so the
// exact matrix is worth asserting rather than assuming.
//
// apps/web/src/lib/mock/team.ts renders this same matrix to admins from
// its own hardcoded copy; keep the two in sync (that file says so too).
func TestPermissionGrantsMatchTheSeededPolicy(t *testing.T) {
	pool := mustPool(t)
	svc := newService(pool)
	repo := auth.NewRepository(pool)
	ctx := context.Background()

	universalReads := []string{
		"campaign.read", "analytics.read", "content.read",
		"custom_metric.read", "report_preset.read", "referral.read",
	}
	all := append(append([]string{}, universalReads...),
		"campaign.write", "offer.read", "offer.write", "source.read", "source.write",
		"content.write", "custom_metric.write", "report_preset.write", "referral.write",
		"team.read", "team.write", "settings.write",
	)

	want := map[string][]string{
		"Owner": all,
		"Admin": all,
		"Manager": append(append([]string{}, universalReads...),
			"campaign.write", "offer.read", "offer.write", "source.read", "source.write",
			"content.write", "custom_metric.write", "report_preset.write", "team.read"),
		"Buyer":   append(append([]string{}, universalReads...), "campaign.write", "offer.read", "source.read", "source.write"),
		"Analyst": append(append([]string{}, universalReads...), "offer.read", "source.read"),
		"Viewer":  universalReads,
	}

	for role, expected := range want {
		granted, err := repo.PermissionsForRole(ctx, role)
		if err != nil {
			t.Fatalf("PermissionsForRole(%q): %v", role, err)
		}
		has := map[string]bool{}
		for _, p := range granted {
			has[p] = true
		}
		if len(granted) != len(expected) {
			t.Errorf("%s: has %d permissions %v, want exactly %d %v", role, len(granted), granted, len(expected), expected)
		}
		for _, p := range expected {
			if !has[p] {
				t.Errorf("%s: missing %q (has %v)", role, p, granted)
			}
		}
	}

	// The two keys migration 00035 added, checked through the same
	// HasPermission path RequirePermission itself uses — not just the
	// listing above — since that is what actually gates a request.
	owner := signupOrg(t, ctx, pool, svc, "Content Perm Org", uniqueEmail(t, pool))
	for _, tc := range []struct {
		key  string
		want bool
	}{
		{"content.read", true},
		{"content.write", true},
	} {
		ok, err := repo.HasPermission(ctx, owner.User.ID, owner.Organization.ID, tc.key)
		if err != nil {
			t.Fatalf("HasPermission(%q): %v", tc.key, err)
		}
		if ok != tc.want {
			t.Errorf("Owner HasPermission(%q) = %v, want %v", tc.key, ok, tc.want)
		}
	}
}
