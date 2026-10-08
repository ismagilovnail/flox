package alert

import (
	"context"
	"math"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// notificationLimit is how many notifications the bell lists.
const notificationLimit = 50

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

func (s *Service) ListRules(ctx context.Context, orgID string) ([]Rule, error) {
	return s.repo.ListRules(ctx, orgID)
}

func (s *Service) CreateRule(ctx context.Context, orgID string, in RuleInput) (Rule, error) {
	in, err := s.validate(ctx, orgID, in)
	if err != nil {
		return Rule{}, err
	}
	return s.repo.CreateRule(ctx, idgen.New(), orgID, in)
}

func (s *Service) UpdateRule(ctx context.Context, orgID, id string, in RuleInput) (Rule, error) {
	in, err := s.validate(ctx, orgID, in)
	if err != nil {
		return Rule{}, err
	}
	return s.repo.UpdateRule(ctx, orgID, id, in)
}

func (s *Service) DeleteRule(ctx context.Context, orgID, id string) error {
	return s.repo.DeleteRule(ctx, orgID, id)
}

func (s *Service) Notifications(ctx context.Context, orgID, userID string) (NotificationList, error) {
	return s.repo.ListNotifications(ctx, orgID, userID, notificationLimit)
}

func (s *Service) MarkAllRead(ctx context.Context, orgID, userID string) error {
	return s.repo.MarkAllRead(ctx, orgID, userID)
}

// validate checks a rule against its kind and clears the fields the kind
// does not use, so a stored rule never carries a threshold nothing reads.
func (s *Service) validate(ctx context.Context, orgID string, in RuleInput) (RuleInput, error) {
	if !in.Kind.Valid() {
		return in, apierror.Validation("invalid alert rule", map[string]string{"kind": "unknown kind"})
	}
	errs := map[string]string{}
	finite := func(p *float64) bool { return p == nil || (!math.IsNaN(*p) && !math.IsInf(*p, 0)) }
	if !finite(in.Threshold) || !finite(in.MinSpend) {
		return in, apierror.Validation("invalid alert rule", map[string]string{"threshold": "must be a number"})
	}

	switch in.Kind {
	case KindSpendOver:
		if in.Threshold == nil || *in.Threshold <= 0 {
			errs["threshold"] = "spend threshold must be greater than 0"
		}
		in.MinSpend = nil
	case KindROIBelow:
		if in.Threshold == nil {
			errs["threshold"] = "ROI threshold is required"
		} else if *in.Threshold < -100 || *in.Threshold > 100000 {
			errs["threshold"] = "ROI threshold must be between -100 and 100000"
		}
		if in.MinSpend != nil && *in.MinSpend < 0 {
			errs["minSpend"] = "must not be negative"
		}
	case KindSSLExpiring:
		if in.Threshold == nil || *in.Threshold < 1 || *in.Threshold > 90 || *in.Threshold != math.Trunc(*in.Threshold) {
			errs["threshold"] = "days must be a whole number from 1 to 90"
		}
		in.MinSpend = nil
	case KindOfferCapped, KindOfferPaused:
		in.Threshold = nil
		in.MinSpend = nil
	}

	if !in.Kind.perCampaign() {
		in.CampaignID = nil
	} else if in.CampaignID != nil {
		if *in.CampaignID == "" {
			in.CampaignID = nil
		} else {
			ok, err := s.repo.CampaignExists(ctx, orgID, *in.CampaignID)
			if err != nil {
				return in, err
			}
			if !ok {
				errs["campaignId"] = "campaign not found"
			}
		}
	}
	if len(errs) > 0 {
		return in, apierror.Validation("invalid alert rule", errs)
	}
	return in, nil
}
