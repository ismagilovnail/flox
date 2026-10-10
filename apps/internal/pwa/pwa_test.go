package pwa_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
	"github.com/ismagilovnail/flox/apps/internal/pwa"
)

func validInput(name string) pwa.CreateInput {
	return pwa.CreateInput{
		Name: name, ShortName: "Sweeps", ThemeColor: "#16a34a", BackgroundColor: "#0a0a0a",
		IconURL: "https://cdn.floxlink.io/pwa/sweeps/icon-512.png", StartURL: "/install/sweeps",
		BounceInAppWebview: true,
	}
}

func TestCreateGetUpdateDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))

	created, err := svc.Create(ctx, orgID, validInput("Sweeps PWA"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != pwa.StatusActive {
		t.Fatalf("Status = %q, want active (the DB default)", created.Status)
	}

	got, err := svc.Get(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != created.Name {
		t.Fatalf("Get returned %+v, want the just-created row", got)
	}

	newName := "Sweeps PWA (renamed)"
	updated, err := svc.Update(ctx, orgID, created.ID, pwa.UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("Name = %q, want %q", updated.Name, newName)
	}

	if err := svc.Delete(ctx, orgID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, orgID, created.ID); err == nil {
		t.Fatal("Get after Delete succeeded, want not-found")
	}
}

func TestCreateRejectsInvalidShapes(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))

	bad := validInput("Bad Theme")
	bad.ThemeColor = "green"
	if _, err := svc.Create(ctx, orgID, bad); err == nil {
		t.Fatal("Create with a non-hex theme color succeeded, want a validation error")
	}

	bad = validInput("Bad Icon")
	bad.IconURL = "not-a-url"
	if _, err := svc.Create(ctx, orgID, bad); err == nil {
		t.Fatal("Create with an invalid icon URL succeeded, want a validation error")
	}

	bad = validInput("Bad Short Name")
	bad.ShortName = ""
	if _, err := svc.Create(ctx, orgID, bad); err == nil {
		t.Fatal("Create with an empty short name succeeded, want a validation error")
	}

	bad = validInput("Bad Start URL")
	bad.StartURL = "   "
	if _, err := svc.Create(ctx, orgID, bad); err == nil {
		t.Fatal("Create with a blank start URL succeeded, want a validation error")
	}
}

