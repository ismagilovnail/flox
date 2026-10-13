package push

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
)

type Service struct {
	repo *Repository
	// configured: the deployment has a VAPID key pair — without one
	// nothing can be sent, so no broadcast is created.
	configured bool
}

func NewService(repo *Repository, configured bool) *Service {
	return &Service{repo: repo, configured: configured}
}

func (s *Service) Configured() bool { return s.configured }

func (s *Service) Summary(ctx context.Context, orgID string) (Summary, error) {
	return s.repo.Summary(ctx, orgID)
}

func (s *Service) ListSubscriptions(ctx context.Context, orgID string, f SubscriptionFilter, page pagination.Page) ([]Subscription, int, error) {
	return s.repo.ListSubscriptions(ctx, orgID, f, page)
}

func (s *Service) ListBroadcasts(ctx context.Context, orgID string, page pagination.Page) ([]Broadcast, int, error) {
	return s.repo.ListBroadcasts(ctx, orgID, page)
}

func (s *Service) GetBroadcast(ctx context.Context, orgID, id string) (Broadcast, error) {
	return s.repo.GetBroadcast(ctx, orgID, id)
}

func (s *Service) CountAudience(ctx context.Context, orgID string, f Filters) (int, error) {
	if fields := validateFilters(f); len(fields) > 0 {
		return 0, apierror.Validation("invalid filters", fields)
	}
	return s.repo.CountAudience(ctx, orgID, f)
}

// CreateBroadcast validates and sends a broadcast now. An audience of no
// one is refused rather than stored as an empty send.
func (s *Service) CreateBroadcast(ctx context.Context, orgID, userID string, in CreateBroadcastInput) (Broadcast, error) {
	if !s.configured {
		return Broadcast{}, apierror.Unavailable("push is not configured on this deployment (PUSH_VAPID_* keys)")
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	in.URL = strings.TrimSpace(in.URL)
	if fields := validateBroadcast(in); len(fields) > 0 {
		return Broadcast{}, apierror.Validation("invalid broadcast", fields)
	}
	n, err := s.repo.CountAudience(ctx, orgID, in.Filters)
	if err != nil {
		return Broadcast{}, err
	}
	if n == 0 {
		return Broadcast{}, apierror.Validation("no subscribers match these filters", map[string]string{"filters": "no subscribers match"})
	}
	return s.repo.CreateBroadcast(ctx, orgID, userID, in)
}

func validateBroadcast(in CreateBroadcastInput) map[string]string {
	fields := validateFilters(in.Filters)
	if n := utf8.RuneCountInString(in.Name); n < 2 || n > 100 {
		fields["name"] = "2–100 characters"
	}
	if n := utf8.RuneCountInString(in.Title); n == 0 || n > MaxTitle {
		fields["title"] = "1–80 characters"
	}
	if utf8.RuneCountInString(in.Body) > MaxBody {
		fields["body"] = "at most 240 characters"
	}
	for field, u := range map[string]string{"iconUrl": in.IconURL, "imageUrl": in.ImageURL} {
		if u != "" && !isHTTPSURL(u) {
			fields[field] = "an https URL"
		}
	}
	switch in.Target {
	case TargetDestination:
		if in.URL != "" {
			fields["url"] = "only with target url"
		}
	case TargetURL:
		if len(in.URL) > MaxURL || !isHTTPURL(in.URL) {
			fields["url"] = "an http(s) URL"
		}
	default:
		fields["target"] = "destination or url"
	}
	return fields
}

var (
	idPattern      = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)
	countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)
)

func validateFilters(f Filters) map[string]string {
	fields := map[string]string{}
	for field, ids := range map[string][]string{"pwaIds": f.PwaIDs, "campaignIds": f.CampaignIDs} {
		for _, id := range ids {
			if !idPattern.MatchString(id) {
				fields[field] = "unknown id"
			}
		}
	}
	for _, c := range f.Countries {
		if !countryPattern.MatchString(c) {
			fields["countries"] = "ISO 3166-1 alpha-2 codes"
		}
	}
	return fields
}

func isHTTPURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func isHTTPSURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && u.Scheme == "https" && u.Host != ""
}
