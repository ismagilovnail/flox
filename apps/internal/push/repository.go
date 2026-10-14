package push

import (
	"context"
	"errors"
	"fmt"
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

	status := BroadcastSent
	if in.SendAt != nil {
		status = BroadcastScheduled
	}
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO push_broadcasts (id, organization_id, name, title, body, icon_url, image_url, target, url,
		                             pwa_ids, campaign_ids, countries, created_by_user_id, status, send_at, sent_at)
		VALUES (flox_new_ulid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
		        CASE WHEN $13 = 'sent' THEN now() END)
		RETURNING id`,
		orgID, in.Name, in.Title, in.Body, in.IconURL, in.ImageURL, string(in.Target), in.URL,
		nonNil(in.Filters.PwaIDs), nonNil(in.Filters.CampaignIDs), nonNil(in.Filters.Countries), userID,
		status, in.SendAt,
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
		INSERT INTO push_broadcast_stats (organization_id, broadcast_id, pwa_id, campaign_id, country, sent)
		SELECT s.organization_id, $5, s.pwa_id, s.campaign_id, s.country, count(*)
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
		ORDER BY send_at, id LIMIT 1 FOR UPDATE SKIP LOCKED`, now,
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

var statsKeys = map[StatsGroup]string{
	StatsByDay:       `to_char((b.sent_at AT TIME ZONE $4)::date, 'YYYY-MM-DD')`,
	StatsByBroadcast: `b.id`,
	StatsByPwa:       `st.pwa_id`,
	StatsByCampaign:  `st.campaign_id`,
	StatsByCountry:   `st.country`,
}

// Stats is push metrics over the organization's broadcasts sent from day
// from to day to (inclusive) in timezone, grouped by group, plus the
// totals. A broadcast counts on its send day: its opens stay with it
// whenever they come.
func (r *Repository) Stats(ctx context.Context, orgID string, from, to time.Time, timezone string, group StatsGroup) ([]StatsRow, StatsCounts, error) {
	key, ok := statsKeys[group]
	if !ok {
		return nil, StatsCounts{}, apierror.Validation("unknown groupBy", map[string]string{"groupBy": "unknown"})
	}
	const where = `st.organization_id = $1 AND b.status = 'sent'
		AND b.sent_at >= ($2::date::timestamp AT TIME ZONE $4)
		AND b.sent_at < (($3::date + 1)::timestamp AT TIME ZONE $4)`
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

const broadcastColumns = `id, name, title, body, icon_url, image_url, target, url, pwa_ids, campaign_ids, countries,
	total, delivered, failed, unsubscribed, opened, status, send_at, sent_at, created_at`

func scanBroadcast(row pgx.Row) (Broadcast, error) {
	var b Broadcast
	var target string
	err := row.Scan(&b.ID, &b.Name, &b.Title, &b.Body, &b.IconURL, &b.ImageURL, &target, &b.URL,
		&b.Filters.PwaIDs, &b.Filters.CampaignIDs, &b.Filters.Countries,
		&b.Total, &b.Delivered, &b.Failed, &b.Unsubscribed, &b.Opened, &b.Status, &b.SendAt, &b.SentAt, &b.CreatedAt)
	b.Target = Target(target)
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
	return b, nil
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
	return out, total, rows.Err()
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
			INSERT INTO push_broadcast_stats (organization_id, broadcast_id, pwa_id, campaign_id, country, opened)
			SELECT pd.organization_id, pd.broadcast_id, pd.pwa_id, pd.campaign_id, pd.country, 1
			FROM `+r.deliveries+` pd JOIN d ON d.id = pd.id WHERE d.first
			ON CONFLICT (broadcast_id, pwa_id, campaign_id, country) DO UPDATE
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