func TestUpdateAcceptsRelativeStartURL(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, validInput("Relative Start"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// startUrl is a device-relative install path (e.g. "/install/sweeps"),
	// not a full URL — it must never be run through isValidURL the way
	// iconUrl is.
	relative := "/install/casino"
	updated, err := svc.Update(ctx, orgID, created.ID, pwa.UpdateInput{StartURL: &relative})
	if err != nil {
		t.Fatalf("Update with a relative start URL: %v", err)
	}
	if updated.StartURL != relative {
		t.Fatalf("StartURL = %q, want %q", updated.StartURL, relative)
	}
}

func TestPauseActivateTransitions(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, validInput("Toggle PWA"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	paused, err := svc.Pause(ctx, orgID, created.ID)
	if err != nil || paused.Status != pwa.StatusPaused {
		t.Fatalf("Pause: got %+v, err %v", paused, err)
	}
	if again, err := svc.Pause(ctx, orgID, created.ID); err != nil || again.Status != pwa.StatusPaused {
		t.Fatalf("Pause (idempotent): got %+v, err %v", again, err)
	}

	active, err := svc.Activate(ctx, orgID, created.ID)
	if err != nil || active.Status != pwa.StatusActive {
		t.Fatalf("Activate: got %+v, err %v", active, err)
	}

	archived := pwa.StatusArchived
	if _, err := svc.Update(ctx, orgID, created.ID, pwa.UpdateInput{Status: &archived}); err != nil {
		t.Fatalf("archiving via Update: %v", err)
	}
	if _, err := svc.Pause(ctx, orgID, created.ID); err == nil {
		t.Fatal("Pause on an archived pwa succeeded, want a conflict error")
	}
	if _, err := svc.Activate(ctx, orgID, created.ID); err == nil {
		t.Fatal("Activate on an archived pwa succeeded, want a conflict error")
	}
}

func TestDuplicateKeepsStatus(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))
	created, err := svc.Create(ctx, orgID, validInput("Original"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Pause(ctx, orgID, created.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	dup, err := svc.Duplicate(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup.ID == created.ID {
		t.Fatal("Duplicate returned the same id as the source")
	}
	if dup.Name != "Original (Copy)" {
		t.Fatalf("Name = %q, want %q", dup.Name, "Original (Copy)")
	}
	if dup.Status != pwa.StatusPaused {
		t.Fatalf("Status = %q, want paused (copied from the source, not reset)", dup.Status)
	}
}

func TestListingFieldsRoundTripAndDefaultToNilNotZero(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))

	// Never entered: Rating/RatingCount/SizeMB must come back nil, not a
	// fabricated 0 — the store-listing preview must render "—", not "0
	// stars"/"0 MB", for a PWA nobody has rated or sized yet.
	created, err := svc.Create(ctx, orgID, validInput("No Listing Data"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Rating != nil || created.RatingCount != nil || created.SizeMB != nil {
		t.Fatalf("Create with no listing fields = %+v, want Rating/RatingCount/SizeMB all nil", created)
	}
	if created.Developer != "" || created.Description != "" || created.DownloadsLabel != "" {
		t.Fatalf("Create with no listing fields = %+v, want empty strings, not fabricated placeholders", created)
	}

	rating := 4.7
	ratingCount := 100
	sizeMB := 15.0
	in := validInput("With Listing Data")
	in.Developer = "  Acme Corp  "
	in.Description = "A real description"
	in.Rating = &rating
	in.RatingCount = &ratingCount
	in.DownloadsLabel = "1K+"
	in.SizeMB = &sizeMB

	withData, err := svc.Create(ctx, orgID, in)
	if err != nil {
		t.Fatalf("Create with listing fields: %v", err)
	}
	if withData.Developer != "Acme Corp" {
		t.Fatalf("Developer = %q, want trimmed %q", withData.Developer, "Acme Corp")
	}
	if withData.Rating == nil || *withData.Rating != 4.7 {
		t.Fatalf("Rating = %v, want 4.7", withData.Rating)
	}
	if withData.RatingCount == nil || *withData.RatingCount != 100 {
		t.Fatalf("RatingCount = %v, want 100", withData.RatingCount)
	}
	if withData.DownloadsLabel != "1K+" {
		t.Fatalf("DownloadsLabel = %q, want %q", withData.DownloadsLabel, "1K+")
	}
	if withData.SizeMB == nil || *withData.SizeMB != 15.0 {
		t.Fatalf("SizeMB = %v, want 15.0", withData.SizeMB)
	}

	// Round-trips through Update too.
	newRating := 3.2
	updated, err := svc.Update(ctx, orgID, created.ID, pwa.UpdateInput{Rating: &newRating})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Rating == nil || *updated.Rating != 3.2 {
		t.Fatalf("Rating after Update = %v, want 3.2", updated.Rating)
	}
}

func TestCreateRejectsOutOfRangeListingFields(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))

	tooHigh := 5.5
	bad := validInput("Bad Rating")
	bad.Rating = &tooHigh
	if _, err := svc.Create(ctx, orgID, bad); err == nil {
		t.Fatal("Create with rating 5.5 succeeded, want a validation error (must be 0-5)")
	}

	negative := -1
	bad = validInput("Bad Rating Count")
	bad.RatingCount = &negative
	if _, err := svc.Create(ctx, orgID, bad); err == nil {
		t.Fatal("Create with a negative ratingCount succeeded, want a validation error")
	}

	negativeSize := -5.0
	bad = validInput("Bad Size")
	bad.SizeMB = &negativeSize
	if _, err := svc.Create(ctx, orgID, bad); err == nil {
		t.Fatal("Create with a negative sizeMb succeeded, want a validation error")
	}
}

func TestDuplicateCopiesListingFields(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))
	rating := 4.9
	in := validInput("Source With Listing")
	in.Developer = "Acme"
	in.Rating = &rating
	created, err := svc.Create(ctx, orgID, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	dup, err := svc.Duplicate(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup.Developer != "Acme" || dup.Rating == nil || *dup.Rating != 4.9 {
		t.Fatalf("Duplicate = %+v, want listing fields copied from the source", dup)
	}
}

func TestScreenshotsWhatsNewAndRatingBreakdownRoundTrip(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))

	// Never entered: Screenshots must come back an empty (not nil) slice
	// and WhatsNew/RatingStars* must come back empty/nil, not fabricated.
	created, err := svc.Create(ctx, orgID, validInput("No Gallery Data"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Screenshots == nil || len(created.Screenshots) != 0 {
		t.Fatalf("Screenshots = %#v, want an empty slice", created.Screenshots)
	}
	if created.WhatsNew != "" {
		t.Fatalf("WhatsNew = %q, want empty", created.WhatsNew)
	}
	if created.RatingStars5 != nil || created.RatingStars1 != nil {
		t.Fatalf("RatingStars5/1 = %v/%v, want both nil", created.RatingStars5, created.RatingStars1)
	}

	s5, s4, s3, s2, s1 := 80.0, 15.0, 3.0, 1.0, 1.0
	in := validInput("With Gallery Data")
	in.Screenshots = []string{" https://cdn.example.com/shot1.png ", "https://cdn.example.com/shot2.png"}
	in.WhatsNew = "  Fixed a crash on launch  "
	in.RatingStars5, in.RatingStars4, in.RatingStars3, in.RatingStars2, in.RatingStars1 = &s5, &s4, &s3, &s2, &s1

	withData, err := svc.Create(ctx, orgID, in)
	if err != nil {
		t.Fatalf("Create with gallery data: %v", err)
	}
	if len(withData.Screenshots) != 2 || withData.Screenshots[0] != "https://cdn.example.com/shot1.png" {
		t.Fatalf("Screenshots = %#v, want trimmed URLs", withData.Screenshots)
	}
	if withData.WhatsNew != "Fixed a crash on launch" {
		t.Fatalf("WhatsNew = %q, want trimmed", withData.WhatsNew)
	}
	if withData.RatingStars5 == nil || *withData.RatingStars5 != 80.0 {
		t.Fatalf("RatingStars5 = %v, want 80.0", withData.RatingStars5)
	}
	if withData.RatingStars1 == nil || *withData.RatingStars1 != 1.0 {
		t.Fatalf("RatingStars1 = %v, want 1.0", withData.RatingStars1)
	}

	// A blank row from the frontend's add/remove screenshot list editor
	// must never persist as an empty-string URL.
	withBlank := validInput("Blank Screenshot Row")
	withBlank.Screenshots = []string{"https://cdn.example.com/shot1.png", "   ", ""}
	created2, err := svc.Create(ctx, orgID, withBlank)
	if err != nil {
		t.Fatalf("Create with a blank screenshot row: %v", err)
	}
	if len(created2.Screenshots) != 1 {
		t.Fatalf("Screenshots = %#v, want the blank rows dropped", created2.Screenshots)
	}

	newWhatsNew := "v2: added dark mode"
	updated, err := svc.Update(ctx, orgID, created.ID, pwa.UpdateInput{WhatsNew: &newWhatsNew})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.WhatsNew != newWhatsNew {
		t.Fatalf("WhatsNew after Update = %q, want %q", updated.WhatsNew, newWhatsNew)
	}
}

func TestCreateRejectsOutOfRangeGalleryFields(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))

	bad := validInput("Bad Screenshot URL")
	bad.Screenshots = []string{"not-a-url"}
	if _, err := svc.Create(ctx, orgID, bad); err == nil {
		t.Fatal("Create with an invalid screenshot URL succeeded, want a validation error")
	}

	tooHigh := 150.0
	bad = validInput("Bad Rating Star Percentage")
	bad.RatingStars5 = &tooHigh
	if _, err := svc.Create(ctx, orgID, bad); err == nil {
		t.Fatal("Create with ratingStars5=150 succeeded, want a validation error (must be 0-100)")
	}
}

