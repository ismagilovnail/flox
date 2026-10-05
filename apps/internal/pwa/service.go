package pwa

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

const (
	nameMinLen        = 2
	nameMaxLen        = 80
	shortNameMaxLen   = 20
	developerMaxLen   = 100
	descriptionMaxLen = 4000
	downloadsLabelMax = 20
	whatsNewMaxLen    = 2000
	// No cap on screenshot count — the reference's own upload widget
	// advertises "No limit on quantity" and screenshots is a plain
	// text[] column, nothing here needs one.
	// Detail-sheet labels ("1.0.0", "12+", "Casino", "Interactive
	// elements") — short free text, so a single shared cap is enough.
	detailLabelMaxLen = 60
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type Service struct {
	repo      *Repository
	testLinks *TestLinks // nil: TestLink unavailable (see testlink.go)
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, orgID string) ([]Pwa, error) {
	return s.repo.List(ctx, orgID)
}

func (s *Service) Get(ctx context.Context, orgID, id string) (Pwa, error) {
	return s.repo.GetByID(ctx, orgID, id)
}

// blankDateToNil turns an empty updatedDate ("" — what a cleared date
// input sends) into nil, so it lands as SQL NULL rather than failing
// the ”::date cast.
func blankDateToNil(d *string) *string {
	if d != nil && strings.TrimSpace(*d) == "" {
		return nil
	}
	return d
}

func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (Pwa, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.ShortName = strings.TrimSpace(in.ShortName)
	in.StartURL = strings.TrimSpace(in.StartURL)
	in.Developer = strings.TrimSpace(in.Developer)
	in.Description = strings.TrimSpace(in.Description)
	in.DownloadsLabel = strings.TrimSpace(in.DownloadsLabel)
	in.WhatsNew = strings.TrimSpace(in.WhatsNew)
	in.Version = strings.TrimSpace(in.Version)
	in.AgeRating = strings.TrimSpace(in.AgeRating)
	in.Category = strings.TrimSpace(in.Category)
	in.InteractiveElements = strings.TrimSpace(in.InteractiveElements)
	in.UpdatedDate = blankDateToNil(in.UpdatedDate)
	in.Screenshots = trimAll(in.Screenshots)
	if in.Screenshots == nil {
		in.Screenshots = []string{}
	}

	fields := validate(in.Name, in.ShortName, in.ThemeColor, in.BackgroundColor, in.IconURL, in.StartURL)
	for k, v := range validateListingFields(listingFields{
		developer: in.Developer, description: in.Description, downloadsLabel: in.DownloadsLabel, whatsNew: in.WhatsNew,
		screenshots: in.Screenshots,
		rating:      in.Rating, ratingCount: in.RatingCount, sizeMB: in.SizeMB,
		ratingStars5: in.RatingStars5, ratingStars4: in.RatingStars4, ratingStars3: in.RatingStars3,
		ratingStars2: in.RatingStars2, ratingStars1: in.RatingStars1,
		version: in.Version, ageRating: in.AgeRating, category: in.Category, interactiveElements: in.InteractiveElements,
		updatedDate: in.UpdatedDate,
	}) {
		fields[k] = v
	}
	if len(fields) > 0 {
		return Pwa{}, apierror.Validation("invalid pwa", fields)
	}
	return s.repo.Create(ctx, idgen.New(), orgID, in)
}

func (s *Service) Update(ctx context.Context, orgID, id string, in UpdateInput) (Pwa, error) {
	fields := map[string]string{}

	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		in.Name = &trimmed
		if len(trimmed) < nameMinLen || len(trimmed) > nameMaxLen {
			fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
		}
	}
	if in.ShortName != nil {
		trimmed := strings.TrimSpace(*in.ShortName)
		in.ShortName = &trimmed
		if trimmed == "" || len(trimmed) > shortNameMaxLen {
			fields["shortName"] = fmt.Sprintf("required, up to %d characters", shortNameMaxLen)
		}
	}
	if in.ThemeColor != nil && !hexColor.MatchString(*in.ThemeColor) {
		fields["themeColor"] = "enter a hex color like #16a34a"
	}
	if in.BackgroundColor != nil && !hexColor.MatchString(*in.BackgroundColor) {
		fields["backgroundColor"] = "enter a hex color like #16a34a"
	}
	if in.IconURL != nil && !isValidURL(*in.IconURL) {
		fields["iconUrl"] = "enter a valid icon URL"
	}
	if in.StartURL != nil {
		trimmed := strings.TrimSpace(*in.StartURL)
		in.StartURL = &trimmed
		if trimmed == "" {
			fields["startUrl"] = "required"
		}
	}
	if in.Developer != nil {
		trimmed := strings.TrimSpace(*in.Developer)
		in.Developer = &trimmed
	}
	if in.Description != nil {
		trimmed := strings.TrimSpace(*in.Description)
		in.Description = &trimmed
	}
	if in.DownloadsLabel != nil {
		trimmed := strings.TrimSpace(*in.DownloadsLabel)
		in.DownloadsLabel = &trimmed
	}
	if in.WhatsNew != nil {
		trimmed := strings.TrimSpace(*in.WhatsNew)
		in.WhatsNew = &trimmed
	}
	if in.Screenshots != nil {
		trimmed := trimAll(*in.Screenshots)
		in.Screenshots = &trimmed
	}
	for _, f := range []**string{&in.Version, &in.AgeRating, &in.Category, &in.InteractiveElements} {
		if *f != nil {
			trimmed := strings.TrimSpace(**f)
			*f = &trimmed
		}
	}
	in.UpdatedDate = blankDateToNil(in.UpdatedDate)
	developer := ""
	if in.Developer != nil {
		developer = *in.Developer
	}
	description := ""
	if in.Description != nil {
		description = *in.Description
	}
	downloadsLabel := ""
	if in.DownloadsLabel != nil {
		downloadsLabel = *in.DownloadsLabel
	}
	whatsNew := ""
	if in.WhatsNew != nil {
		whatsNew = *in.WhatsNew
	}
	var screenshots []string
	if in.Screenshots != nil {
		screenshots = *in.Screenshots
	}
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	for k, v := range validateListingFields(listingFields{
		developer: developer, description: description, downloadsLabel: downloadsLabel, whatsNew: whatsNew,
		screenshots: screenshots,
		rating:      in.Rating, ratingCount: in.RatingCount, sizeMB: in.SizeMB,
		ratingStars5: in.RatingStars5, ratingStars4: in.RatingStars4, ratingStars3: in.RatingStars3,
		ratingStars2: in.RatingStars2, ratingStars1: in.RatingStars1,
		version: deref(in.Version), ageRating: deref(in.AgeRating), category: deref(in.Category),
		interactiveElements: deref(in.InteractiveElements), updatedDate: in.UpdatedDate,
	}) {
		fields[k] = v
	}
	if in.Status != nil && !in.Status.Valid() {
		fields["status"] = "must be one of active, paused, archived"
	}
	if len(fields) > 0 {
		return Pwa{}, apierror.Validation("invalid pwa", fields)
	}

	return s.repo.Update(ctx, orgID, id, in)
}

