package alert

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"
	_ "time/tzdata" // zone names resolve without a system tzdata (distroless images)

	"github.com/ismagilovnail/flox/apps/internal/analytics"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/offercap"
)

const (
	// Interval is how often the worker evaluates rules and delivers to
	// Slack. Spend arrives hourly from the ad-platform sync at best, so a
	// tighter loop would only add ClickHouse load.
	Interval = 2 * time.Minute

	// pausedLookback bounds how far back a pause still fires: a rule
	// switched on (or a worker that was down) does not dig up old pauses.
	pausedLookback = 24 * time.Hour

	slackBatch       = 50
	slackLease       = 2 * time.Minute
	slackMaxAttempts = 5
)

// CampaignReporter is the campaign report spend and ROI are read from —
// analytics.Service in production, the same numbers the Analytics page
// shows.
type CampaignReporter interface {
	Report(ctx context.Context, organizationID string, in analytics.ReportInput) ([]analytics.ReportRow, error)
}

// CapReader returns every capped offer's count of conversions today
// (offercap.Statuses in production).
type CapReader func(ctx context.Context, now time.Time) ([]offercap.Status, error)

// ErrSlackNotConnected is what a SlackSender returns when the organization
// has no Slack integration.
var ErrSlackNotConnected = errors.New("slack is not connected")

// SlackSender posts text to an organization's Slack integration.
type SlackSender interface {
	PostSlack(ctx context.Context, organizationID, text string) error
}

type Evaluator struct {
	Repo    *Repository
	Reports CampaignReporter
	Caps    CapReader
	// Slack may be nil: Slack notifications then fail with a reason.
	Slack  SlackSender
	Logger *slog.Logger
}

// Evaluate checks every enabled rule as of now and records what fired. It
// returns how many new notifications it wrote. One organization's failure
// is logged and does not stop the others.
func (e *Evaluator) Evaluate(ctx context.Context, now time.Time) (int, error) {
	rules, err := e.Repo.enabledRules(ctx)
	if err != nil {
		return 0, err
	}
	byOrg := map[string][]orgRule{}
	var orgs []string
	needCaps := false
	for _, r := range rules {
		if _, ok := byOrg[r.OrganizationID]; !ok {
			orgs = append(orgs, r.OrganizationID)
		}
		byOrg[r.OrganizationID] = append(byOrg[r.OrganizationID], r)
		needCaps = needCaps || r.Kind == KindOfferCapped
	}

	var caps []offercap.Status
	if needCaps && e.Caps != nil {
		caps, err = e.Caps(ctx, now)
		if err != nil {
			// Offer caps are skipped this round; the other kinds still run.
			e.Logger.Warn("alerts: reading offer caps", "error", err)
			caps = nil
		}
	}

	fired := 0
	for _, org := range orgs {
		n, err := e.evaluateOrg(ctx, now, org, byOrg[org], caps)
		fired += n
		if err != nil {
			if ctx.Err() != nil {
				return fired, ctx.Err()
			}
			e.Logger.Error("alerts: evaluating organization", "organization_id", org, "error", err)
		}
	}
	return fired, nil
}