func TestDuplicateCopiesGalleryFields(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))
	s5 := 90.0
	in := validInput("Source With Gallery")
	in.Screenshots = []string{"https://cdn.example.com/shot1.png"}
	in.WhatsNew = "Initial release"
	in.RatingStars5 = &s5
	created, err := svc.Create(ctx, orgID, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	dup, err := svc.Duplicate(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if len(dup.Screenshots) != 1 || dup.WhatsNew != "Initial release" || dup.RatingStars5 == nil || *dup.RatingStars5 != 90.0 {
		t.Fatalf("Duplicate = %+v, want gallery fields copied from the source", dup)
	}
}

// Detail-sheet fields (migration 00025): version/age rating/category/
// interactive elements back the listing's "detailed information" table.
// Unlike rating/size they're plain strings, so "" is the never-entered
// state the UI hides a row for — this asserts the default really is ""
// and that a round trip preserves what was entered.
func TestDetailFieldsRoundTripAndDefaultToEmpty(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))

	bare, err := svc.Create(ctx, orgID, validInput("No Detail Fields"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if bare.Version != "" || bare.AgeRating != "" || bare.Category != "" || bare.InteractiveElements != "" {
		t.Fatalf("unset detail fields = %+v, want empty strings", bare)
	}

	in := validInput("With Detail Fields")
	in.Version = " 1.0.0 " // leading/trailing space must be trimmed
	in.AgeRating = "12+"
	in.Category = "Casino"
	in.InteractiveElements = "In-game purchases"
	created, err := svc.Create(ctx, orgID, in)
	if err != nil {
		t.Fatalf("Create with detail fields: %v", err)
	}
	if created.Version != "1.0.0" {
		t.Fatalf("Version = %q, want %q (trimmed)", created.Version, "1.0.0")
	}

	got, err := svc.Get(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AgeRating != "12+" || got.Category != "Casino" || got.InteractiveElements != "In-game purchases" {
		t.Fatalf("Get = %+v, want the detail fields as created", got)
	}

	category := "Arcade"
	updated, err := svc.Update(ctx, orgID, created.ID, pwa.UpdateInput{Category: &category})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Category != "Arcade" || updated.Version != "1.0.0" {
		t.Fatalf("Update = %+v, want category changed and the other detail fields untouched", updated)
	}
}

func TestCreateRejectsOverlongDetailFields(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))

	bad := validInput("Overlong Category")
	bad.Category = strings.Repeat("x", 61)
	if _, err := svc.Create(ctx, orgID, bad); err == nil {
		t.Fatal("Create with a 61-character category succeeded, want a validation error")
	}
}

