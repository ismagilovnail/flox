package pwa

import (
	"context"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/landinglink"
)

// TestLinks mints the PWA constructor's "open test page" links: a real,
// signed apps/tracker install-page URL for one PWA, marked Test so the
// tracker serves it exactly as a visitor would get it (manifest, service
// worker, install prompt) but records no events. "Continue"/"open" lead
// back to this PWA's editor page.
type TestLinks struct {
	Signer     *landinglink.Signer
	TrackerURL string // where a browser reaches apps/tracker, e.g. http://localhost:8081
	AppURL     string // the web app, for the "back to the editor" destination
}

// SetTestLinks enables TestLink. Left unset (no LANDING_LINK_KEY or no
// TRACKER_PUBLIC_URL configured), TestLink answers Unavailable.
func (s *Service) SetTestLinks(t TestLinks) { s.testLinks = &t }

func (s *Service) TestLink(ctx context.Context, orgID, id string) (string, error) {
	p, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return "", err
	}
	if s.testLinks == nil {
		return "", apierror.Unavailable("test links are not configured (LANDING_LINK_KEY / TRACKER_PUBLIC_URL)")
	}
	token := s.testLinks.Signer.Sign(landinglink.Claims{
		OrganizationID: orgID,
		CampaignID:     "",
		ClickID:        "test-" + idgen.New(),
		PwaID:          p.ID,
		Destination:    strings.TrimSuffix(s.testLinks.AppURL, "/") + "/pwa/" + p.ID,
		IssuedAt:       time.Now().Unix(),
		Test:           true,
	})
	return strings.TrimSuffix(s.testLinks.TrackerURL, "/") + "/pwa/" + token + "/", nil
}
