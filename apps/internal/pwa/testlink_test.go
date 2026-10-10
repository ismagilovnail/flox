package pwa_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
	"github.com/ismagilovnail/flox/apps/internal/pwa"
)

func TestTestLink(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA, orgB := seedOrg(t, ctx, pool), seedOrg(t, ctx, pool)
	svc := pwa.NewService(pwa.NewRepository(pool))
	p, err := svc.Create(ctx, orgA, pwa.CreateInput{
		Name: "Link App", ShortName: "Link", ThemeColor: "#000000", BackgroundColor: "#ffffff",
		IconURL: "https://cdn.example/i.png", StartURL: "/install",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = svc.TestLink(ctx, orgA, p.ID)
	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured TestLink err = %v, want 503", err)
	}

	signer := landinglink.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	svc.SetTestLinks(pwa.TestLinks{Signer: signer, TrackerURL: "http://localhost:8081/", AppURL: "http://localhost:3000"})

	link, err := svc.TestLink(ctx, orgA, p.ID)
	if err != nil {
		t.Fatalf("TestLink: %v", err)
	}
	if !strings.HasPrefix(link, "http://localhost:8081/pwa/") || !strings.HasSuffix(link, "/") {
		t.Fatalf("link = %q", link)
	}
	token := strings.TrimSuffix(strings.TrimPrefix(link, "http://localhost:8081/pwa/"), "/")
	c, err := signer.Verify(token, time.Now())
	if err != nil {
		t.Fatalf("minted token doesn't verify: %v", err)
	}
	if !c.Test || c.PwaID != p.ID || c.OrganizationID != orgA || c.Destination != "http://localhost:3000/pwa/"+p.ID {
		t.Fatalf("claims = %+v, want a Test link for this PWA back to its editor", c)
	}
	// Honoured only on the tracker's public host, not on any customer's
	// tracking domain (tech audit M4).
	if !c.OnHost("localhost:8081") || c.OnHost("customer.example") {
		t.Fatalf("test link host = %q, want localhost only", c.Host)
	}

	if _, err := svc.TestLink(ctx, orgB, p.ID); err == nil {
		t.Fatal("org B minted a test link for org A's PWA")
	}
}
