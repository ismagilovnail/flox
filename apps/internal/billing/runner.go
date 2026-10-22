package billing

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Interval is how often apps/worker runs Runner.
const Interval = 10 * time.Minute

const (
	// pendingWindow: pending payments younger than this are read back from
	// the provider each run (a lost notification); older ones are given
	// up as abandoned (CancelStale). The first minutes are left to the
	// notification.
	pendingWindow   = 48 * time.Hour
	pendingSettle   = 5 * time.Minute
	pendingPerRun   = 200
	reminderSubject = "FLOX: продлите тариф"
	lapsedSubject   = "FLOX: тариф закончился"
)

// Mailer sends the owners a notice; *mailer.Client is the real one.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// Runner is apps/worker's billing job: lost notifications, renewal
// reminders, and moving lapsed plans to Free.
type Runner struct {
	Repo     *Repository
	Payments *Payments // nil or without a provider: pending payments are not read back
	Mail     Mailer    // nil: no emails
	AppURL   string
	Logger   *slog.Logger
	Now      func() time.Time
	// Only limits a run to one organization (tests); "" is all of them.
	Only string
}

func (r *Runner) RunLoop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if err := r.RunOnce(ctx); err != nil && ctx.Err() == nil {
			r.Logger.Error("billing run failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (r *Runner) RunOnce(ctx context.Context) error {
	now := time.Now()
	if r.Now != nil {
		now = r.Now()
	}
	if r.Payments.Available() {
		pending, err := r.Repo.PendingPayments(ctx, r.Only, now.Add(-pendingWindow), now.Add(-pendingSettle), pendingPerRun)
		if err != nil {
			return err
		}
		for _, p := range pending {
			if err := r.Payments.Refresh(ctx, p); err != nil {
				r.Logger.Warn("billing: reading a pending payment back", "payment_id", p.ID, "error", err)
			}
		}
	}
	if n, err := r.Repo.CancelStale(ctx, r.Only, now.Add(-pendingWindow)); err != nil {
		return err
	} else if n > 0 {
		r.Logger.Info("billing: abandoned payments cancelled", "count", n)
	}

	due, err := r.Repo.ClaimReminders(ctx, r.Only, now)
	if err != nil {
		return err
	}
	for _, d := range due {
		body := fmt.Sprintf("Тариф %s оплачен до %s (UTC). Через %d дня после этой даты организация перейдёт на Free: трекинг продолжит работать, но с лимитами Free.\n\nПродлить: %s/settings?tab=billing\n",
			d.PlanName, d.PaidUntil.UTC().Format("02.01.2006 15:04"), int(GracePeriod.Hours()/24), strings.TrimRight(r.AppURL, "/"))
		r.notifyOwners(ctx, d.OrganizationID, reminderSubject, body)
	}

	lapsed, err := r.Repo.ExpireLapsed(ctx, r.Only, now)
	if err != nil {
		return err
	}
	for _, l := range lapsed {
		r.Logger.Info("billing: paid period over, moved to Free", "organization_id", l.OrganizationID, "plan", l.PlanName)
		body := fmt.Sprintf("Оплаченный период тарифа %s закончился, организация переведена на Free. Данные и трекинг не тронуты; действуют лимиты Free.\n\nОплатить тариф: %s/settings?tab=billing\n",
			l.PlanName, strings.TrimRight(r.AppURL, "/"))
		r.notifyOwners(ctx, l.OrganizationID, lapsedSubject, body)
	}
	return nil
}

func (r *Runner) notifyOwners(ctx context.Context, orgID, subject, body string) {
	if r.Mail == nil {
		return
	}
	emails, err := r.Repo.OwnerEmails(ctx, orgID)
	if err != nil {
		r.Logger.Warn("billing: listing owners to notify", "organization_id", orgID, "error", err)
		return
	}
	for _, e := range emails {
		if err := r.Mail.Send(ctx, e, subject, body); err != nil {
			r.Logger.Warn("billing: emailing an owner", "organization_id", orgID, "error", err)
		}
	}
}
