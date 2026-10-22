package push

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
)

// Repository is push's Postgres access. Every query is scoped by
// organization (§36-TENANCY).
type Repository struct {
	db *pgxpool.Pool
	// deliveries is push_deliveries; tests use a private copy so the
	// dev-stack worker cannot send their rows.
	deliveries string
	// scope limits the worker's sends (SendDue, SendTriggered) to these
	// organizations; empty is all. Only tests set it (export_test.go):
	// the dev database is shared, and a test's "later now" would otherwise
	// send other organizations' broadcasts into its private table.
	scope []string
}

// bucketOf is the 15-minute stats bucket (push_broadcast_stats.bucket,
// migration 00074) of a time expression.
func bucketOf(ts string) string {
	return `date_bin('15 minutes', ` + ts + `, timestamptz '2000-01-01 00:00:00+00')`
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, deliveries: "push_deliveries"}
}

// NewRepositoryOnTable is NewRepository over another copy of
// push_deliveries — tests' private tables (testpg.PrivateTable), here
// rather than in an export_test.go because internal/pushsend's tests
// queue broadcasts too.
func NewRepositoryOnTable(db *pgxpool.Pool, deliveries string) *Repository {
	return &Repository{db: db, deliveries: deliveries}
}

// SaveSubscription stores an installed app's subscription and reports
// whether it is new. The same endpoint again (the app re-subscribing)
// refreshes its keys and click and makes it active again — but only within
// the organization that holds it. A PWA not of this organization stores
// nothing.
func (r *Repository) SaveSubscription(ctx context.Context, s NewSubscription) (bool, error) {
	var inserted bool
	err := r.db.QueryRow(ctx, `
		INSERT INTO push_subscriptions (id, organization_id, pwa_id, campaign_id, click_id, endpoint, p256dh, auth,
		                                link_token, context, country, device, language, origin)
		SELECT flox_new_ulid(), $1::text, $2::text, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		WHERE EXISTS (SELECT 1 FROM pwas WHERE id = $2::text AND organization_id = $1::text)
		ON CONFLICT (endpoint) DO UPDATE
		SET p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth,
		    pwa_id = EXCLUDED.pwa_id, campaign_id = EXCLUDED.campaign_id, click_id = EXCLUDED.click_id,
		    link_token = EXCLUDED.link_token, context = EXCLUDED.context, origin = EXCLUDED.origin,
		    country = EXCLUDED.country, device = EXCLUDED.device, language = EXCLUDED.language,
		    status = 'active', unsubscribed_at = NULL
		WHERE push_subscriptions.organization_id = EXCLUDED.organization_id
		RETURNING xmax = 0`,
		s.OrganizationID, s.PwaID, s.CampaignID, s.ClickID, s.Endpoint, s.P256dh, s.Auth,
		s.LinkToken, s.Context, s.Country, s.Device, s.Language, s.Origin,
	).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("push: saving subscription: %w", err)
	}
	return inserted, nil
}

// audienceWhere selects an organization's active subscriptions matching
// filters; $1 org, $2 PWAs, $3 campaigns, $4 countries.
const audienceWhere = `s.organization_id = $1 AND s.status = 'active'
	AND (cardinality($2::text[]) = 0 OR s.pwa_id = ANY($2::text[]))
	AND (cardinality($3::text[]) = 0 OR s.campaign_id = ANY($3::text[]))
	AND (cardinality($4::text[]) = 0 OR s.country = ANY($4::text[]))`

