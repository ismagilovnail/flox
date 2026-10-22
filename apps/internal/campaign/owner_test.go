package campaign_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/campaign"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func seedMember(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID, status string) string {
	t.Helper()
	id := idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, 'Buyer')`, id, strings.ToLower(id)+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO memberships (id, organization_id, user_id, role_id, status)
		SELECT $1, $2, $3, id, $4 FROM roles WHERE key = 'Buyer'`, idgen.New(), orgID, id, status); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id) })
	return id
}

// Audit TEAM-1: a campaign is its creator's, a copy its copier's; it is
// reassigned only to an active member of its organization.
func TestCampaignOwnership(t *testing.T) {
	ctx := context.Background()
	pool := mustPool(t)
	orgA, orgB := seedOrg(t, ctx, pool), seedOrg(t, ctx, pool)
	repo := campaign.NewRepository(pool)
	svc := campaign.NewService(repo)
	ann := seedMember(t, ctx, pool, orgA, "active")
	bob := seedMember(t, ctx, pool, orgA, "active")
	suspended := seedMember(t, ctx, pool, orgA, "suspended")
	stranger := seedMember(t, ctx, pool, orgB, "active")

	c, err := svc.Create(ctx, orgA, campaign.CreateInput{TrafficSourceID: seedTrafficSource(t, ctx, pool, orgA),
		Name: "Ann's", FallbackURL: "https://example.com/f", OwnerUserID: ann})
	if err != nil {
		t.Fatal(err)
	}
	if c.OwnerUserID == nil || *c.OwnerUserID != ann {
		t.Fatalf("owner = %v, want Ann", c.OwnerUserID)
	}
	keyMade, err := svc.Create(ctx, orgA, campaign.CreateInput{TrafficSourceID: c.TrafficSourceID, Name: "By a key", FallbackURL: "https://example.com/f"})
	if err != nil || keyMade.OwnerUserID != nil {
		t.Fatalf("no owner given = %v, %v; want unowned", keyMade.OwnerUserID, err)
	}
	copied, err := svc.Duplicate(ctx, orgA, c.ID, bob)
	if err != nil || copied.OwnerUserID == nil || *copied.OwnerUserID != bob {
		t.Fatalf("Bob's copy = %+v, %v; want owned by Bob", copied.OwnerUserID, err)
	}

	for name, user := range map[string]string{"suspended": suspended, "another organization's": stranger, "unknown": idgen.New()} {
		if _, err := svc.SetOwner(ctx, orgA, c.ID, user); err == nil {
			t.Errorf("assigning to a %s member succeeded", name)
		}
	}
	if moved, err := svc.SetOwner(ctx, orgA, c.ID, bob); err != nil || *moved.OwnerUserID != bob {
		t.Fatalf("reassigning to Bob = %v, %v", moved.OwnerUserID, err)
	}
	if _, err := svc.SetOwner(ctx, orgB, c.ID, stranger); err == nil {
		t.Error("another organization reassigned the campaign")
	}

	owners, err := repo.Owners(ctx, orgA)
	if err != nil {
		t.Fatal(err)
	}
	if len(owners) != 3 || owners[c.ID] != bob || owners[keyMade.ID] != "" || owners[copied.ID] != bob {
		t.Errorf("owners = %v", owners)
	}
	if b, _ := repo.Owners(ctx, orgB); len(b) != 0 {
		t.Errorf("org B sees owners %v", b)
	}
	if owner, found, err := repo.OwnerOf(ctx, orgB, c.ID); err != nil || found || owner != "" {
		t.Errorf("OwnerOf from org B = %q %v %v, want not found", owner, found, err)
	}
}
