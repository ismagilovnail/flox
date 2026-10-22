package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/auth"
	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// campaignWriteRoute: a mutating route on one campaign — the campaign
// itself or anything under it.
var campaignWriteRoute = regexp.MustCompile(`^/campaigns/\{(id|campaignId)\}`)

// notOwnerGated: mutating routes on one campaign that the owner check
// deliberately does not cover, each with why.
var notOwnerGated = map[string]string{
	"PUT /campaigns/{id}/owner":                     "reassigning needs campaign.manage_all, which no buyer has",
	"POST /campaigns/{campaignId}/routing/simulate": "a read: the simulator persists nothing",
}

// Audit TEAM-1: a Buyer changes only campaigns they own. Every mutating
// route on one campaign — found by walking the router, so a route added
// later is covered — answers 403 to another buyer, and the owner gets
// through. Reassigning is for campaign.manage_all only.
func TestBuyersChangeOnlyTheirCampaigns(t *testing.T) {
	env := newIsolationEnv(t)
	ctx, pool := env.ctx, env.pool
	authSvc := auth.NewService(auth.NewRepository(pool), "http://localhost:3000", noMail{})

	addBuyer := func(name string) (userID string, c *isoClient) {
		t.Helper()
		email := "buyer-" + strings.ToLower(idgen.New()) + "@example.test"
		link, err := authSvc.Invite(ctx, env.orgA, env.userA, auth.InviteInput{Email: email, Name: name, RoleKey: "Buyer"})
		if err != nil {
			t.Fatalf("inviting %s: %v", name, err)
		}
		u, _ := url.Parse(link)
		if _, err := authSvc.AcceptInvite(ctx, auth.AcceptInviteInput{Token: u.Query().Get("token"), Name: name, Password: isoPassword, ClientIP: "127.0.0.1"}); err != nil {
			t.Fatalf("accepting %s's invite: %v", name, err)
		}
		if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = lower($1)`, email).Scan(&userID); err != nil {
			t.Fatal(err)
		}
		c = &isoClient{t: t, h: env.srv.Handler(), origin: env.c.origin, ip: randomTestIP(), email: map[string]string{env.orgA: email}}
		c.signIn(env.orgA)
		return userID, c
	}
	annID, ann := addBuyer("Ann")
	_, bob := addBuyer("Bob")

	campaigns := campaign.NewRepository(pool)
	camp := env.idsA["campaign"]
	if _, err := campaigns.SetOwner(ctx, env.orgA, camp, annID); err != nil {
		t.Fatal(err)
	}

	// Bob, on Ann's campaign: 403 on every write.
	param := regexp.MustCompile(`\{[^}]+\}`)
	checked := 0
	for rk := range routerRoutes(t, env.srv.Mux()) {
		if rk.method == http.MethodGet || !campaignWriteRoute.MatchString(rk.path) {
			continue
		}
		if _, skip := notOwnerGated[rk.method+" "+rk.path]; skip {
			continue
		}
		path := campaignWriteRoute.ReplaceAllString(rk.path, "/campaigns/"+camp)
		path = param.ReplaceAllStringFunc(path, func(string) string { return idgen.New() })
		status, body, _ := bob.request(rk.method, path, map[string]any{})
		if status != http.StatusForbidden || !strings.Contains(body, "another buyer") {
			t.Errorf("%s %s as another buyer = %d %s, want 403", rk.method, rk.path, status, body)
		}
		checked++
	}
	if checked < 15 {
		t.Fatalf("checked %d campaign write routes — the walk no longer finds them", checked)
	}

	// Ann, on her own campaign, gets through; so does a create, which she then owns.
	if status, body, _ := ann.request(http.MethodPatch, "/campaigns/"+camp, map[string]string{"name": "Renamed by Ann"}); status != http.StatusOK {
		t.Errorf("owner renaming her campaign = %d %s", status, body)
	}
	status, body, _ := ann.request(http.MethodPost, "/campaigns", map[string]any{
		"trafficSourceId": env.idsA["trafficSource"], "name": "Ann's own", "fallbackUrl": "https://example.com/f"})
	var created campaign.Campaign
	_ = json.Unmarshal([]byte(body), &created)
	if status != http.StatusCreated || created.OwnerUserID == nil || *created.OwnerUserID != annID {
		t.Fatalf("buyer's new campaign = %d %s, want owned by her", status, body)
	}
	if status, _, _ := ann.request(http.MethodPost, "/campaigns/"+created.ID+"/duplicate", nil); status != http.StatusCreated {
		t.Errorf("duplicating her own campaign = %d", status)
	}

	// Reassigning: not for a buyer, for the owner of the organization;
	// only to an active member of it.
	if status, _, _ := ann.request(http.MethodPut, "/campaigns/"+camp+"/owner", map[string]string{"userId": annID}); status != http.StatusForbidden {
		t.Errorf("buyer reassigning = %d, want 403", status)
	}
	env.c.signIn(env.orgA)
	if status, body, _ := env.c.request(http.MethodPut, "/campaigns/"+camp+"/owner", map[string]string{"userId": env.userB}); status != http.StatusUnprocessableEntity {
		t.Errorf("assigning to another organization's member = %d %s, want 422", status, body)
	}
	if status, body, _ := env.c.request(http.MethodPut, "/campaigns/"+camp+"/owner", map[string]string{"userId": ""}); status != http.StatusOK || !strings.Contains(body, `"ownerUserId":null`) {
		t.Errorf("unassigning = %d %s", status, body)
	}
	// An unowned campaign is not a buyer's to change either.
	if status, _, _ := ann.request(http.MethodPatch, "/campaigns/"+camp, map[string]string{"name": "Ann again"}); status != http.StatusForbidden {
		t.Errorf("buyer on an unowned campaign = %d, want 403", status)
	}
	// The organization's owner (campaign.manage_all) changes any campaign.
	if status, body, _ := env.c.request(http.MethodPatch, "/campaigns/"+camp, map[string]string{"name": "Renamed by the owner"}); status != http.StatusOK {
		t.Errorf("owner of the organization renaming = %d %s", status, body)
	}
	// Another organization's campaign stays 404 for reassigning.
	if status, _, _ := env.c.request(http.MethodPut, "/campaigns/"+env.idsB["campaign"]+"/owner", map[string]string{"userId": annID}); status != http.StatusNotFound {
		t.Errorf("reassigning another organization's campaign = %d, want 404", status)
	}
}
