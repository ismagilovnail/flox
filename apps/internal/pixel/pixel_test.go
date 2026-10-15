package pixel_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
)

func TestCreateGetUpdateDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pixel.NewService(pixel.NewRepository(pool, testKey))

	created, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "Facebook — Sweeps conversions", Provider: pixel.ProviderFacebook, PixelID: "1029384756102938",
		Events: []string{"PWA_INSTALL", "CPA_ACCEPT"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != pixel.StatusActive {
		t.Fatalf("Status = %q, want active (the DB default)", created.Status)
	}
	if len(created.Events) != 2 {
		t.Fatalf("Events = %v, want 2 entries", created.Events)
	}

	got, err := svc.Get(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != created.Name {
		t.Fatalf("Get returned %+v, want the just-created row", got)
	}

	newName := "Facebook — Sweeps (renamed)"
	updated, err := svc.Update(ctx, orgID, created.ID, pixel.UpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("Name = %q, want %q", updated.Name, newName)
	}
	if len(updated.Events) != 2 {
		t.Fatalf("Events after a name-only update = %v, want unchanged (2 entries)", updated.Events)
	}

	if err := svc.Delete(ctx, orgID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, orgID, created.ID); err == nil {
		t.Fatal("Get after Delete succeeded, want not-found")
	}
}

func TestCreateAllowsEmptyPixelID(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pixel.NewService(pixel.NewRepository(pool, testKey))
	created, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "Facebook — no id yet", Provider: pixel.ProviderFacebook, PixelID: "",
		Events: []string{"CPA_HOLD"},
	})
	if err != nil {
		t.Fatalf("Create with an empty pixelId failed, want success (pixelId is optional): %v", err)
	}
	if created.PixelID != "" {
		t.Fatalf("PixelID = %q, want empty", created.PixelID)
	}
}

func TestCreateRejectsInvalidShapes(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pixel.NewService(pixel.NewRepository(pool, testKey))

	if _, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "Bad Provider", Provider: "myspace", Events: []string{"CPA_HOLD"},
	}); err == nil {
		t.Fatal("Create with an unrecognized provider succeeded, want a validation error")
	}
	if _, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "No Events", Provider: pixel.ProviderTikTok, Events: nil,
	}); err == nil {
		t.Fatal("Create with zero events succeeded, want a validation error")
	}
	if _, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "Bad Event", Provider: pixel.ProviderTikTok, Events: []string{"NOT_A_REAL_EVENT"},
	}); err == nil {
		t.Fatal("Create with an unrecognized event type succeeded, want a validation error")
	}
	if _, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "X", Provider: pixel.ProviderTikTok, Events: []string{"CPA_HOLD"},
	}); err == nil {
		t.Fatal("Create with a 1-character name succeeded, want a validation error")
	}
	if _, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "Not offered", Provider: pixel.ProviderTikTok, Events: []string{"NOTIFICATION_SUBSCRIBE"},
	}); err == nil {
		t.Fatal("Create with NOTIFICATION_SUBSCRIBE (no platform mapping) succeeded, want a validation error")
	}
	if _, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "Twitter", Provider: pixel.ProviderTwitter, Events: []string{"CPA_HOLD"},
	}); err == nil {
		t.Fatal("Create with provider twitter succeeded, want a validation error (it cannot send)")
	}
	for _, url := range []string{"", "not a url", "ftp://example.com/p", "http://127.0.0.1/p"} {
		if _, err := svc.Create(ctx, orgID, pixel.CreateInput{
			Name: "Generic", Provider: pixel.ProviderGeneric, URL: url, Events: []string{"CPA_HOLD"},
		}); err == nil {
			t.Fatalf("Create generic with URL %q succeeded, want a validation error", url)
		}
	}
	if _, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "Bad override", Provider: pixel.ProviderFacebook, Events: []string{"CPA_HOLD"},
		EventNames: map[string]string{"SOURCE_CLICK": "PageView"},
	}); err == nil {
		t.Fatal("Create with an event name override for an event a pixel cannot report succeeded, want a validation error")
	}
}

