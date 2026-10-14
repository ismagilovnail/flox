package contentgallery_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/contentgallery"
	"github.com/ismagilovnail/flox/apps/internal/landing"
	"github.com/ismagilovnail/flox/apps/internal/pwa"
)

// TestUseSystemItems runs every hand-off for real: a creative lands in the
// team library, a landing template becomes an internal landing, and a PWA
// template's fields (with copied images) pass the PWA service's own
// validation once a start URL is added.
func TestUseSystemItems(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	otherOrg := seedOrg(t, ctx, pool)
	userID := seedUser(t, ctx, pool)
	storageClient := mustStorage(t)
	landings := landing.NewService(landing.NewRepository(pool), storageClient, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc := contentgallery.NewService(contentgallery.NewRepository(pool), storageClient).WithLandings(landings)

	creative, err := svc.UseSystemItem(ctx, orgID, userID, "creative-install-banner", contentgallery.SystemUseInput{})
	if err != nil || creative.File == nil || creative.File.OrganizationID != orgID || creative.File.MimeType != "image/png" || creative.File.URL == "" {
		t.Fatalf("creative = %+v, %v", creative, err)
	}

	l, err := svc.UseSystemItem(ctx, orgID, userID, "landing-quiz", contentgallery.SystemUseInput{Name: "My quiz"})
	if err != nil || l.Landing == nil {
		t.Fatalf("landing = %+v, %v", l, err)
	}
	if l.Landing.Name != "My quiz" || l.Landing.Type != landing.TypeInternal || l.Landing.LogoURL == "" || l.Landing.FileCount < 3 {
		t.Fatalf("landing = %+v, want an internal landing named My quiz with its logo and files", l.Landing)
	}
	if _, err := landings.Get(ctx, otherOrg, l.Landing.ID); err == nil {
		t.Fatal("another organization can read the landing")
	}

	p, err := svc.UseSystemItem(ctx, orgID, userID, "pwa-pulse", contentgallery.SystemUseInput{})
	if err != nil || p.PWA == nil {
		t.Fatalf("pwa = %+v, %v", p, err)
	}
	if shots, _ := p.PWA["screenshots"].([]string); len(shots) != 3 {
		t.Fatalf("screenshots = %v, want 3 copied URLs", p.PWA["screenshots"])
	}
	p.PWA["startUrl"] = "https://example.test/start"
	raw, _ := json.Marshal(p.PWA)
	var in pwa.CreateInput
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatalf("decoding the template into pwa.CreateInput: %v", err)
	}
	created, err := pwa.NewService(pwa.NewRepository(pool)).Create(ctx, orgID, in)
	if err != nil {
		t.Fatalf("the template does not pass PWA validation: %v", err)
	}
	if created.Name != "Pulse Arcade" || created.IconURL != p.PWA["iconUrl"] {
		t.Fatalf("created PWA = %+v", created)
	}

	// Everything copied is the team's own: 1 creative + 1 logo + 1 icon + 3 screenshots.
	files, err := svc.ListFiles(ctx, orgID, contentgallery.ListFilesParams{})
	if err != nil || len(files.Files) != 6 {
		t.Fatalf("team files = %d, %v; want 6", len(files.Files), err)
	}
	if other, _ := svc.ListFiles(ctx, otherOrg, contentgallery.ListFilesParams{}); len(other.Files) != 0 {
		t.Fatalf("another organization sees %d copied files", len(other.Files))
	}

	if _, err := svc.UseSystemItem(ctx, orgID, userID, "no-such-item", contentgallery.SystemUseInput{}); err == nil {
		t.Fatal("an unknown item was used")
	}
}