func (e *Evaluator) evaluateOrg(ctx context.Context, now time.Time, orgID string, rules []orgRule, caps []offercap.Status) (int, error) {
	loc, err := time.LoadLocation(rules[0].Timezone)
	if err != nil || rules[0].Timezone == "" {
		loc = time.UTC
	}
	local := now.In(loc)
	day := local.Format(time.DateOnly)

	fired := 0
	fire := func(rule Rule, key string, params map[string]any) error {
		isNew, err := e.Repo.fire(ctx, idgen.New(), orgID, rule, key, params)
		if isNew {
			fired++
		}
		return err
	}

	var rows []analytics.ReportRow
	rowsLoaded := false
	var campaignNames map[string]string

	for _, r := range rules {
		switch r.Kind {
		case KindSpendOver, KindROIBelow:
			if !rowsLoaded {
				rows, err = e.Reports.Report(ctx, orgID, analytics.ReportInput{
					Dims: []chstore.ReportDimension{chstore.DimCampaign}, From: local, To: local, Timezone: loc.String(),
					KeepUSD: true, // thresholds are USD
				})
				if err != nil {
					return fired, fmt.Errorf("campaign report: %w", err)
				}
				ids := make([]string, 0, len(rows))
				for _, row := range rows {
					ids = append(ids, row.DimValues[chstore.DimCampaign])
				}
				if campaignNames, err = e.Repo.names(ctx, "campaigns", orgID, ids); err != nil {
					return fired, err
				}
				rowsLoaded = true
			}
			for _, row := range rows {
				campaignID := row.DimValues[chstore.DimCampaign]
				if r.CampaignID != nil && *r.CampaignID != campaignID {
					continue
				}
				params, ok := campaignCondition(r.Rule, row.Metrics)
				if !ok {
					continue
				}
				params["campaignId"] = campaignID
				params["campaignName"] = campaignNames[campaignID]
				params["date"] = day
				if err := fire(r.Rule, fmt.Sprintf("%s:%s:%s:%s", r.Kind, r.ID, campaignID, day), params); err != nil {
					return fired, err
				}
			}

		case KindOfferCapped:
			var reached []offercap.Status
			var ids []string
			for _, s := range caps {
				if s.OrganizationID == orgID && s.Reached() {
					reached = append(reached, s)
					ids = append(ids, s.OfferID)
				}
			}
			names, err := e.Repo.names(ctx, "offers", orgID, ids)
			if err != nil {
				return fired, err
			}
			for _, s := range reached {
				// The cap's own day, in the organization's timezone.
				capDay := s.Since.In(loc).Format(time.DateOnly)
				params := map[string]any{"offerId": s.OfferID, "offerName": names[s.OfferID], "cap": s.Cap, "count": s.Count, "date": capDay}
				if err := fire(r.Rule, fmt.Sprintf("offer_capped:%s:%s", s.OfferID, capDay), params); err != nil {
					return fired, err
				}
			}

		case KindOfferPaused:
			since := now.Add(-pausedLookback)
			if r.CreatedAt.After(since) {
				since = r.CreatedAt
			}
			paused, err := e.Repo.pausedOffers(ctx, orgID, since)
			if err != nil {
				return fired, err
			}
			for _, p := range paused {
				params := map[string]any{"offerId": p.ID, "offerName": p.Name, "at": p.ChangedAt.UTC()}
				if err := fire(r.Rule, fmt.Sprintf("offer_paused:%s:%d", p.ID, p.ChangedAt.UnixMilli()), params); err != nil {
					return fired, err
				}
			}

		case KindSSLExpiring:
			if r.Threshold == nil {
				continue
			}
			days := int(*r.Threshold)
			domains, err := e.Repo.expiringDomains(ctx, orgID, now.Add(time.Duration(days)*24*time.Hour))
			if err != nil {
				return fired, err
			}
			for _, d := range domains {
				left := int(math.Ceil(d.ExpiresAt.Sub(now).Hours() / 24))
				if left < 0 {
					left = 0
				}
				params := map[string]any{"domainId": d.ID, "domain": d.Domain, "expiresAt": d.ExpiresAt.UTC(), "daysLeft": left, "expired": !d.ExpiresAt.After(now), "threshold": days}
				key := fmt.Sprintf("ssl_expiring:%s:%d:%d", d.ID, d.ExpiresAt.Unix(), days)
				if err := fire(r.Rule, key, params); err != nil {
					return fired, err
				}
			}
		}
	}
	return fired, nil
}

// campaignCondition reports whether one campaign's numbers today trip a
// spend or ROI rule, with the values for the message. No spend on file
// means neither can trip: ROI without cost does not exist (CLAUDE.md #6).
func campaignCondition(r Rule, m analytics.MetricValues) (map[string]any, bool) {
	if m.Cost == nil || r.Threshold == nil {
		return nil, false
	}
	spend := *m.Cost
	switch r.Kind {
	case KindSpendOver:
		if spend > *r.Threshold {
			return map[string]any{"spend": round2(spend), "threshold": *r.Threshold}, true
		}
	case KindROIBelow:
		if m.ROI == nil || spend <= 0 {
			return nil, false
		}
		if r.MinSpend != nil && spend < *r.MinSpend {
			return nil, false
		}
		if *m.ROI < *r.Threshold {
			revenue := 0.0
			if m.Revenue != nil {
				revenue = *m.Revenue
			}
			return map[string]any{"roi": round2(*m.ROI), "threshold": *r.Threshold, "spend": round2(spend), "revenue": round2(revenue)}, true
		}
	}
	return nil, false
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// DeliverSlack sends due Slack notifications and returns how many it
// handled. A failed post is retried with growing pauses, up to
// slackMaxAttempts; an organization without Slack fails at once.
func (e *Evaluator) DeliverSlack(ctx context.Context, now time.Time) (int, error) {
	jobs, err := e.Repo.claimSlack(ctx, slackBatch, slackLease)
	if err != nil {
		return 0, err
	}
	for _, j := range jobs {
		var sendErr error
		if e.Slack == nil {
			sendErr = errors.New("slack delivery is not configured on this server")
		} else {
			sendErr = e.Slack.PostSlack(ctx, j.OrganizationID, SlackText(j.Kind, j.Params))
		}
		status, errText, retryAt := "sent", "", now
		if sendErr != nil {
			errText = sendErr.Error()
			permanent := errors.Is(sendErr, ErrSlackNotConnected) || e.Slack == nil
			if permanent || j.Attempts >= slackMaxAttempts {
				status = "failed"
			} else {
				status = "pending"
				retryAt = now.Add(time.Duration(j.Attempts*j.Attempts) * time.Minute)
			}
			e.Logger.Warn("alerts: slack delivery failed", "organization_id", j.OrganizationID, "notification_id", j.ID, "attempt", j.Attempts, "final", status == "failed", "error", sendErr)
		}
		if err := e.Repo.finishSlack(ctx, j.OrganizationID, j.ID, status, errText, retryAt); err != nil {
			return len(jobs), err
		}
	}
	return len(jobs), nil
}

func (e *Evaluator) RunLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := e.Evaluate(ctx, time.Now()); err != nil && ctx.Err() == nil {
			e.Logger.Error("alerts: evaluation failed", "error", err)
		}
		if _, err := e.DeliverSlack(ctx, time.Now()); err != nil && ctx.Err() == nil {
			e.Logger.Error("alerts: slack delivery failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
