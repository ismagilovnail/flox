package billing

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

// Store is the Postgres side Service reads (*Repository).
type Store interface {
	Plans(ctx context.Context) ([]Plan, error)
	Subscription(ctx context.Context, orgID string) (Subscription, error)
	Counts(ctx context.Context, orgID string) (Counts, error)
}

// UsageReader counts an organization's clicks in a period (*CHUsage).
type UsageReader interface {
	Clicks(ctx context.Context, orgID string, p Period) (int64, error)
}

// Gate is what the packages that create limited things call before they
// do (campaign, streamset, domain, auth's invite). A nil Gate allows
// everything — every service works without one (tests, tools).
type Gate interface {
	Allow(ctx context.Context, orgID string, r Resource) error
}

type Service struct {
	store  Store
	usage  UsageReader // nil: clicks are unknown and never block
	now    func() time.Time
	logger *slog.Logger
	canPay bool
	// shortenAfter: RETENTION_SHORTEN_AFTER_DAYS, for the date a shorter
	// raw-click retention applies (internal/retention.Plans).
	shortenAfter time.Duration
}

// WithRetentionShortenAfter sets how long a shorter raw-click retention
// waits before it applies.
func (s *Service) WithRetentionShortenAfter(d time.Duration) *Service {
	s.shortenAfter = d
	return s
}

// UnappliedRetentionMonths is what an organization's raw rows mean before
// internal/retention.Plans first ran for it: the column default.
const UnappliedRetentionMonths = 24

// WithPayments marks plans as payable online (Status.CanPay).
func (s *Service) WithPayments(available bool) *Service {
	s.canPay = available
	return s
}

func NewService(store Store, usage UsageReader, logger *slog.Logger) *Service {
	return &Service{store: store, usage: usage, now: time.Now, logger: logger}
}

// WithClock replaces time.Now, for tests.
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

func (s *Service) Plans(ctx context.Context) ([]Plan, error) {
	return s.store.Plans(ctx)
}

func (s *Service) Status(ctx context.Context, orgID string) (Status, error) {
	if orgID == "" {
		return Status{}, apierror.Validation("missing organization", nil)
	}
	sub, err := s.store.Subscription(ctx, orgID)
	if err != nil {
		return Status{}, err
	}
	plan := sub.Plan
	counts, err := s.store.Counts(ctx, orgID)
	if err != nil {
		return Status{}, err
	}
	now := s.now()
	period := PeriodAt(now)
	clicks := s.clicks(ctx, orgID, period)

	st := Status{Plan: plan, Since: sub.Since, Usage: Usage{Clicks: clicks, Counts: counts}, Level: LevelFor(plan, clicks), CanPay: s.canPay}
	if sub.PaidUntil != nil {
		grace := sub.PaidUntil.Add(GracePeriod)
		st.PaidUntil, st.GraceUntil = sub.PaidUntil, &grace
		st.Renew = !now.Before(sub.PaidUntil.Add(-RemindBefore))
	}
	st.RawRetentionMonths, st.RawRetentionShortens = rawRetention(sub, now, s.shortenAfter)
	st.Period.From = period.From.Format(time.DateOnly)
	st.Period.To = period.To.AddDate(0, 0, -1).Format(time.DateOnly)
	return st, nil
}

// clicks is this period's clicks, nil when they cannot be read: a failed
// read is logged and treated as unknown, never as zero and never as a
// reason to refuse — ClickHouse being down must not stop a client working.
func (s *Service) clicks(ctx context.Context, orgID string, p Period) *int64 {
	if s.usage == nil {
		return nil
	}
	n, err := s.usage.Clicks(ctx, orgID, p)
	if err != nil {
		s.logger.Warn("billing: reading clicks failed", "organization_id", orgID, "error", err)
		return nil
	}
	return &n
}

// LevelFor is how far clicks are into plan's monthly clicks.
func LevelFor(plan Plan, clicks *int64) Level {
	if plan.MonthlyClicks == nil {
		return LevelOK
	}
	if clicks == nil {
		return LevelUnknown
	}
	used := float64(*clicks) / float64(*plan.MonthlyClicks)
	switch {
	case used >= BlockAt:
		return LevelBlocked
	case used >= 1:
		return LevelExceeded
	case used >= WarnAt:
		return LevelWarning
	}
	return LevelOK
}

// Allow refuses, with 402 plan_limit, creating one more r when the plan's
// count limit for it is reached, or — for campaigns and stream sets —
// when this month's clicks are past BlockAt. Counts are checked before
// the insert, not inside it: two creates at once can both pass at the
// last free slot. For a soft limit that is accepted.
func (s *Service) Allow(ctx context.Context, orgID string, r Resource) error {
	sub, err := s.store.Subscription(ctx, orgID)
	if err != nil {
		return err
	}
	plan := sub.Plan
	counts, err := s.store.Counts(ctx, orgID)
	if err != nil {
		return err
	}

	var limit *int
	var have int
	switch r {
	case ResourceCampaign:
		limit, have = plan.MaxCampaigns, counts.Campaigns
	case ResourceDomain:
		limit, have = plan.MaxDomains, counts.Domains
	case ResourceMember:
		limit, have = plan.MaxMembers, counts.Members
	case ResourceStreamSet:
	default:
		return fmt.Errorf("billing: unknown resource %q", r)
	}
	if limit != nil && have >= *limit {
		return apierror.PlanLimit(fmt.Sprintf("The %s plan allows %d %s. Archive or delete one, or change the plan.", plan.Name, *limit, plural(r)))
	}

	if (r == ResourceCampaign || r == ResourceStreamSet) && plan.MonthlyClicks != nil {
		if LevelFor(plan, s.clicks(ctx, orgID, PeriodAt(s.now()))) == LevelBlocked {
			return apierror.PlanLimit(fmt.Sprintf("This month's clicks are over the %s plan's %d. Tracking goes on; new campaigns and stream sets wait for a plan change or the next month.", plan.Name, *plan.MonthlyClicks))
		}
	}
	return nil
}

func plural(r Resource) string {
	switch r {
	case ResourceCampaign:
		return "campaigns"
	case ResourceDomain:
		return "domains"
	case ResourceMember:
		return "members"
	}
	return string(r) + "s"
}

// rawRetention: the months the organization's raw clicks are kept now, and
// the shorter period its plan brings and when, if one is waiting.
func rawRetention(sub Subscription, now time.Time, shortenAfter time.Duration) (int, *RetentionChange) {
	months, target, since := UnappliedRetentionMonths, sub.Plan.RawRetentionMonths, now
	if sub.Retention != nil {
		months, target, since = sub.Retention.Months, sub.Retention.Target, sub.Retention.TargetSince
	}
	if target >= months {
		return months, nil
	}
	return months, &RetentionChange{Months: target, At: since.Add(shortenAfter)}
}
