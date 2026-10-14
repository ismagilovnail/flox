package trackinglink_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/trackinglink"
)

func TestCreateGetUpdateDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)
	domainID := seedDomain(t, ctx, pool, orgID, "tl-track.example.com")
	svc := trackinglink.NewService(trackinglink.NewRepository(pool))

	created, err := svc.Create(ctx, orgID, campaignID, trackinglink.CreateInput{DomainID: domainID, Slug: "summer-promo"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.URL != "http://tl-track.example.com/t/summer-promo" {
		t.Fatalf("URL = %q, want the tracker's own /t/{slug} shape", created.URL)
	}
	if created.Secure {
		t.Fatal("Secure = true for a domain without a certificate")
	}

	// Marketer audit #4: once the domain serves a valid certificate the
	// link is https.
	if _, err := pool.Exec(ctx, `UPDATE domains SET ssl = 'issued' WHERE id = $1`, domainID); err != nil {
		t.Fatalf("issuing: %v", err)
	}
	secure, err := svc.Get(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !secure.Secure || secure.URL != "https://tl-track.example.com/t/summer-promo" {
		t.Fatalf("with a certificate: secure=%v URL=%q, want https", secure.Secure, secure.URL)
	}

	if created.Status != trackinglink.StatusActive {
		t.Fatalf("Status = %q, want active default", created.Status)
	}

	links, err := svc.List(ctx, orgID, campaignID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(links) != 1 || links[0].ID != created.ID {
		t.Fatalf("links = %+v, want just the one created", links)
	}

	paused := trackinglink.StatusPaused
	updated, err := svc.UpdateStatus(ctx, orgID, created.ID, paused)
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if updated.Status != trackinglink.StatusPaused {
		t.Fatalf("Status after update = %q, want paused", updated.Status)
	}

	if err := svc.Delete(ctx, orgID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, orgID, created.ID); !isNotFound(err) {
		t.Fatalf("Get after delete: err = %v, want not_found", err)
	}
}

// TestCreateRejectsAnotherOrgsDomain is the tenant-isolation check that
// matters most in this package: domain_id arrives in the request body, not
// scoped by the URL the way campaign_id is, so nothing upstream stops a
// client from naming any domain_id it can guess/enumerate.
func TestCreateRejectsAnotherOrgsDomain(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	campaignA := seedCampaign(t, ctx, pool, orgA)
	domainB := seedDomain(t, ctx, pool, orgB, "orgb-owned.example.com")
	svc := trackinglink.NewService(trackinglink.NewRepository(pool))

	_, err := svc.Create(ctx, orgA, campaignA, trackinglink.CreateInput{DomainID: domainB, Slug: "steal"})
	apiErr, ok := err.(*apierror.Error)
	if !ok || apiErr.Status != 422 {
		t.Fatalf("err = %v, want a validation error rejecting the other org's domain", err)
	}
}

// TestCreateRejectsAnotherOrgsCampaign covers the same class of gap for
// campaign_id: a route param, but nothing before Repository.Create checks
// it actually belongs to the caller's own org the way List/Get's WHERE
// organization_id=$1 already does implicitly.
func TestCreateRejectsAnotherOrgsCampaign(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	campaignB := seedCampaign(t, ctx, pool, orgB)
	domainA := seedDomain(t, ctx, pool, orgA, "orga-owned.example.com")
	svc := trackinglink.NewService(trackinglink.NewRepository(pool))

	_, err := svc.Create(ctx, orgA, campaignB, trackinglink.CreateInput{DomainID: domainA, Slug: "steal"})
	if err == nil {
		t.Fatal("expected an error creating a tracking link under another org's campaign")
	}
}

func TestCreateRejectsDuplicateSlugOnSameDomain(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)
	domainID := seedDomain(t, ctx, pool, orgID, "tl-track.example.com")
	svc := trackinglink.NewService(trackinglink.NewRepository(pool))

	if _, err := svc.Create(ctx, orgID, campaignID, trackinglink.CreateInput{DomainID: domainID, Slug: "promo"}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := svc.Create(ctx, orgID, campaignID, trackinglink.CreateInput{DomainID: domainID, Slug: "promo"})
	apiErr, ok := err.(*apierror.Error)
	if !ok || apiErr.Code != "conflict" {
		t.Fatalf("second Create err = %v, want a conflict on (domain, slug)", err)
	}
}

func TestCreateRejectsInvalidSlug(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	campaignID := seedCampaign(t, ctx, pool, orgID)
	domainID := seedDomain(t, ctx, pool, orgID, "tl-track.example.com")
	svc := trackinglink.NewService(trackinglink.NewRepository(pool))

	// "Has-Caps" is deliberately not in this list — Service.Create
	// lowercases the slug first (matching domain.Service's own
	// lowercasing), so "Has-Caps" normalizes to a valid "has-caps" rather
	// than being rejected.
	for _, slug := range []string{"", "has spaces", "-leading-hyphen", "trailing-hyphen-", "has/slash"} {
		if _, err := svc.Create(ctx, orgID, campaignID, trackinglink.CreateInput{DomainID: domainID, Slug: slug}); err == nil {
			t.Fatalf("slug %q: expected a validation error", slug)
		}
	}
}

// ListByOrg is every campaign's links of this organization only, filtered
// by status — the campaigns list's "Copy tracking URL".
func TestListByOrg(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	campaign1, campaign2 := seedCampaign(t, ctx, pool, orgA), seedCampaign(t, ctx, pool, orgA)
	domainA := seedDomain(t, ctx, pool, orgA, "tl-org.example.com")
	svc := trackinglink.NewService(trackinglink.NewRepository(pool))

	one, err := svc.Create(ctx, orgA, campaign1, trackinglink.CreateInput{DomainID: domainA, Slug: "one"})
	if err != nil {
		t.Fatal(err)
	}
	two, err := svc.Create(ctx, orgA, campaign2, trackinglink.CreateInput{DomainID: domainA, Slug: "two"})
	if err != nil {
		t.Fatal(err)
	}
	paused, err := svc.Create(ctx, orgA, campaign2, trackinglink.CreateInput{DomainID: domainA, Slug: "paused"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateStatus(ctx, orgA, paused.ID, trackinglink.StatusPaused); err != nil {
		t.Fatal(err)
	}
	campaignB := seedCampaign(t, ctx, pool, orgB)
	domainB := seedDomain(t, ctx, pool, orgB, "tl-org-b.example.com")
	if _, err := svc.Create(ctx, orgB, campaignB, trackinglink.CreateInput{DomainID: domainB, Slug: "theirs"}); err != nil {
		t.Fatal(err)
	}

	ids := func(links []trackinglink.TrackingLink) map[string]string {
		out := map[string]string{}
		for _, l := range links {
			out[l.ID] = l.CampaignID
		}
		return out
	}
	all, err := svc.ListByOrg(ctx, orgA, "")
	if err != nil {
		t.Fatalf("ListByOrg: %v", err)
	}
	if got := ids(all); len(got) != 3 || got[one.ID] != campaign1 || got[two.ID] != campaign2 || got[paused.ID] != campaign2 {
		t.Fatalf("all = %v, want this organization's three links with their campaigns", got)
	}
	active, err := svc.ListByOrg(ctx, orgA, trackinglink.StatusActive)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(active); len(got) != 2 || got[paused.ID] != "" {
		t.Fatalf("active = %v, want the two active ones", got)
	}
	if active[0].URL != "http://tl-org.example.com/t/two" {
		t.Fatalf("first = %q, want the newest with its URL", active[0].URL)
	}
	if _, err := svc.ListByOrg(ctx, orgA, "deleted"); !isValidation(err) {
		t.Fatalf("unknown status: %v, want a validation error", err)
	}
}

func isValidation(err error) bool {
	var apiErr *apierror.Error
	return errors.As(err, &apiErr) && apiErr.Status == 422
}

func isNotFound(err error) bool {
	apiErr, ok := err.(*apierror.Error)
	return ok && apiErr.Code == "not_found"
}

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

func seedOrg(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, id, "Test Org "+id)
	if err != nil {
		t.Fatalf("seeding organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, id)
	})
	return id
}

func seedCampaign(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID string) string {
	t.Helper()
	sourceID := idgen.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO traffic_sources (id, organization_id, name, type) VALUES ($1, $2, $3, $4)`,
		sourceID, orgID, "Test Source", "Facebook",
	)
	if err != nil {
		t.Fatalf("seeding traffic source: %v", err)
	}
	id := idgen.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO campaigns (id, organization_id, traffic_source_id, name, fallback_url) VALUES ($1, $2, $3, $4, $5)`,
		id, orgID, sourceID, "Test Campaign", "https://example.com/fallback",
	)
	if err != nil {
		t.Fatalf("seeding campaign: %v", err)
	}
	return id
}

func seedDomain(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID, hostname string) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO domains (id, organization_id, domain, purpose) VALUES ($1, $2, $3, $4)`,
		id, orgID, hostname, []string{"tracking"},
	)
	if err != nil {
		t.Fatalf("seeding domain: %v", err)
	}
	return id
}