func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	return s.repo.Delete(ctx, orgID, id)
}

// Duplicate keeps status as-is, same reasoning as
// landing.Service.Duplicate/network.Service.Duplicate: no draft-equivalent
// status exists to reset to.
func (s *Service) Duplicate(ctx context.Context, orgID, id string) (Pwa, error) {
	source, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Pwa{}, err
	}

	created, err := s.repo.Create(ctx, idgen.New(), orgID, CreateInput{
		Name:                source.Name + " (Copy)",
		ShortName:           source.ShortName,
		ThemeColor:          source.ThemeColor,
		BackgroundColor:     source.BackgroundColor,
		ThemeAuto:           source.ThemeAuto,
		IconURL:             source.IconURL,
		StartURL:            source.StartURL,
		BounceInAppWebview:  source.BounceInAppWebview,
		Developer:           source.Developer,
		Description:         source.Description,
		Rating:              source.Rating,
		RatingCount:         source.RatingCount,
		DownloadsLabel:      source.DownloadsLabel,
		SizeMB:              source.SizeMB,
		Screenshots:         source.Screenshots,
		WhatsNew:            source.WhatsNew,
		RatingStars5:        source.RatingStars5,
		RatingStars4:        source.RatingStars4,
		RatingStars3:        source.RatingStars3,
		RatingStars2:        source.RatingStars2,
		RatingStars1:        source.RatingStars1,
		Version:             source.Version,
		AgeRating:           source.AgeRating,
		Category:            source.Category,
		InteractiveElements: source.InteractiveElements,
		ContainsAds:         source.ContainsAds,
		InAppPurchases:      source.InAppPurchases,
		UpdatedDate:         source.UpdatedDate,
	})
	if err != nil {
		return Pwa{}, err
	}
	if source.Status != StatusActive {
		status := source.Status
		return s.repo.Update(ctx, orgID, created.ID, UpdateInput{Status: &status})
	}
	return created, nil
}

func (s *Service) Pause(ctx context.Context, orgID, id string) (Pwa, error) {
	p, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Pwa{}, err
	}
	switch p.Status {
	case StatusPaused:
		return p, nil
	case StatusActive:
		status := StatusPaused
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Pwa{}, apierror.Conflict(fmt.Sprintf("cannot pause a pwa with status %q", p.Status))
	}
}

