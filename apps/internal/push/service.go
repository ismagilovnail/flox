package push

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
)

type Service struct {
	repo *Repository
	// configured: the deployment has a VAPID key pair — without one
	// nothing can be sent, so no broadcast is created.
	configured bool
	now        func() time.Time
}

func NewService(repo *Repository, configured bool) *Service {
	return &Service{repo: repo, configured: configured, now: time.Now}
}

// maxStatsDays bounds a push metrics range, like the analytics reports.
const maxStatsDays = 366

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

// CreateBroadcast validates and sends a broadcast now, schedules it, or
// starts a trigger. An audience of no one is refused for a send now rather
// than stored as an empty send; a scheduled or triggered broadcast's
// audience is counted when it goes out, so it may be empty today.
func (s *Service) CreateBroadcast(ctx context.Context, orgID, userID string, in CreateBroadcastInput) (Broadcast, error) {
	if !s.configured {
		return Broadcast{}, apierror.Unavailable("push is not configured on this deployment (PUSH_VAPID_* keys)")
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	in.URL = strings.TrimSpace(in.URL)
	fields := validateBroadcast(in)
	if in.Trigger != nil {
		if in.SendAt != nil {
			fields["sendAt"] = "not with a trigger"
		}
		for k, v := range validateTrigger(in.Trigger) {
			fields[k] = v
		}
	}
	if in.SendAt != nil {
		now := s.now()
		switch {
		case in.SendAt.Before(now.Add(MinScheduleAhead)):
			fields["sendAt"] = "at least a minute ahead; send now instead"
		case in.SendAt.After(now.Add(MaxScheduleAhead)):
			fields["sendAt"] = "at most 30 days ahead"
		}
	}
	if len(fields) > 0 {
		return Broadcast{}, apierror.Validation("invalid broadcast", fields)
	}
	if in.SendAt != nil || in.Trigger != nil {
		return s.repo.CreateBroadcast(ctx, orgID, userID, in)
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

// CancelBroadcast calls off a scheduled broadcast before it is sent.
func (s *Service) CancelBroadcast(ctx context.Context, orgID, id string) (Broadcast, error) {
	return s.repo.CancelBroadcast(ctx, orgID, id)
}

// SetTriggerActive pauses (active false) or resumes a triggered broadcast.
func (s *Service) SetTriggerActive(ctx context.Context, orgID, id string, active bool) (Broadcast, error) {
	return s.repo.SetTriggerActive(ctx, orgID, id, active)
}

// Stats is push metrics for broadcasts sent between from and to (days,
// inclusive) in timezone (empty: UTC), grouped by group.
func (s *Service) Stats(ctx context.Context, orgID string, from, to time.Time, timezone string, group StatsGroup) ([]StatsRow, StatsCounts, error) {
	if timezone == "" {
		timezone = "UTC"
	}
	fields := map[string]string{}
	if !group.valid() {
		fields["groupBy"] = "day, broadcast, pwa, campaign or country"
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		fields["timezone"] = "an IANA time zone"
	}
	if to.Before(from) {
		fields["to"] = "on or after from"
	} else if to.Sub(from) > (maxStatsDays-1)*24*time.Hour {
		fields["to"] = fmt.Sprintf("at most %d days after from", maxStatsDays)
	}
	if len(fields) > 0 {
		return nil, StatsCounts{}, apierror.Validation("invalid stats query", fields)
	}
	return s.repo.Stats(ctx, orgID, from, to, timezone, group)
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

func validateTrigger(t *Trigger) map[string]string {
	fields := map[string]string{}
	if t.DelayMinutes < MinTriggerDelay || t.DelayMinutes > MaxTriggerDelay {
		fields["trigger.delayMinutes"] = fmt.Sprintf("%d minutes to 30 days", MinTriggerDelay)
	}
	switch t.Condition {
	case "", ConditionNone:
		t.Condition = ConditionNone
		if len(t.Statuses) > 0 {
			fields["trigger.statuses"] = "only with a condition"
		}
	case ConditionWith, ConditionWithout:
		if len(t.Statuses) == 0 {
			fields["trigger.statuses"] = "at least one status"
		}
		for _, st := range t.Statuses {
			if !slices.Contains(ConversionStatuses, st) {
				fields["trigger.statuses"] = strings.Join(ConversionStatuses, ", ")
			}
		}
	default:
		fields["trigger.condition"] = "none, with or without"
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