func TestPauseActivateTransitions(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pixel.NewService(pixel.NewRepository(pool, testKey))
	created, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "Toggle Pixel", Provider: pixel.ProviderSnapchat, Events: []string{"CPA_HOLD"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	paused, err := svc.Pause(ctx, orgID, created.ID)
	if err != nil || paused.Status != pixel.StatusPaused {
		t.Fatalf("Pause: got %+v, err %v", paused, err)
	}
	if again, err := svc.Pause(ctx, orgID, created.ID); err != nil || again.Status != pixel.StatusPaused {
		t.Fatalf("Pause (idempotent): got %+v, err %v", again, err)
	}

	active, err := svc.Activate(ctx, orgID, created.ID)
	if err != nil || active.Status != pixel.StatusActive {
		t.Fatalf("Activate: got %+v, err %v", active, err)
	}

	archived := pixel.StatusArchived
	if _, err := svc.Update(ctx, orgID, created.ID, pixel.UpdateInput{Status: &archived}); err != nil {
		t.Fatalf("archiving via Update: %v", err)
	}
	if _, err := svc.Pause(ctx, orgID, created.ID); err == nil {
		t.Fatal("Pause on an archived pixel succeeded, want a conflict error")
	}
	if _, err := svc.Activate(ctx, orgID, created.ID); err == nil {
		t.Fatal("Activate on an archived pixel succeeded, want a conflict error")
	}
}

func TestDuplicateKeepsStatus(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	svc := pixel.NewService(pixel.NewRepository(pool, testKey))
	created, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "Original", Provider: pixel.ProviderTikTok, PixelID: "abc123",
		Events: []string{"PWA_INSTALL", "CPA_ACCEPT"}, AccessToken: "tt-secret-token-9876",
		TestEventCode: "TEST123", EventNames: map[string]string{"CPA_ACCEPT": "Purchase"},
	})
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
	if dup.Status != pixel.StatusPaused {
		t.Fatalf("Status = %q, want paused (copied from the source, not reset)", dup.Status)
	}
	if dup.PixelID != created.PixelID {
		t.Fatalf("PixelID = %q, want %q (copied verbatim, no server-computed value here)", dup.PixelID, created.PixelID)
	}
	if len(dup.Events) != 2 {
		t.Fatalf("Events = %v, want 2 entries copied from the source", dup.Events)
	}
	if !dup.HasAccessToken || dup.AccessTokenLast4 != "9876" || dup.TestEventCode != "TEST123" || dup.EventNames["CPA_ACCEPT"] != "Purchase" {
		t.Fatalf("duplicate = %+v, want the token, test code and event names copied", dup)
	}
	creds, err := pixel.NewRepository(pool, testKey).Credentials(ctx, orgID, dup.ID)
	if err != nil || creds.AccessToken != "tt-secret-token-9876" {
		t.Fatalf("duplicate's token = %q (%v), want the source's", creds.AccessToken, err)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)

	svc := pixel.NewService(pixel.NewRepository(pool, testKey))
	created, err := svc.Create(ctx, orgA, pixel.CreateInput{
		Name: "Org A Pixel", Provider: pixel.ProviderGeneric, URL: "https://example.com/p?c={click_id}", Events: []string{"CPA_HOLD"},
	})
	if err != nil {
		t.Fatalf("creating for org A: %v", err)
	}

	t.Run("get", func(t *testing.T) {
		if _, err := svc.Get(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B fetched org A's pixel, want not-found")
		}
	})
	t.Run("update", func(t *testing.T) {
		name := "hijacked"
		if _, err := svc.Update(ctx, orgB, created.ID, pixel.UpdateInput{Name: &name}); err == nil {
			t.Fatal("org B updated org A's pixel, want not-found")
		}
	})
	t.Run("delete", func(t *testing.T) {
		if err := svc.Delete(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B deleted org A's pixel, want not-found")
		}
	})
	t.Run("deliveries", func(t *testing.T) {
		if _, err := svc.Deliveries(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B read org A's pixel delivery log, want not-found")
		}
	})
	t.Run("credentials", func(t *testing.T) {
		if _, err := pixel.NewRepository(pool, testKey).Credentials(ctx, orgB, created.ID); err == nil {
			t.Fatal("org B read org A's pixel credentials, want not-found")
		}
	})
	t.Run("list", func(t *testing.T) {
		pixels, _, err := svc.List(ctx, orgB, pagination.Page{Limit: pagination.MaxLimit})
		if err != nil {
			t.Fatalf("listing as org B: %v", err)
		}
		if len(pixels) != 0 {
			t.Fatalf("org B saw %d of org A's pixels, want 0", len(pixels))
		}
	})
}

// testKey encrypts access tokens in these tests.
var testKey = []byte("0123456789abcdef0123456789abcdef")