func (s *Service) Activate(ctx context.Context, orgID, id string) (Pwa, error) {
	p, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Pwa{}, err
	}
	switch p.Status {
	case StatusActive:
		return p, nil
	case StatusPaused:
		status := StatusActive
		return s.repo.Update(ctx, orgID, id, UpdateInput{Status: &status})
	default:
		return Pwa{}, apierror.Conflict(fmt.Sprintf("cannot activate a pwa with status %q", p.Status))
	}
}

func validate(name, shortName, themeColor, backgroundColor, iconURL, startURL string) map[string]string {
	fields := map[string]string{}
	if len(name) < nameMinLen || len(name) > nameMaxLen {
		fields["name"] = fmt.Sprintf("must be %d-%d characters", nameMinLen, nameMaxLen)
	}
	if shortName == "" || len(shortName) > shortNameMaxLen {
		fields["shortName"] = fmt.Sprintf("required, up to %d characters", shortNameMaxLen)
	}
	if !hexColor.MatchString(themeColor) {
		fields["themeColor"] = "enter a hex color like #16a34a"
	}
	if !hexColor.MatchString(backgroundColor) {
		fields["backgroundColor"] = "enter a hex color like #16a34a"
	}
	if !isValidURL(iconURL) {
		fields["iconUrl"] = "enter a valid icon URL"
	}
	if startURL == "" {
		fields["startUrl"] = "required"
	}
	return fields
}

func isValidURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// listingFields is validateListingFields' input — a struct rather than a
// long positional-argument list now that the store-listing preview
// spans this many optional fields.
type listingFields struct {
	developer      string
	description    string
	downloadsLabel string
	whatsNew       string
	screenshots    []string
	rating         *float64
	ratingCount    *int
	sizeMB         *float64
	ratingStars5   *float64
	ratingStars4   *float64
	ratingStars3   *float64
	ratingStars2   *float64
	ratingStars1   *float64
	// Detail-sheet labels, all optional free text (see model.go).
	version             string
	ageRating           string
	category            string
	interactiveElements string
	updatedDate         *string
}

// validateListingFields covers the store-listing preview's own fields —
// all optional, so an empty/nil value never fails here; only a value
// that's actually out of range does.
func validateListingFields(f listingFields) map[string]string {
	fields := map[string]string{}
	if len(f.developer) > developerMaxLen {
		fields["developer"] = fmt.Sprintf("up to %d characters", developerMaxLen)
	}
	if len(f.description) > descriptionMaxLen {
		fields["description"] = fmt.Sprintf("up to %d characters", descriptionMaxLen)
	}
	if len(f.downloadsLabel) > downloadsLabelMax {
		fields["downloadsLabel"] = fmt.Sprintf("up to %d characters", downloadsLabelMax)
	}
	if len(f.whatsNew) > whatsNewMaxLen {
		fields["whatsNew"] = fmt.Sprintf("up to %d characters", whatsNewMaxLen)
	}
	for _, u := range f.screenshots {
		if !isValidURL(u) {
			fields["screenshots"] = "each screenshot must be a valid URL"
			break
		}
	}
	if f.rating != nil && (*f.rating < 0 || *f.rating > 5) {
		fields["rating"] = "must be between 0 and 5"
	}
	if f.ratingCount != nil && *f.ratingCount < 0 {
		fields["ratingCount"] = "must not be negative"
	}
	if f.sizeMB != nil && *f.sizeMB < 0 {
		fields["sizeMb"] = "must not be negative"
	}
	for key, v := range map[string]*float64{
		"ratingStars5": f.ratingStars5, "ratingStars4": f.ratingStars4, "ratingStars3": f.ratingStars3,
		"ratingStars2": f.ratingStars2, "ratingStars1": f.ratingStars1,
	} {
		if v != nil && (*v < 0 || *v > 100) {
			fields[key] = "must be between 0 and 100"
		}
	}
	for key, v := range map[string]string{
		"version": f.version, "ageRating": f.ageRating, "category": f.category,
		"interactiveElements": f.interactiveElements,
	} {
		if len(v) > detailLabelMaxLen {
			fields[key] = fmt.Sprintf("up to %d characters", detailLabelMaxLen)
		}
	}
	// updatedDate crosses the API as a plain YYYY-MM-DD string (the
	// listing shows a date, not a timestamp) — reject anything the
	// date column would choke on, rather than letting Postgres raise it.
	if f.updatedDate != nil && *f.updatedDate != "" {
		if _, err := time.Parse("2006-01-02", *f.updatedDate); err != nil {
			fields["updatedDate"] = "must be a date like 2026-08-21"
		}
	}
	return fields
}

// trimAll trims whitespace from every element and drops empty ones —
// applied to Screenshots so a stray blank row from the frontend's
// add/remove list editor never persists as an empty-string URL.
func trimAll(items []string) []string {
	if items == nil {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, s := range items {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}
