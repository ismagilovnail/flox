package pixel

import (
	"context"
	"fmt"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
	"github.com/ismagilovnail/flox/apps/internal/urlsafety"
)

const (
	nameMinLen       = 2
	nameMaxLen       = 100
	pixelIDMax       = 80
	accessTokenMax   = 1000
	testEventCodeMax = 100
	urlMax           = 2000
	eventNameMax     = 100

	// DeliveriesLimit is how many rows of a pixel's delivery log the API
	// returns: the recent ones an operator checks a setup against.
	DeliveriesLimit = 100
)

// sendable is the part of a pixel that decides how it sends — validated as
// a whole, since PATCH may change any one piece of it.
type sendable struct {
	provider    Provider
	pixelID     string
	events      []string
	accessToken string
	testCode    string
	url         string
	eventNames  map[string]string
}

// normalize trims what an operator pastes and drops what the provider does
// not use, so a stale URL or token never lingers on a pixel that cannot use
// it.
func (v *sendable) normalize() {
	v.pixelID = strings.TrimSpace(v.pixelID)
	v.accessToken = strings.TrimSpace(v.accessToken)
	v.testCode = strings.TrimSpace(v.testCode)
	v.url = strings.TrimSpace(v.url)
	if v.provider == ProviderGeneric {
		v.pixelID, v.accessToken, v.testCode = "", "", ""
	} else {
		v.url = ""
	}
	names := map[string]string{}
	for k, name := range v.eventNames {
		if name = strings.TrimSpace(name); name != "" {
			names[k] = name
		}
	}
	v.eventNames = names
}

// validate adds field errors to fields. providerChanged: the provider is
// being set (create, or a switch) — twitter cannot be (Provider.Sends), but
// an existing twitter pixel can still be edited.
func (v sendable) validate(fields map[string]string, providerChanged bool) {
	switch {
	case !v.provider.Valid():
		fields["provider"] = "must be one of facebook, tiktok, snapchat, generic"
	case providerChanged && !v.provider.Sends():
		fields["provider"] = "twitter cannot send events: its Conversions API needs OAuth 1.0a app credentials FLOX does not have"
	}
	if len(v.pixelID) > pixelIDMax {
		fields["pixelId"] = fmt.Sprintf("must be at most %d characters", pixelIDMax)
	}
	if err := validateEvents(v.events); err != "" {
		fields["events"] = err
	}
	if len(v.accessToken) > accessTokenMax {
		fields["accessToken"] = fmt.Sprintf("must be at most %d characters", accessTokenMax)
	}
	if len(v.testCode) > testEventCodeMax {
		fields["testEventCode"] = fmt.Sprintf("must be at most %d characters", testEventCodeMax)
	}
	if v.provider == ProviderGeneric {
		switch {
		case v.url == "":
			fields["url"] = "required for the generic provider"
		case len(v.url) > urlMax:
			fields["url"] = fmt.Sprintf("must be at most %d characters", urlMax)
		case urlsafety.ValidateURL(v.url) != nil:
			fields["url"] = "must be a valid absolute http(s) URL"
		}
	}
	for k, name := range v.eventNames {
		if !isValidEventType(k) {
			fields["eventNames"] = fmt.Sprintf("%q is not an event a pixel can report", k)
			break
		}
		if len(name) > eventNameMax {
			fields["eventNames"] = fmt.Sprintf("names must be at most %d characters", eventNameMax)
			break
		}
	}
}

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// List returns one page of the organization's pixels and their total.
func (s *Service) List(ctx context.Context, orgID string, page pagination.Page) ([]Pixel, int, error) {
	items, err := s.repo.List(ctx, orgID, page)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.Count(ctx, orgID)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *Service) Get(ctx context.Context, orgID, id string) (Pixel, error) {
	return s.repo.GetByID(ctx, orgID, id)
}

func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (Pixel, error) {
	in.Name = strings.TrimSpace(in.Name)
	fields := map[string]string{}

	if len(in.Name) < nameMinLen || len(in.Name) > nameMaxLen {
		fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
	}
	v := sendable{
		provider: in.Provider, pixelID: in.PixelID, events: in.Events, accessToken: in.AccessToken,
		testCode: in.TestEventCode, url: in.URL, eventNames: in.EventNames,
	}
	v.normalize()
	v.validate(fields, true)

	if len(fields) > 0 {
		return Pixel{}, apierror.Validation("invalid pixel", fields)
	}
	in.PixelID, in.AccessToken, in.TestEventCode, in.URL, in.EventNames = v.pixelID, v.accessToken, v.testCode, v.url, v.eventNames
	return s.repo.Create(ctx, idgen.New(), orgID, in)
}