func TestDuplicateCopiesDetailFields(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))
	in := validInput("Source With Detail Fields")
	in.Version = "2.1.0"
	in.AgeRating = "18+"
	in.Category = "Casino"
	in.InteractiveElements = "Interactive elements"
	created, err := svc.Create(ctx, orgID, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	dup, err := svc.Duplicate(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup.Version != "2.1.0" || dup.AgeRating != "18+" || dup.Category != "Casino" || dup.InteractiveElements != "Interactive elements" {
		t.Fatalf("Duplicate = %+v, want detail fields copied from the source", dup)
	}
}

// contains_ads / in_app_purchases / updated_date (migration 00026):
// the first two replaced a hardcoded "no ads or paid content" claim in
// the preview, the third replaced today's date being printed as if it
// were the listing's own. updated_date crosses the API as YYYY-MM-DD.
func TestAdsPurchasesAndUpdatedDateRoundTrip(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))

	bare, err := svc.Create(ctx, orgID, validInput("No Flags"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if bare.ContainsAds || bare.InAppPurchases {
		t.Fatalf("unset flags = %+v, want both false", bare)
	}
	if bare.UpdatedDate != nil {
		t.Fatalf("UpdatedDate = %v, want nil when never entered", *bare.UpdatedDate)
	}

	date := "2026-08-21"
	in := validInput("With Flags")
	in.ContainsAds = true
	in.InAppPurchases = true
	in.UpdatedDate = &date
	created, err := svc.Create(ctx, orgID, in)
	if err != nil {
		t.Fatalf("Create with flags: %v", err)
	}
	if !created.ContainsAds || !created.InAppPurchases {
		t.Fatalf("flags = %+v, want both true", created)
	}
	if created.UpdatedDate == nil || *created.UpdatedDate != date {
		t.Fatalf("UpdatedDate = %v, want %q", created.UpdatedDate, date)
	}

	got, err := svc.Get(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.UpdatedDate == nil || *got.UpdatedDate != date {
		t.Fatalf("Get UpdatedDate = %v, want %q", got.UpdatedDate, date)
	}

	dup, err := svc.Duplicate(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if !dup.ContainsAds || !dup.InAppPurchases || dup.UpdatedDate == nil || *dup.UpdatedDate != date {
		t.Fatalf("Duplicate = %+v, want the flags and date copied", dup)
	}
}

// A cleared date input sends "", which must land as SQL NULL rather
// than failing the ::date cast.
func TestBlankUpdatedDateBecomesNull(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))
	blank := ""
	in := validInput("Blank Date")
	in.UpdatedDate = &blank
	created, err := svc.Create(ctx, orgID, in)
	if err != nil {
		t.Fatalf("Create with a blank updatedDate: %v", err)
	}
	if created.UpdatedDate != nil {
		t.Fatalf("UpdatedDate = %v, want nil for a blank input", *created.UpdatedDate)
	}
}

func TestCreateRejectsMalformedUpdatedDate(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))
	bad := validInput("Bad Date")
	notADate := "21.08.2026"
	bad.UpdatedDate = &notADate
	if _, err := svc.Create(ctx, orgID, bad); err == nil {
		t.Fatal("Create with updatedDate=21.08.2026 succeeded, want a validation error")
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)

	svc := pwa.NewService(pwa.NewRepository(pool))
	created, err := svc.Create(ctx, orgA, validInput("Org A PWA"))
	if err != nil {
		t.Fatalf("creating for org A: %v", err)
	}

	t.Run("get", func(t *testing.T) {
		if _, err := svc.Get(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B fetched org A's pwa, want not-found")
		}
	})
	t.Run("update", func(t *testing.T) {
		name := "hijacked"
		if _, err := svc.Update(ctx, orgB, created.ID, pwa.UpdateInput{Name: &name}); err == nil {
			t.Fatal("org B updated org A's pwa, want not-found")
		}
	})
	t.Run("delete", func(t *testing.T) {
		if err := svc.Delete(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B deleted org A's pwa, want not-found")
		}
	})
	t.Run("list", func(t *testing.T) {
		pwas, _, err := svc.List(ctx, orgB, pagination.Page{Limit: pagination.MaxLimit})
		if err != nil {
			t.Fatalf("listing as org B: %v", err)
		}
		if len(pwas) != 0 {
			t.Fatalf("org B saw %d of org A's pwas, want 0", len(pwas))
		}
	})
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

