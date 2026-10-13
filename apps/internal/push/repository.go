package push

import (
	"context"
	"errors"
	"fmt"

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

// CreateBroadcast stores the broadcast and queues one delivery per
// subscriber it matches, in one transaction: the audience is fixed when it
// is sent.
func (r *Repository) CreateBroadcast(ctx context.Context, orgID, userID string, in CreateBroadcastInput) (Broadcast, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Broadcast{}, fmt.Errorf("push: beginning broadcast: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO push_broadcasts (id, organization_id, name, title, body, icon_url, image_url, target, url,
		                             pwa_ids, campaign_ids, countries, created_by_user_id)
		VALUES (flox_new_ulid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id`,
		orgID, in.Name, in.Title, in.Body, in.IconURL, in.ImageURL, string(in.Target), in.URL,
		nonNil(in.Filters.PwaIDs), nonNil(in.Filters.CampaignIDs), nonNil(in.Filters.Countries), userID,
	).Scan(&id); err != nil {
		return Broadcast{}, fmt.Errorf("push: storing broadcast: %w", err)
	}
	args := append(audienceArgs(orgID, in.Filters), id)
	tag, err := tx.Exec(ctx, `
		INSERT INTO `+r.deliveries+` (id, organization_id, broadcast_id, subscription_id)
		SELECT flox_new_ulid(), s.organization_id, $5, s.id
		FROM push_subscriptions s WHERE `+audienceWhere, args...)
	if err != nil {
		return Broadcast{}, fmt.Errorf("push: queueing deliveries: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE push_broadcasts SET total = $2 WHERE id = $1`, id, tag.RowsAffected()); err != nil {
		return Broadcast{}, fmt.Errorf("push: counting broadcast: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Broadcast{}, fmt.Errorf("push: committing broadcast: %w", err)
	}
	return r.GetBroadcast(ctx, orgID, id)
}

const broadcastColumns = `id, name, title, body, icon_url, image_url, target, url, pwa_ids, campaign_ids, countries,
	total, delivered, failed, unsubscribed, opened, created_at`

func scanBroadcast(row pgx.Row) (Broadcast, error) {
	var b Broadcast
	var target string
	err := row.Scan(&b.ID, &b.Name, &b.Title, &b.Body, &b.IconURL, &b.ImageURL, &target, &b.URL,
		&b.Filters.PwaIDs, &b.Filters.CampaignIDs, &b.Filters.Countries,
		&b.Total, &b.Delivered, &b.Failed, &b.Unsubscribed, &b.Opened, &b.CreatedAt)
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