func (s *Service) Update(ctx context.Context, orgID, id string, in UpdateInput) (Pixel, error) {
	current, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Pixel{}, err
	}

	fields := map[string]string{}

	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		in.Name = &trimmed
		if len(trimmed) < nameMinLen || len(trimmed) > nameMaxLen {
			fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
		}
	}
	if in.Status != nil && !in.Status.Valid() {
		fields["status"] = "must be one of active, paused, archived"
	}

	// The pixel as it will be, validated whole. The current token is not
	// readable here, so a token left out stays as it is ("unknown, set").
	v := sendable{
		provider: current.Provider, pixelID: current.PixelID, events: current.Events,
		testCode: current.TestEventCode, url: current.URL, eventNames: current.EventNames,
	}
	if in.Provider != nil {
		v.provider = *in.Provider
	}
	if in.PixelID != nil {
		v.pixelID = *in.PixelID
	}
	if in.Events != nil {
		v.events = *in.Events
	}
	if in.AccessToken != nil {
		v.accessToken = *in.AccessToken
	}
	if in.TestEventCode != nil {
		v.testCode = *in.TestEventCode
	}
	if in.URL != nil {
		v.url = *in.URL
	}
	if in.EventNames != nil {
		v.eventNames = *in.EventNames
	}
	v.normalize()
	v.validate(fields, in.Provider != nil && *in.Provider != current.Provider)
	if len(fields) > 0 {
		return Pixel{}, apierror.Validation("invalid pixel", fields)
	}

	// Write back what normalize settled; a provider switch to generic also
	// removes the token it no longer uses.
	in.PixelID, in.TestEventCode, in.URL, in.EventNames = &v.pixelID, &v.testCode, &v.url, &v.eventNames
	if in.AccessToken != nil {
		in.AccessToken = &v.accessToken
	}
	if !v.provider.UsesToken() && current.HasAccessToken {
		none := ""
		in.AccessToken = &none
	}
	return s.repo.Update(ctx, orgID, id, in)
}

func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	return s.repo.Delete(ctx, orgID, id)
}

// Duplicate copies every field, the access token included (still
// encrypted); status is preserved separately via a follow-up Update.
func (s *Service) Duplicate(ctx context.Context, orgID, id string) (Pixel, error) {
	source, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Pixel{}, err
	}

	created, err := s.repo.Create(ctx, idgen.New(), orgID, CreateInput{
		Name:          source.Name + " (Copy)",
		Provider:      source.Provider,
		PixelID:       source.PixelID,
		Events:        source.Events,
		TestEventCode: source.TestEventCode,
		URL:           source.URL,
		EventNames:    source.EventNames,
	})
	if err != nil {
		return Pixel{}, err
	}
	if source.HasAccessToken {
		if err := s.repo.copyToken(ctx, orgID, source.ID, created.ID); err != nil {
			return Pixel{}, err
		}
	}
	if source.Status != StatusActive {
		status := source.Status
		return s.repo.Update(ctx, orgID, created.ID, UpdateInput{Status: &status})
	}
	return s.repo.GetByID(ctx, orgID, created.ID)
}

// Deliveries is the pixel's recent delivery log (DeliveriesLimit rows).
func (s *Service) Deliveries(ctx context.Context, orgID, id string) ([]Delivery, error) {
	if _, err := s.repo.GetByID(ctx, orgID, id); err != nil {
		return nil, err
	}
	return s.repo.Deliveries(ctx, orgID, id, DeliveriesLimit)
}

func (s *Service) Pause(ctx context.Context, orgID, id string) (Pixel, error) {
	p, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Pixel{}, err
	}
	switch p.Status {
	case StatusPaused:
		return p, nil
	case StatusActive:
		status := StatusPaused
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Pixel{}, apierror.Conflict(fmt.Sprintf("cannot pause a pixel with status %q", p.Status))
	}
}

func (s *Service) Activate(ctx context.Context, orgID, id string) (Pixel, error) {
	p, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Pixel{}, err
	}
	switch p.Status {
	case StatusActive:
		return p, nil
	case StatusPaused:
		status := StatusActive
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Pixel{}, apierror.Conflict(fmt.Sprintf("cannot activate a pixel with status %q", p.Status))
	}
}

// validateEvents returns a non-empty field-error message when events is
// invalid, or "" when it's fine. At least one event is required (matches
// the frontend zod schema's z.array(...).min(1)); every value must be one
// of ValidEventTypes.
func validateEvents(events []string) string {
	if len(events) == 0 {
		return "select at least one event"
	}
	for _, e := range events {
		if !isValidEventType(e) {
			return fmt.Sprintf("%q is not a recognized event type", e)
		}
	}
	return ""
}
