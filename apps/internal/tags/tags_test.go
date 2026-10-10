package tags_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
	"github.com/ismagilovnail/flox/apps/internal/tags"
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

func seedOrg(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	id := idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, id, "Test Org "+id); err != nil {
		t.Fatalf("seeding organization: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, id) })
	return id
}

// seedPwa creates a minimal real pwas row — used to prove
// EntityExists/the ownership check against a real, FK-backed table
// rather than a fabricated id. One entity type is enough to exercise
// the generic (entity-type -> table) lookup path; it's the same code
// for all seven.
func seedPwa(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID string) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO pwas (id, organization_id, name, short_name, theme_color, background_color, icon_url, start_url)
		VALUES ($1, $2, 'Test PWA', 'Test', '#16a34a', '#ffffff', 'https://example.com/icon.png', '/install')`,
		id, orgID,
	)
	if err != nil {
		t.Fatalf("seeding pwa: %v", err)
	}
	return id
}

func TestCreateListUpdateDeleteTag(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := tags.NewService(tags.NewRepository(pool))

	created, err := svc.Create(ctx, orgID, tags.CreateTagInput{Name: "Top Performer", Color: "green"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Name != "Top Performer" || created.Color != "green" {
		t.Fatalf("Create returned %+v, want Top Performer/green", created)
	}

	list, _, err := svc.List(ctx, orgID, pagination.Page{Limit: pagination.MaxLimit})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("List = %+v, want exactly the created tag", list)
	}

	newName := "Renamed"
	newColor := "blue"
	updated, err := svc.Update(ctx, orgID, created.ID, tags.UpdateTagInput{Name: &newName, Color: &newColor})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != newName || updated.Color != newColor {
		t.Fatalf("Update = %+v, want %s/%s", updated, newName, newColor)
	}

	if err := svc.Delete(ctx, orgID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, _, err = svc.List(ctx, orgID, pagination.Page{Limit: pagination.MaxLimit})
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List after delete = %+v, want empty", list)
	}
}

func TestCreateRejectsInvalidShapes(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := tags.NewService(tags.NewRepository(pool))

	if _, err := svc.Create(ctx, orgID, tags.CreateTagInput{Name: "  ", Color: "green"}); err == nil {
		t.Fatal("Create with blank name succeeded, want validation error")
	}
	if _, err := svc.Create(ctx, orgID, tags.CreateTagInput{Name: "Ok", Color: "not-a-color"}); err == nil {
		t.Fatal("Create with invalid color succeeded, want validation error")
	}
}

func TestCrossOrgIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	svc := tags.NewService(tags.NewRepository(pool))

	created, err := svc.Create(ctx, orgA, tags.CreateTagInput{Name: "Org A Tag", Color: "red"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	listB, _, err := svc.List(ctx, orgB, pagination.Page{Limit: pagination.MaxLimit})
	if err != nil {
		t.Fatalf("List (org B): %v", err)
	}
	if len(listB) != 0 {
		t.Fatalf("org B sees %+v, want no tags from org A", listB)
	}

	if _, err := svc.Update(ctx, orgB, created.ID, tags.UpdateTagInput{Name: strPtr("Hijacked")}); err == nil {
		t.Fatal("org B updated org A's tag, want not-found")
	}
	if err := svc.Delete(ctx, orgB, created.ID); err == nil {
		t.Fatal("org B deleted org A's tag, want not-found")
	}
}

func TestSetEntityTagsRequiresRealEntity(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := tags.NewService(tags.NewRepository(pool))

	tag, err := svc.Create(ctx, orgID, tags.CreateTagInput{Name: "T", Color: "gray"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = svc.SetEntityTags(ctx, orgID, tags.SetEntityTagsInput{
		EntityType: tags.EntityPwa,
		EntityID:   idgen.New(), // real ULID shape, but no such row
		TagIDs:     []string{tag.ID},
	})
	if err == nil {
		t.Fatal("SetEntityTags against a nonexistent pwa succeeded, want not-found")
	}
}

func TestSetEntityTagsRejectsForeignTag(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	svc := tags.NewService(tags.NewRepository(pool))

	tagInOrgB, err := svc.Create(ctx, orgB, tags.CreateTagInput{Name: "B's tag", Color: "gray"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	pwaInOrgA := seedPwa(t, ctx, pool, orgA)

	err = svc.SetEntityTags(ctx, orgA, tags.SetEntityTagsInput{
		EntityType: tags.EntityPwa,
		EntityID:   pwaInOrgA,
		TagIDs:     []string{tagInOrgB.ID},
	})
	if err == nil {
		t.Fatal("SetEntityTags with another org's tag_id succeeded, want validation error")
	}
}

func TestSetEntityTagsReplacesFullSet(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := tags.NewService(tags.NewRepository(pool))
	pwaID := seedPwa(t, ctx, pool, orgID)

	tagA, _ := svc.Create(ctx, orgID, tags.CreateTagInput{Name: "A", Color: "red"})
	tagB, _ := svc.Create(ctx, orgID, tags.CreateTagInput{Name: "B", Color: "blue"})
	tagC, _ := svc.Create(ctx, orgID, tags.CreateTagInput{Name: "C", Color: "green"})

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("SetEntityTags: %v", err)
		}
	}

	must(svc.SetEntityTags(ctx, orgID, tags.SetEntityTagsInput{
		EntityType: tags.EntityPwa, EntityID: pwaID, TagIDs: []string{tagA.ID, tagB.ID},
	}))
	assignments, err := svc.ListAssignments(ctx, orgID, tags.EntityPwa)
	if err != nil {
		t.Fatalf("ListAssignments: %v", err)
	}
	if !hasTag(assignments, tagA.ID) || !hasTag(assignments, tagB.ID) || hasTag(assignments, tagC.ID) {
		t.Fatalf("assignments = %+v, want exactly A and B", assignments)
	}

	// Replace with a different set entirely — B dropped, C added, A kept.
	must(svc.SetEntityTags(ctx, orgID, tags.SetEntityTagsInput{
		EntityType: tags.EntityPwa, EntityID: pwaID, TagIDs: []string{tagA.ID, tagC.ID},
	}))
	assignments, err = svc.ListAssignments(ctx, orgID, tags.EntityPwa)
	if err != nil {
		t.Fatalf("ListAssignments: %v", err)
	}
	if !hasTag(assignments, tagA.ID) || hasTag(assignments, tagB.ID) || !hasTag(assignments, tagC.ID) {
		t.Fatalf("assignments after replace = %+v, want exactly A and C", assignments)
	}
}

// TestBulkEditSemantics exercises §30.6's exact bulk rule: toAdd applies
// to every entity (no-op where already present), toRemove removes from
// every entity (no-op where absent), and a tag present on only SOME of
// the selection but named in neither list is left exactly as it was —
// per-entity, not globally.
func TestBulkEditSemantics(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := tags.NewService(tags.NewRepository(pool))
	pwa1 := seedPwa(t, ctx, pool, orgID)
	pwa2 := seedPwa(t, ctx, pool, orgID)

	common, _ := svc.Create(ctx, orgID, tags.CreateTagInput{Name: "common", Color: "red"})
	onlyPwa1, _ := svc.Create(ctx, orgID, tags.CreateTagInput{Name: "only-pwa1", Color: "blue"})
	toAdd, _ := svc.Create(ctx, orgID, tags.CreateTagInput{Name: "to-add", Color: "green"})

	if err := svc.SetEntityTags(ctx, orgID, tags.SetEntityTagsInput{
		EntityType: tags.EntityPwa, EntityID: pwa1, TagIDs: []string{common.ID, onlyPwa1.ID},
	}); err != nil {
		t.Fatalf("seed pwa1 tags: %v", err)
	}
	if err := svc.SetEntityTags(ctx, orgID, tags.SetEntityTagsInput{
		EntityType: tags.EntityPwa, EntityID: pwa2, TagIDs: []string{common.ID},
	}); err != nil {
		t.Fatalf("seed pwa2 tags: %v", err)
	}

	err := svc.BulkEdit(ctx, orgID, tags.BulkEditInput{
		EntityType: tags.EntityPwa,
		EntityIDs:  []string{pwa1, pwa2},
		ToAdd:      []string{toAdd.ID},
		ToRemove:   []string{common.ID},
	})
	if err != nil {
		t.Fatalf("BulkEdit: %v", err)
	}

	assignments, err := svc.ListAssignments(ctx, orgID, tags.EntityPwa)
	if err != nil {
		t.Fatalf("ListAssignments: %v", err)
	}

	pwa1Tags := tagsFor(assignments, pwa1)
	pwa2Tags := tagsFor(assignments, pwa2)

	// pwa1: common removed, onlyPwa1 untouched (wasn't named), to-add added.
	if pwa1Tags[common.ID] {
		t.Error("pwa1 still has 'common', want removed")
	}
	if !pwa1Tags[onlyPwa1.ID] {
		t.Error("pwa1 lost 'only-pwa1', want untouched")
	}
	if !pwa1Tags[toAdd.ID] {
		t.Error("pwa1 missing 'to-add', want added")
	}

	// pwa2: common removed, to-add added, never had onlyPwa1.
	if pwa2Tags[common.ID] {
		t.Error("pwa2 still has 'common', want removed")
	}
	if !pwa2Tags[toAdd.ID] {
		t.Error("pwa2 missing 'to-add', want added")
	}
	if pwa2Tags[onlyPwa1.ID] {
		t.Error("pwa2 has 'only-pwa1', want never assigned")
	}
}

func TestDeleteTagCascadesAssignments(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := tags.NewService(tags.NewRepository(pool))
	pwaID := seedPwa(t, ctx, pool, orgID)

	tag, err := svc.Create(ctx, orgID, tags.CreateTagInput{Name: "Deletable", Color: "gray"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.SetEntityTags(ctx, orgID, tags.SetEntityTagsInput{
		EntityType: tags.EntityPwa, EntityID: pwaID, TagIDs: []string{tag.ID},
	}); err != nil {
		t.Fatalf("SetEntityTags: %v", err)
	}

	if err := svc.Delete(ctx, orgID, tag.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	assignments, err := svc.ListAssignments(ctx, orgID, tags.EntityPwa)
	if err != nil {
		t.Fatalf("ListAssignments: %v", err)
	}
	if hasTag(assignments, tag.ID) {
		t.Fatalf("assignments still reference deleted tag: %+v", assignments)
	}
}

func hasTag(assignments []tags.Assignment, tagID string) bool {
	for _, a := range assignments {
		if a.TagID == tagID {
			return true
		}
	}
	return false
}

func tagsFor(assignments []tags.Assignment, entityID string) map[string]bool {
	out := map[string]bool{}
	for _, a := range assignments {
		if a.EntityID == entityID {
			out[a.TagID] = true
		}
	}
	return out
}

func strPtr(s string) *string { return &s }