func TestAccessTokenIsEncryptedAndNeverReturned(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	repo := pixel.NewRepository(pool, testKey)
	svc := pixel.NewService(repo)

	created, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "FB", Provider: pixel.ProviderFacebook, PixelID: "123", Events: []string{"CPA_ACCEPT"},
		AccessToken: "  EAAB-secret-token-abcd  ",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !created.HasAccessToken || created.AccessTokenLast4 != "abcd" {
		t.Fatalf("created = %+v, want hasAccessToken and last4 abcd", created)
	}
	raw, _ := json.Marshal(created)
	if strings.Contains(string(raw), "secret") {
		t.Fatalf("pixel JSON contains the token: %s", raw)
	}
	var stored string
	if err := pool.QueryRow(ctx, `SELECT access_token FROM pixels WHERE id = $1`, created.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == "" || strings.Contains(stored, "secret") {
		t.Fatalf("stored access_token = %q, want ciphertext", stored)
	}
	creds, err := repo.Credentials(ctx, orgID, created.ID)
	if err != nil || creds.AccessToken != "EAAB-secret-token-abcd" {
		t.Fatalf("Credentials = %q (%v), want the trimmed token", creds.AccessToken, err)
	}

	// Left out of a PATCH: kept. "": removed.
	name := "FB renamed"
	updated, err := svc.Update(ctx, orgID, created.ID, pixel.UpdateInput{Name: &name})
	if err != nil || !updated.HasAccessToken {
		t.Fatalf("name-only update: %+v, %v — want the token kept", updated, err)
	}
	empty := ""
	updated, err = svc.Update(ctx, orgID, created.ID, pixel.UpdateInput{AccessToken: &empty})
	if err != nil || updated.HasAccessToken || updated.AccessTokenLast4 != "" {
		t.Fatalf("token removal: %+v, %v — want no token", updated, err)
	}
}

func TestSwitchToGenericDropsTokenAndRequiresURL(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := pixel.NewService(pixel.NewRepository(pool, testKey))

	created, err := svc.Create(ctx, orgID, pixel.CreateInput{
		Name: "TT", Provider: pixel.ProviderTikTok, PixelID: "C123", Events: []string{"CPA_HOLD"},
		AccessToken: "tok-1234", TestEventCode: "TEST1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	generic := pixel.ProviderGeneric
	if _, err := svc.Update(ctx, orgID, created.ID, pixel.UpdateInput{Provider: &generic}); err == nil {
		t.Fatal("switching to generic without a URL succeeded, want a validation error")
	}
	url := "https://track.example.com/px?cid={click_id}&e={status}"
	updated, err := svc.Update(ctx, orgID, created.ID, pixel.UpdateInput{Provider: &generic, URL: &url})
	if err != nil {
		t.Fatalf("switching to generic: %v", err)
	}
	if updated.HasAccessToken || updated.PixelID != "" || updated.TestEventCode != "" || updated.URL != url {
		t.Fatalf("after the switch: %+v — want token, pixel id and test code dropped, URL set", updated)
	}
}

func TestExistingTwitterPixelStaysEditable(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := pixel.NewService(pixel.NewRepository(pool, testKey))

	id := idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO pixels (id, organization_id, name, provider, pixel_id, events)
		VALUES ($1, $2, 'Old twitter', 'twitter', 'tw1', '{CPA_HOLD}')`, id, orgID); err != nil {
		t.Fatal(err)
	}
	name := "Old twitter (renamed)"
	if _, err := svc.Update(ctx, orgID, id, pixel.UpdateInput{Name: &name}); err != nil {
		t.Fatalf("renaming an existing twitter pixel: %v", err)
	}
	fb := pixel.ProviderFacebook
	if _, err := svc.Update(ctx, orgID, id, pixel.UpdateInput{Provider: &fb}); err != nil {
		t.Fatalf("moving a twitter pixel to facebook: %v", err)
	}
	tw := pixel.ProviderTwitter
	if _, err := svc.Update(ctx, orgID, id, pixel.UpdateInput{Provider: &tw}); err == nil {
		t.Fatal("switching back to twitter succeeded, want a validation error")
	}
}

func TestEventName(t *testing.T) {
	cases := []struct {
		provider  pixel.Provider
		overrides map[string]string
		event     string
		want      string
	}{
		{pixel.ProviderFacebook, nil, "CPA_ACCEPT", "Purchase"},
		{pixel.ProviderFacebook, map[string]string{"CPA_ACCEPT": "Deposit"}, "CPA_ACCEPT", "Deposit"},
		{pixel.ProviderTikTok, nil, "CPA_HOLD", "SubmitForm"},
		{pixel.ProviderGeneric, nil, "CPA_REDEP", "CPA_REDEP"},
	}
	for _, c := range cases {
		if got := pixel.EventName(c.provider, c.overrides, c.event); got != c.want {
			t.Errorf("EventName(%s, %v, %s) = %q, want %q", c.provider, c.overrides, c.event, got, c.want)
		}
	}
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