func audienceArgs(orgID string, f Filters) []any {
	return []any{orgID, nonNil(f.PwaIDs), nonNil(f.CampaignIDs), nonNil(f.Countries)}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// OrganizationTimezone is the organization's IANA zone (UTC when unset).
func (r *Repository) OrganizationTimezone(ctx context.Context, orgID string) (string, error) {
	var tz string
	if err := r.db.QueryRow(ctx, `SELECT coalesce(timezone, '') FROM organizations WHERE id = $1`, orgID).Scan(&tz); err != nil {
		return "", fmt.Errorf("push: reading organization timezone: %w", err)
	}
	if tz == "" {
		tz = "UTC"
	}
	return tz, nil
}

// CountAudience is how many subscribers a broadcast with these filters
// would go to now.
func (r *Repository) CountAudience(ctx context.Context, orgID string, f Filters) (int, error) {
	var n int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM push_subscriptions s WHERE `+audienceWhere,
		audienceArgs(orgID, f)...).Scan(&n); err != nil {
		return 0, fmt.Errorf("push: counting audience: %w", err)
	}
	return n, nil
}

// CreateBroadcast stores the broadcast. Sent now (in.SendAt nil), it
// queues one delivery per subscriber it matches in the same transaction:
// the audience is fixed when it is sent. Scheduled, it waits for SendDue.
func (r *Repository) CreateBroadcast(ctx context.Context, orgID, userID string, in CreateBroadcastInput) (Broadcast, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Broadcast{}, fmt.Errorf("push: beginning broadcast: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	kind, status := KindOnce, BroadcastSent
	var delay *int
	condition, statuses := ConditionNone, []string{}
	switch {
	case in.Trigger != nil:
		kind, status = KindTriggered, BroadcastActive
		delay, condition, statuses = &in.Trigger.DelayMinutes, in.Trigger.Condition, nonNil(in.Trigger.Statuses)
	case in.SendAt != nil:
		status = BroadcastScheduled
	}
	// A recurring template: its first slot from now (the service checked
	// there is one).
	var recTime, recUntil *string
	var recWeekdays []int
	recTimezone := ""
	var nextSendAt *time.Time
	if rec := in.Recurrence; rec != nil {
		kind, status = KindRecurring, BroadcastActive
		next, ok, err := NextOccurrence(*rec, time.Now())
		if err != nil || !ok {
			return Broadcast{}, apierror.Validation("invalid recurrence", map[string]string{"recurrence.until": "ends before its first send"})
		}
		recTime, recWeekdays, recTimezone, nextSendAt = &rec.Time, rec.Weekdays, rec.Timezone, &next
		if rec.Until != "" {
			recUntil = &rec.Until
		}
	}
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO push_broadcasts (id, organization_id, name, title, body, icon_url, image_url, target, url,
		                             pwa_ids, campaign_ids, countries, created_by_user_id, status, send_at, sent_at,
		                             kind, trigger_delay_minutes, trigger_condition, trigger_statuses,
		                             recur_weekdays, recur_time, recur_timezone, recur_until, next_send_at)
		VALUES (flox_new_ulid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
		        CASE WHEN $13 = 'sent' THEN now() END, $15, $16, $17, $18,
		        $19, $20::time, $21, $22::date, $23)
		RETURNING id`,
		orgID, in.Name, in.Title, in.Body, in.IconURL, in.ImageURL, string(in.Target), in.URL,
		nonNil(in.Filters.PwaIDs), nonNil(in.Filters.CampaignIDs), nonNil(in.Filters.Countries), userID,
		status, in.SendAt, kind, delay, condition, statuses,
		weekdays16(recWeekdays), recTime, recTimezone, recUntil, nextSendAt,
	).Scan(&id); err != nil {
		return Broadcast{}, fmt.Errorf("push: storing broadcast: %w", err)
	}
	if status == BroadcastSent {
		if err := r.queue(ctx, tx, orgID, id, in.Filters); err != nil {
			return Broadcast{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Broadcast{}, fmt.Errorf("push: committing broadcast: %w", err)
	}
	return r.GetBroadcast(ctx, orgID, id)
}

// queue fixes broadcast id's audience: one delivery per matching active
// subscriber, carrying the subscriber's PWA, campaign and country as they
// are now; the per-group sent counts (push_broadcast_stats); the total.
func (r *Repository) queue(ctx context.Context, tx pgx.Tx, orgID, id string, f Filters) error {
	args := append(audienceArgs(orgID, f), id)
	tag, err := tx.Exec(ctx, `
		INSERT INTO `+r.deliveries+` (id, organization_id, broadcast_id, subscription_id, pwa_id, campaign_id, country)
		SELECT flox_new_ulid(), s.organization_id, $5, s.id, s.pwa_id, s.campaign_id, s.country
		FROM push_subscriptions s WHERE `+audienceWhere, args...)
	if err != nil {
		return fmt.Errorf("push: queueing deliveries: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO push_broadcast_stats (organization_id, broadcast_id, bucket, pwa_id, campaign_id, country, sent)
		SELECT s.organization_id, $5, `+bucketOf("now()")+`, s.pwa_id, s.campaign_id, s.country, count(*)
		FROM push_subscriptions s WHERE `+audienceWhere+`
		GROUP BY s.organization_id, s.pwa_id, s.campaign_id, s.country`, args...); err != nil {
		return fmt.Errorf("push: counting broadcast groups: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE push_broadcasts SET total = $2 WHERE id = $1`, id, tag.RowsAffected()); err != nil {
		return fmt.Errorf("push: counting broadcast: %w", err)
	}
	return nil
}

// SendDue sends up to limit scheduled broadcasts whose send time is at or
// before now, each in its own transaction, and reports how many it sent.
// Broadcasts another worker holds are skipped. A broadcast whose filters
// match no one by then is sent with total 0.
func (r *Repository) SendDue(ctx context.Context, now time.Time, limit int) (int, error) {
	sent := 0
	for sent < limit {
		ok, err := r.sendOneDue(ctx, now)
		if err != nil {
			return sent, err
		}
		if !ok {
			break
		}
		sent++
	}
	return sent, nil
}

func (r *Repository) sendOneDue(ctx context.Context, now time.Time) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("push: beginning scheduled send: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id, orgID string
	var f Filters
	err = tx.QueryRow(ctx, `
		SELECT id, organization_id, pwa_ids, campaign_ids, countries FROM push_broadcasts
		WHERE status = 'scheduled' AND send_at <= $1
		  AND (cardinality($2::text[]) = 0 OR organization_id = ANY($2::text[]))
		ORDER BY send_at, id LIMIT 1 FOR UPDATE SKIP LOCKED`, now, nonNil(r.scope),
	).Scan(&id, &orgID, &f.PwaIDs, &f.CampaignIDs, &f.Countries)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("push: claiming scheduled broadcast: %w", err)
	}
	if err := r.queue(ctx, tx, orgID, id, f); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE push_broadcasts SET status = 'sent', sent_at = now() WHERE id = $1`, id); err != nil {
		return false, fmt.Errorf("push: marking scheduled broadcast sent: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("push: committing scheduled send: %w", err)
	}
	return true, nil
}

// CancelBroadcast calls off a scheduled broadcast. One already sent, or
// canceled, is a conflict.
func (r *Repository) CancelBroadcast(ctx context.Context, orgID, id string) (Broadcast, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE push_broadcasts SET status = 'canceled'
		WHERE id = $1 AND organization_id = $2 AND status = 'scheduled'`, id, orgID)
	if err != nil {
		return Broadcast{}, fmt.Errorf("push: canceling broadcast: %w", err)
	}
	b, err := r.GetBroadcast(ctx, orgID, id)
	if err != nil {
		return Broadcast{}, err
	}
	if tag.RowsAffected() == 0 {
		return Broadcast{}, apierror.Conflict("only a scheduled broadcast can be canceled; this one is " + b.Status)
	}
	return b, nil
}

// SendTriggered sends every active triggered broadcast to the subscribers
// due by now (see Trigger), at most limit per broadcast per call — the next
// call continues. Each broadcast is sent in its own transaction holding its
// row, so two workers never send the same one at once; the deliveries'
// unique (broadcast, subscription) makes it once per subscriber regardless.
// It reports how many deliveries it queued.
func (r *Repository) SendTriggered(ctx context.Context, now time.Time, limit int) (int, error) {
	rows, err := r.db.Query(ctx, `SELECT id FROM push_broadcasts WHERE status = 'active'
		AND (cardinality($1::text[]) = 0 OR organization_id = ANY($1::text[])) ORDER BY id`, nonNil(r.scope))
	if err != nil {
		return 0, fmt.Errorf("push: listing triggers: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, fmt.Errorf("push: listing triggers: %w", err)
	}
	queued := 0
	for _, id := range ids {
		n, err := r.sendTrigger(ctx, id, now, limit)
		queued += n
		if err != nil {
			return queued, err
		}
	}
	return queued, nil
}

func (r *Repository) sendTrigger(ctx context.Context, id string, now time.Time, limit int) (int, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("push: beginning trigger send: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var orgID, condition string
	var f Filters
	var delay int
	var statuses []string
	var created time.Time
	err = tx.QueryRow(ctx, `
		SELECT organization_id, pwa_ids, campaign_ids, countries, trigger_delay_minutes, trigger_condition,
		       trigger_statuses, created_at
		FROM push_broadcasts WHERE id = $1 AND status = 'active' FOR UPDATE SKIP LOCKED`, id,
	).Scan(&orgID, &f.PwaIDs, &f.CampaignIDs, &f.Countries, &delay, &condition, &statuses, &created)
	if errors.Is(err, pgx.ErrNoRows) { // paused meanwhile, or another worker has it
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("push: claiming trigger: %w", err)
	}
	// Due: subscribed after the trigger was created, delay ago, within the
	// window; not sent this broadcast yet; the condition on the click's
	// recorded conversions ($10 'with' / 'without') holds now.
	args := append(audienceArgs(orgID, f), id, created, delay, now, TriggerWindow.Seconds(), condition, nonNil(statuses), limit)
	var n int
	if err := tx.QueryRow(ctx, `
		WITH due AS (
			SELECT s.id, s.organization_id, s.pwa_id, s.campaign_id, s.country
			FROM push_subscriptions s
			WHERE `+audienceWhere+`
			  AND s.created_at >= $6
			  AND s.created_at + make_interval(mins => $7) <= $8
			  AND s.created_at + make_interval(mins => $7) > $8 - make_interval(secs => $9)
			  AND NOT EXISTS (SELECT 1 FROM `+r.deliveries+` d WHERE d.broadcast_id = $5 AND d.subscription_id = s.id)
			  AND ($10 = 'none' OR ($10 = 'with') = (s.click_id <> '' AND (
			        EXISTS (SELECT 1 FROM postbacks p WHERE p.organization_id = s.organization_id AND p.result = 'success'
			                AND p.click_id = s.click_id AND p.status = ANY($11::text[]))
			     OR EXISTS (SELECT 1 FROM postbacks p WHERE p.organization_id = s.organization_id AND p.result = 'success'
			                AND p.attributed_click_id = s.click_id AND p.status = ANY($11::text[])))))
			ORDER BY s.created_at, s.id LIMIT $12
		), ins AS (
			INSERT INTO `+r.deliveries+` (id, organization_id, broadcast_id, subscription_id, pwa_id, campaign_id, country)
			SELECT flox_new_ulid(), organization_id, $5, id, pwa_id, campaign_id, country FROM due
			ON CONFLICT (broadcast_id, subscription_id) DO NOTHING
			RETURNING organization_id, pwa_id, campaign_id, country
		), grouped AS (
			INSERT INTO push_broadcast_stats (organization_id, broadcast_id, bucket, pwa_id, campaign_id, country, sent)
			SELECT organization_id, $5, `+bucketOf("now()")+`, pwa_id, campaign_id, country, count(*)
			FROM ins GROUP BY organization_id, pwa_id, campaign_id, country
			ON CONFLICT (broadcast_id, bucket, pwa_id, campaign_id, country) DO UPDATE
			SET sent = push_broadcast_stats.sent + EXCLUDED.sent
		), counted AS (
			UPDATE push_broadcasts SET total = total + (SELECT count(*) FROM ins) WHERE id = $5
		)
		SELECT count(*) FROM ins`, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("push: sending trigger: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("push: committing trigger send: %w", err)
	}
	return n, nil
}

// SetActive pauses or resumes a triggered or recurring broadcast. A
// resumed trigger sends to the subscribers who became due while it was
// paused only if that was within TriggerWindow; a resumed recurring one
// goes on from its next slot after now (finished if there is none). Any
// other broadcast, or one already so, is a conflict.
func (r *Repository) SetActive(ctx context.Context, orgID, id string, active bool) (Broadcast, error) {
	b, err := r.GetBroadcast(ctx, orgID, id)
	if err != nil {
		return Broadcast{}, err
	}
	from, to := BroadcastActive, BroadcastPaused
	if active {
		from, to = to, from
	}
	if b.Kind == KindOnce || b.Status != from {
		return Broadcast{}, apierror.Conflict("only an " + from + " trigger or recurring broadcast can be set " + to + "; this one is " + b.Kind + ", " + b.Status)
	}
	var next *time.Time
	if active && b.Recurrence != nil {
		n, ok, err := NextOccurrence(*b.Recurrence, time.Now())
		if err != nil {
			return Broadcast{}, err
		}
		if ok {
			next = &n
		} else {
			to = BroadcastFinished
		}
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE push_broadcasts SET status = $3,
		       next_send_at = CASE WHEN kind = 'recurring' AND $3 <> 'paused' THEN $5 ELSE next_send_at END
		WHERE id = $1 AND organization_id = $2 AND status = $4`, id, orgID, to, from, next)
	if err != nil {
		return Broadcast{}, fmt.Errorf("push: setting broadcast %s: %w", to, err)
	}
	if tag.RowsAffected() == 0 { // changed meanwhile
		return Broadcast{}, apierror.Conflict("the broadcast changed meanwhile; reload it")
	}
	return r.GetBroadcast(ctx, orgID, id)
}

// SendRecurring makes each active recurring template whose slot is due by
// now into a sent broadcast — at most limit per call — and reports how
// many it sent. Each template is handled in its own transaction holding
// its row, which also moves it to its next slot, so a slot goes out once
// however many workers run. A slot reached more than RecurringLateness
// late is skipped; one whose filters match no one creates nothing.
func (r *Repository) SendRecurring(ctx context.Context, now time.Time, limit int) (int, error) {
	sent := 0
	for range limit {
		ok, did, err := r.sendOneRecurring(ctx, now)
		if err != nil {
			return sent, err
		}
		if !ok {
			break
		}
		if did {
			sent++
		}
	}
	return sent, nil
}

// sendOneRecurring handles one due template: ok false when none is due;
// sent whether it made a broadcast.
func (r *Repository) sendOneRecurring(ctx context.Context, now time.Time) (ok, sent bool, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, false, fmt.Errorf("push: beginning recurring send: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var orgID string
	b, err := scanBroadcast(tx.QueryRow(ctx, `
		SELECT `+broadcastColumns+`, organization_id FROM push_broadcasts
		WHERE kind = 'recurring' AND status = 'active' AND next_send_at <= $1
		  AND (cardinality($2::text[]) = 0 OR organization_id = ANY($2::text[]))
		ORDER BY next_send_at, id LIMIT 1 FOR UPDATE SKIP LOCKED`, now, nonNil(r.scope)), &orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("push: claiming recurring broadcast: %w", err)
	}
	slot := *b.NextSendAt
	if now.Sub(slot) <= RecurringLateness {
		n, err := r.countAudienceTx(ctx, tx, orgID, b.Filters)
		if err != nil {
			return false, false, err
		}
		if n > 0 {
			loc, _ := time.LoadLocation(b.Recurrence.Timezone)
			var childID string
			if err := tx.QueryRow(ctx, `
				INSERT INTO push_broadcasts (id, organization_id, name, title, body, icon_url, image_url, target, url,
				                             pwa_ids, campaign_ids, countries, created_by_user_id, status, sent_at, kind, parent_id)
				SELECT flox_new_ulid(), organization_id, name || ' · ' || $2, title, body, icon_url, image_url, target, url,
				       pwa_ids, campaign_ids, countries, created_by_user_id, 'sent', now(), 'once', id
				FROM push_broadcasts WHERE id = $1
				RETURNING id`, b.ID, slot.In(loc).Format(time.DateOnly)).Scan(&childID); err != nil {
				return false, false, fmt.Errorf("push: storing recurring send: %w", err)
			}
			if err := r.queue(ctx, tx, orgID, childID, b.Filters); err != nil {
				return false, false, err
			}
			sent = true
		}
	}
	next, more, err := NextOccurrence(*b.Recurrence, now)
	if err != nil {
		return false, false, err
	}
	if more {
		_, err = tx.Exec(ctx, `UPDATE push_broadcasts SET next_send_at = $2 WHERE id = $1`, b.ID, next)
	} else {
		_, err = tx.Exec(ctx, `UPDATE push_broadcasts SET status = 'finished', next_send_at = NULL WHERE id = $1`, b.ID)
	}
	if err != nil {
		return false, false, fmt.Errorf("push: moving recurring broadcast on: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, false, fmt.Errorf("push: committing recurring send: %w", err)
	}
	return true, sent, nil
}

func (r *Repository) countAudienceTx(ctx context.Context, tx pgx.Tx, orgID string, f Filters) (int, error) {
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM push_subscriptions s WHERE `+audienceWhere,
		audienceArgs(orgID, f)...).Scan(&n); err != nil {
		return 0, fmt.Errorf("push: counting audience: %w", err)
	}
	return n, nil
}

// weekdays16 is ISO weekdays as recur_weekdays' smallint[].
func weekdays16(days []int) []int16 {
	out := make([]int16, len(days))
	for i, d := range days {
		out[i] = int16(d)
	}
	return out
}

// withRecurringTotals sets each recurring template's Occurrences and
// counters from its sends (they hold the deliveries).
func (r *Repository) withRecurringTotals(ctx context.Context, orgID string, bs []Broadcast) error {
	idx := map[string]int{}
	var ids []string
	for i, b := range bs {
		if b.Kind == KindRecurring {
			idx[b.ID] = i
			ids = append(ids, b.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT parent_id, count(*), sum(total), sum(delivered), sum(failed), sum(unsubscribed), sum(opened)
		FROM push_broadcasts WHERE organization_id = $1 AND parent_id = ANY($2::text[])
		GROUP BY parent_id`, orgID, ids)
	if err != nil {
		return fmt.Errorf("push: summing recurring sends: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n, total, delivered, failed, unsub, opened int
		if err := rows.Scan(&id, &n, &total, &delivered, &failed, &unsub, &opened); err != nil {
			return fmt.Errorf("push: scanning recurring sums: %w", err)
		}
		b := &bs[idx[id]]
		b.Occurrences, b.Total, b.Delivered, b.Failed, b.Unsubscribed, b.Opened = n, total, delivered, failed, unsub, opened
	}
	return rows.Err()
}

var statsKeys = map[StatsGroup]string{
	StatsByDay:       `to_char((st.bucket AT TIME ZONE $4)::date, 'YYYY-MM-DD')`,
	StatsByBroadcast: `b.id`,
	StatsByPwa:       `st.pwa_id`,
	StatsByCampaign:  `st.campaign_id`,
	StatsByCountry:   `st.country`,
}

// Stats is push metrics over what the organization's broadcasts sent from
// day from to day to (inclusive) in timezone, grouped by group, plus the
// totals. Deliveries count on the day they were queued (a trigger's spread
// over the days it sent): their opens stay with them whenever they come.
func (r *Repository) Stats(ctx context.Context, orgID string, from, to time.Time, timezone string, group StatsGroup) ([]StatsRow, StatsCounts, error) {
	key, ok := statsKeys[group]
	if !ok {
		return nil, StatsCounts{}, apierror.Validation("unknown groupBy", map[string]string{"groupBy": "unknown"})
	}
	const where = `st.organization_id = $1
		AND st.bucket >= ($2::date::timestamp AT TIME ZONE $4)
		AND st.bucket < (($3::date + 1)::timestamp AT TIME ZONE $4)`
	label := `''`
	if group == StatsByBroadcast {
		label = `max(b.name)`
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+key+` AS k, `+label+`, sum(st.sent), sum(st.delivered), sum(st.failed), sum(st.unsubscribed), sum(st.opened)
		FROM push_broadcast_stats st
		JOIN push_broadcasts b ON b.id = st.broadcast_id AND b.organization_id = st.organization_id
		WHERE `+where+`
		GROUP BY k ORDER BY k`, orgID, from.Format(time.DateOnly), to.Format(time.DateOnly), timezone)
	if err != nil {
		return nil, StatsCounts{}, fmt.Errorf("push: querying stats: %w", err)
	}
	defer rows.Close()
	out := []StatsRow{}
	var total StatsCounts
	for rows.Next() {
		var row StatsRow
		if err := rows.Scan(&row.Key, &row.Label, &row.Sent, &row.Delivered, &row.Failed, &row.Unsubscribed, &row.Opened); err != nil {
			return nil, StatsCounts{}, fmt.Errorf("push: scanning stats: %w", err)
		}
		total.Sent += row.Sent
		total.Delivered += row.Delivered
		total.Failed += row.Failed
		total.Unsubscribed += row.Unsubscribed
		total.Opened += row.Opened
		out = append(out, row)
	}
	return out, total, rows.Err()
}

// reportStatsColumns are the push_broadcast_stats columns the report
// builder can group and filter by.
var reportStatsColumns = map[StatsGroup]string{
	StatsByCampaign: "st.campaign_id",
	StatsByCountry:  "st.country",
	StatsByPwa:      "st.pwa_id",
}

// ReportStats is push metrics grouped for the analytics report builder
// (ReportStatsQuery). Rows without a split (broadcasts sent before
// migration 00073) carry empty campaign, country and PWA.
func (r *Repository) ReportStats(ctx context.Context, orgID string, q ReportStatsQuery) ([]ReportStatsRow, error) {
	args := []any{orgID, q.From, q.To}
	where := `st.organization_id = $1 AND st.bucket >= $2 AND st.bucket < $3`
	keys := []string{`''`}
	if q.ByDay {
		tz := q.Timezone
		if tz == "" {
			tz = "UTC"
		}
		args = append(args, tz)
		keys[0] = fmt.Sprintf(`to_char((st.bucket AT TIME ZONE $%d)::date, 'YYYY-MM-DD')`, len(args))
	}
	for _, f := range q.Filters {
		col, ok := reportStatsColumns[f.Group]
		if !ok {
			return nil, fmt.Errorf("push: report stats cannot filter by %q", f.Group)
		}
		if f.In != nil {
			args = append(args, f.In)
			where += fmt.Sprintf(` AND %s::text = ANY($%d)`, col, len(args))
			continue
		}
		args = append(args, f.Value)
		where += fmt.Sprintf(` AND %s = $%d`, col, len(args))
	}
	for _, g := range q.GroupBy {
		col, ok := reportStatsColumns[g]
		if !ok {
			return nil, fmt.Errorf("push: report stats cannot group by %q", g)
		}
		keys = append(keys, col)
	}
	groupBy := make([]string, len(keys))
	for i := range keys {
		groupBy[i] = fmt.Sprint(i + 1)
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+strings.Join(keys, ", ")+`, sum(st.sent), sum(st.delivered), sum(st.failed), sum(st.unsubscribed), sum(st.opened)
		FROM push_broadcast_stats st
		WHERE `+where+`
		GROUP BY `+strings.Join(groupBy, ", "), args...)
	if err != nil {
		return nil, fmt.Errorf("push: querying report stats: %w", err)
	}
	defer rows.Close()
	out := []ReportStatsRow{}
	for rows.Next() {
		row := ReportStatsRow{Keys: make(map[StatsGroup]string, len(q.GroupBy))}
		vals := make([]string, len(q.GroupBy))
		dest := []any{&row.Day}
		for i := range vals {
			dest = append(dest, &vals[i])
		}
		dest = append(dest, &row.Sent, &row.Delivered, &row.Failed, &row.Unsubscribed, &row.Opened)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("push: scanning report stats: %w", err)
		}
		for i, g := range q.GroupBy {
			row.Keys[g] = vals[i]
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

const broadcastColumns = `id, name, title, body, icon_url, image_url, target, url, pwa_ids, campaign_ids, countries,
	total, delivered, failed, unsubscribed, opened, kind, status, trigger_delay_minutes, trigger_condition,
	trigger_statuses, recur_weekdays, to_char(recur_time, 'HH24:MI'), recur_timezone,
	coalesce(to_char(recur_until, 'YYYY-MM-DD'), ''), next_send_at, parent_id, send_at, sent_at, created_at`

// scanBroadcast reads broadcastColumns, then any extra columns the query
// selected after them into extra.
func scanBroadcast(row pgx.Row, extra ...any) (Broadcast, error) {
	var b Broadcast
	var target, condition, recTimezone, recUntil string
	var delay *int
	var statuses []string
	var weekdays []int16
	var recTime *string
	dest := []any{&b.ID, &b.Name, &b.Title, &b.Body, &b.IconURL, &b.ImageURL, &target, &b.URL,
		&b.Filters.PwaIDs, &b.Filters.CampaignIDs, &b.Filters.Countries,
		&b.Total, &b.Delivered, &b.Failed, &b.Unsubscribed, &b.Opened, &b.Kind, &b.Status, &delay, &condition,
		&statuses, &weekdays, &recTime, &recTimezone, &recUntil, &b.NextSendAt, &b.ParentID, &b.SendAt, &b.SentAt, &b.CreatedAt}
	err := row.Scan(append(dest, extra...)...)
	b.Target = Target(target)
	if delay != nil {
		b.Trigger = &Trigger{DelayMinutes: *delay, Condition: condition, Statuses: statuses}
	}
	if recTime != nil {
		days := make([]int, len(weekdays))
		for i, d := range weekdays {
			days[i] = int(d)
		}
		b.Recurrence = &Recurrence{Weekdays: days, Time: *recTime, Timezone: recTimezone, Until: recUntil}
	}
	return b, err
}

func (r *Repository) GetBroadcast(ctx context.Context, orgID, id string) (Broadcast, error) {
	b, err := scanBroadcast(r.db.QueryRow(ctx, `SELECT `+broadcastColumns+` FROM push_broadcasts WHERE id = $1 AND organization_id = $2`, id, orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, apierror.NotFound("broadcast not found")
	}
	if err != nil {
		return Broadcast{}, fmt.Errorf("push: reading broadcast: %w", err)
	}
	one := []Broadcast{b}
	if err := r.withRecurringTotals(ctx, orgID, one); err != nil {
		return Broadcast{}, err
	}
	return one[0], nil
}

// ListBroadcasts is the organization's broadcasts, newest first.
func (r *Repository) ListBroadcasts(ctx context.Context, orgID string, page pagination.Page) ([]Broadcast, int, error) {
	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM push_broadcasts WHERE organization_id = $1`, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("push: counting broadcasts: %w", err)
	}
	rows, err := r.db.Query(ctx, `SELECT `+broadcastColumns+` FROM push_broadcasts
		WHERE organization_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`, orgID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("push: listing broadcasts: %w", err)
	}
	defer rows.Close()
	out := []Broadcast{}
	for rows.Next() {
		b, err := scanBroadcast(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("push: scanning broadcast: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()
	if err := r.withRecurringTotals(ctx, orgID, out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// SubscriptionFilter narrows the subscriber list; empty fields do not.
type SubscriptionFilter struct {
	Status     string
	PwaID      string
	CampaignID string
	Country    string
}

func (r *Repository) ListSubscriptions(ctx context.Context, orgID string, f SubscriptionFilter, page pagination.Page) ([]Subscription, int, error) {
	where := `organization_id = $1
		AND ($2 = '' OR status = $2) AND ($3 = '' OR pwa_id = $3)
		AND ($4 = '' OR campaign_id = $4) AND ($5 = '' OR country = $5)`
	args := []any{orgID, f.Status, f.PwaID, f.CampaignID, f.Country}
	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM push_subscriptions WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("push: counting subscriptions: %w", err)
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, pwa_id, campaign_id, click_id, endpoint, country, device, language,
		       status, unsubscribed_at, created_at
		FROM push_subscriptions WHERE `+where+`
		ORDER BY created_at DESC, id DESC LIMIT $6 OFFSET $7`, append(args, page.Limit, page.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("push: listing subscriptions: %w", err)
	}
	defer rows.Close()
	out := []Subscription{}
	for rows.Next() {
		var s Subscription
		var endpoint string
		if err := rows.Scan(&s.ID, &s.OrganizationID, &s.PwaID, &s.CampaignID, &s.ClickID, &endpoint,
			&s.Country, &s.Device, &s.Language, &s.Status, &s.UnsubscribedAt, &s.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("push: scanning subscription: %w", err)
		}
		s.PushService = serviceHost(endpoint)
		out = append(out, s)
	}
	return out, total, rows.Err()
}

// Summary counts the organization's subscribers.
func (r *Repository) Summary(ctx context.Context, orgID string) (Summary, error) {
	var s Summary
	if err := r.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'active'), count(*) FILTER (WHERE status = 'unsubscribed')
		FROM push_subscriptions WHERE organization_id = $1`, orgID).Scan(&s.Active, &s.Unsubscribed); err != nil {
		return Summary{}, fmt.Errorf("push: counting subscribers: %w", err)
	}
	var err error
	for _, g := range []struct {
		column string
		into   *[]GroupCount
	}{{"pwa_id", &s.ByPwa}, {"campaign_id", &s.ByCampaign}, {"country", &s.ByCountry}} {
		if *g.into, err = r.groupCounts(ctx, orgID, g.column); err != nil {
			return Summary{}, err
		}
	}
	return s, nil
}

// groupCounts is the active subscribers by column (a constant, never
// input), largest first.
func (r *Repository) groupCounts(ctx context.Context, orgID, column string) ([]GroupCount, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+column+`, count(*) FROM push_subscriptions
		WHERE organization_id = $1 AND status = 'active'
		GROUP BY 1 ORDER BY 2 DESC, 1 LIMIT 100`, orgID)
	if err != nil {
		return nil, fmt.Errorf("push: grouping subscribers by %s: %w", column, err)
	}
	defer rows.Close()
	out := []GroupCount{}
	for rows.Next() {
		var g GroupCount
		if err := rows.Scan(&g.Key, &g.Count); err != nil {
			return nil, fmt.Errorf("push: scanning group: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Open is what a notification's click leads to.
type Open struct {
	Target Target
	URL    string
	// First: this delivery's first click — counted on its broadcast.
	First bool
}

// RecordOpen counts a notification's click on its broadcast (once per
// delivery) and returns where it leads. The delivery must be one sent to
// clickID's subscription in this organization; ok is false otherwise, and
// for a delivery deleted by retention.
func (r *Repository) RecordOpen(ctx context.Context, orgID, clickID, deliveryID string) (Open, bool, error) {
	var o Open
	var target string
	err := r.db.QueryRow(ctx, `
		WITH d AS (
			SELECT d.id, d.broadcast_id, d.opened_at IS NULL AS first
			FROM `+r.deliveries+` d
			JOIN push_subscriptions s ON s.id = d.subscription_id AND s.organization_id = d.organization_id
			WHERE d.id = $1 AND d.organization_id = $2 AND s.click_id = $3
		), mark AS (
			UPDATE `+r.deliveries+` pd SET opened_at = now()
			FROM d WHERE pd.id = d.id AND d.first
		), counted AS (
			UPDATE push_broadcasts SET opened = opened + 1
			FROM d WHERE push_broadcasts.id = d.broadcast_id AND d.first
		), grouped AS (
			INSERT INTO push_broadcast_stats (organization_id, broadcast_id, bucket, pwa_id, campaign_id, country, opened)
			SELECT pd.organization_id, pd.broadcast_id, `+bucketOf("pd.created_at")+`, pd.pwa_id, pd.campaign_id, pd.country, 1
			FROM `+r.deliveries+` pd JOIN d ON d.id = pd.id WHERE d.first
			ON CONFLICT (broadcast_id, bucket, pwa_id, campaign_id, country) DO UPDATE
			SET opened = push_broadcast_stats.opened + 1
		)
		SELECT b.target, b.url, d.first FROM d JOIN push_broadcasts b ON b.id = d.broadcast_id`,
		deliveryID, orgID, clickID,
	).Scan(&target, &o.URL, &o.First)
	if errors.Is(err, pgx.ErrNoRows) {
		return Open{}, false, nil
	}
	if err != nil {
		return Open{}, false, fmt.Errorf("push: recording open: %w", err)
	}
	o.Target = Target(target)
	return o, true, nil
}