func TestCommentsReplaceRoundTripAndValidation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := pwa.NewService(pwa.NewRepository(pool))
	p, err := svc.Create(ctx, orgID, validInput("Comments PWA"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	empty, err := svc.ListComments(ctx, orgID, p.ID)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("ListComments on a fresh PWA = %#v, %v; want an empty slice", empty, err)
	}

	saved, err := svc.ReplaceComments(ctx, orgID, p.ID, []pwa.CommentInput{
		{Geo: " ru ", Lang: "RU", Author: " Анна ", Rating: 5, Text: " Отлично "},
		{Geo: "US", Lang: "en", Author: "Sam", Rating: 3, Text: "Fine"},
	})
	if err != nil {
		t.Fatalf("ReplaceComments: %v", err)
	}
	if len(saved) != 2 || saved[0].Geo != "RU" || saved[0].Lang != "ru" || saved[0].Author != "Анна" || saved[0].Text != "Отлично" {
		t.Fatalf("saved = %+v, want normalised values", saved)
	}

	rich, err := svc.ReplaceComments(ctx, orgID, p.ID, []pwa.CommentInput{{
		Geo: "ML", Lang: "fr", Author: "Awa", AvatarURL: "https://cdn.example.com/a.png", Rating: 4, Likes: 12,
		Date: "2026-09-30", Text: "Bien", Reply: " Merci ",
	}})
	if err != nil {
		t.Fatalf("ReplaceComments with details: %v", err)
	}
	if r := rich[0]; r.Likes != 12 || r.Date != "2026-09-30" || r.Reply != "Merci" || r.AvatarURL == "" {
		t.Fatalf("details not kept: %+v", r)
	}
	if back, _ := svc.ListComments(ctx, orgID, p.ID); len(back) != 1 || back[0].Date != "2026-09-30" || back[0].Likes != 12 || back[0].Reply != "Merci" {
		t.Fatalf("details lost on read-back: %+v", back)
	}
	noDate, _ := svc.ReplaceComments(ctx, orgID, p.ID, []pwa.CommentInput{{Geo: "ML", Lang: "fr", Author: "Awa", Rating: 4, Text: "Bien"}})
	if back, _ := svc.ListComments(ctx, orgID, p.ID); len(noDate) != 1 || back[0].Date != "" {
		t.Fatalf("a comment without a date came back with %q", back[0].Date)
	}

	// Replaced wholesale, order kept.
	if _, err := svc.ReplaceComments(ctx, orgID, p.ID, []pwa.CommentInput{{Geo: "DE", Lang: "de", Author: "Max", Rating: 4, Text: "Gut"}}); err != nil {
		t.Fatalf("second ReplaceComments: %v", err)
	}
	got, _ := svc.ListComments(ctx, orgID, p.ID)
	if len(got) != 1 || got[0].Author != "Max" {
		t.Fatalf("after replace = %+v, want only Max", got)
	}

	for name, bad := range map[string]pwa.CommentInput{
		"geo":    {Geo: "RUS", Lang: "ru", Author: "A", Rating: 5, Text: "x"},
		"lang":   {Geo: "RU", Lang: "r", Author: "A", Rating: 5, Text: "x"},
		"author": {Geo: "RU", Lang: "ru", Author: " ", Rating: 5, Text: "x"},
		"rating": {Geo: "RU", Lang: "ru", Author: "A", Rating: 6, Text: "x"},
		"text":   {Geo: "RU", Lang: "ru", Author: "A", Rating: 5, Text: " "},
		"long":   {Geo: "RU", Lang: "ru", Author: "A", Rating: 5, Text: strings.Repeat("я", 501)},
		"reply":  {Geo: "RU", Lang: "ru", Author: "A", Rating: 5, Text: "x", Reply: strings.Repeat("я", 501)},
		"date":   {Geo: "RU", Lang: "ru", Author: "A", Rating: 5, Text: "x", Date: "30.09.2026"},
		"likes":  {Geo: "RU", Lang: "ru", Author: "A", Rating: 5, Text: "x", Likes: -1},
		"avatar": {Geo: "RU", Lang: "ru", Author: "A", Rating: 5, Text: "x", AvatarURL: "javascript:alert(1)"},
	} {
		if _, err := svc.ReplaceComments(ctx, orgID, p.ID, []pwa.CommentInput{bad}); err == nil {
			t.Errorf("invalid %s accepted", name)
		}
	}
	if kept, _ := svc.ListComments(ctx, orgID, p.ID); len(kept) != 1 {
		t.Errorf("a rejected save changed the stored comments: %+v", kept)
	}

	// Deleting the PWA takes its comments with it.
	if err := svc.Delete(ctx, orgID, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pwa_comments WHERE pwa_id = $1`, p.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("orphaned comments after delete: %d, %v", n, err)
	}
}

func TestCommentsTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	svc := pwa.NewService(pwa.NewRepository(pool))
	p, err := svc.Create(ctx, orgA, validInput("Org A PWA"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.ReplaceComments(ctx, orgA, p.ID, []pwa.CommentInput{{Geo: "RU", Lang: "ru", Author: "A", Rating: 5, Text: "mine"}}); err != nil {
		t.Fatalf("ReplaceComments: %v", err)
	}

	if _, err := svc.ListComments(ctx, orgB, p.ID); err == nil {
		t.Error("org B listed org A's comments")
	}
	if _, err := svc.ReplaceComments(ctx, orgB, p.ID, []pwa.CommentInput{{Geo: "RU", Lang: "ru", Author: "B", Rating: 1, Text: "theirs"}}); err == nil {
		t.Error("org B replaced org A's comments")
	}
	got, _ := svc.ListComments(ctx, orgA, p.ID)
	if len(got) != 1 || got[0].Text != "mine" {
		t.Errorf("org A's comments changed: %+v", got)
	}
}
