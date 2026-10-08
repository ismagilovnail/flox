package alert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

type Repository struct {
	db *pgxpool.Pool
	// notifications is the table notifications live in; tests point it at
	// a private copy, out of reach of the dev worker's own evaluation.
	notifications string
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, notifications: "notifications"}
}

const ruleColumns = `id, kind, campaign_id, threshold::float8, min_spend::float8, notify_slack, enabled, created_at, updated_at`

func scanRule(row pgx.Row) (Rule, error) {
	var r Rule
	err := row.Scan(&r.ID, &r.Kind, &r.CampaignID, &r.Threshold, &r.MinSpend, &r.NotifySlack, &r.Enabled, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (r *Repository) ListRules(ctx context.Context, orgID string) ([]Rule, error) {
	rows, err := r.db.Query(ctx, `SELECT `+ruleColumns+` FROM alert_rules WHERE organization_id = $1 ORDER BY created_at, id`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing alert rules: %w", err)
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning alert rule: %w", err)
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (r *Repository) GetRule(ctx context.Context, orgID, id string) (Rule, error) {
	rule, err := scanRule(r.db.QueryRow(ctx, `SELECT `+ruleColumns+` FROM alert_rules WHERE organization_id = $1 AND id = $2`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Rule{}, apierror.NotFound("alert rule not found")
	}
	if err != nil {
		return Rule{}, fmt.Errorf("getting alert rule: %w", err)
	}
	return rule, nil
}

// CampaignExists reports whether campaignID is one of orgID's campaigns.
func (r *Repository) CampaignExists(ctx context.Context, orgID, campaignID string) (bool, error) {
	var ok bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM campaigns WHERE organization_id = $1 AND id = $2)`, orgID, campaignID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("checking campaign: %w", err)
	}
	return ok, nil
}

func (r *Repository) CreateRule(ctx context.Context, id, orgID string, in RuleInput) (Rule, error) {
	rule, err := scanRule(r.db.QueryRow(ctx, `
		INSERT INTO alert_rules (id, organization_id, kind, campaign_id, threshold, min_spend, notify_slack, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+ruleColumns,
		id, orgID, in.Kind, in.CampaignID, in.Threshold, in.MinSpend, in.NotifySlack, in.Enabled))
	if err != nil {
		return Rule{}, fmt.Errorf("creating alert rule: %w", err)
	}
	return rule, nil
}

func (r *Repository) UpdateRule(ctx context.Context, orgID, id string, in RuleInput) (Rule, error) {
	rule, err := scanRule(r.db.QueryRow(ctx, `
		UPDATE alert_rules SET kind = $3, campaign_id = $4, threshold = $5, min_spend = $6, notify_slack = $7, enabled = $8
		WHERE organization_id = $1 AND id = $2
		RETURNING `+ruleColumns,
		orgID, id, in.Kind, in.CampaignID, in.Threshold, in.MinSpend, in.NotifySlack, in.Enabled))
	if errors.Is(err, pgx.ErrNoRows) {
		return Rule{}, apierror.NotFound("alert rule not found")
	}
	if err != nil {
		return Rule{}, fmt.Errorf("updating alert rule: %w", err)
	}
	return rule, nil
}

func (r *Repository) DeleteRule(ctx context.Context, orgID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM alert_rules WHERE organization_id = $1 AND id = $2`, orgID, id)
	if err != nil {
		return fmt.Errorf("deleting alert rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apierror.NotFound("alert rule not found")
	}
	return nil
}

// ListNotifications returns orgID's latest notifications, newest first, each
// marked read or not for userID, plus userID's unread count.
func (r *Repository) ListNotifications(ctx context.Context, orgID, userID string, limit int) (NotificationList, error) {
	// No user (an API key): nothing is read.
	var readUntil *time.Time
	if userID != "" {
		err := r.db.QueryRow(ctx, `SELECT read_until FROM notification_reads WHERE user_id = $1 AND organization_id = $2`, userID, orgID).Scan(&readUntil)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return NotificationList{}, fmt.Errorf("reading notification state: %w", err)
		}
	}

	rows, err := r.db.Query(ctx, `
		SELECT id, rule_id, kind, params, created_at
		FROM `+r.notifications+` WHERE organization_id = $1
		ORDER BY created_at DESC, id DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return NotificationList{}, fmt.Errorf("listing notifications: %w", err)
	}
	defer rows.Close()
	out := NotificationList{Notifications: []Notification{}}
	for rows.Next() {
		var n Notification
		var params []byte
		if err := rows.Scan(&n.ID, &n.RuleID, &n.Kind, &params, &n.CreatedAt); err != nil {
			return NotificationList{}, fmt.Errorf("scanning notification: %w", err)
		}
		if err := json.Unmarshal(params, &n.Params); err != nil {
			return NotificationList{}, fmt.Errorf("decoding notification params: %w", err)
		}
		n.Read = readUntil != nil && !n.CreatedAt.After(*readUntil)
		out.Notifications = append(out.Notifications, n)
	}
	if err := rows.Err(); err != nil {
		return NotificationList{}, err
	}

	err = r.db.QueryRow(ctx, `
		SELECT count(*) FROM `+r.notifications+`
		WHERE organization_id = $1 AND ($2::timestamptz IS NULL OR created_at > $2)`, orgID, readUntil).Scan(&out.Unread)
	if err != nil {
		return NotificationList{}, fmt.Errorf("counting unread notifications: %w", err)
	}
	return out, nil
}

// MarkAllRead marks every notification orgID has so far as read for userID.
func (r *Repository) MarkAllRead(ctx context.Context, orgID, userID string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO notification_reads (user_id, organization_id, read_until) VALUES ($1, $2, now())
		ON CONFLICT (user_id, organization_id) DO UPDATE SET read_until = now()`, userID, orgID)
	if err != nil {
		return fmt.Errorf("marking notifications read: %w", err)
	}
	return nil
}

// --- evaluator side: every organization at once, each row carrying its own
// organization_id, which every write below filters on.

// orgRule is an enabled rule with its organization's timezone.
type orgRule struct {
	Rule
	OrganizationID string
	Timezone       string
}

func (r *Repository) enabledRules(ctx context.Context) ([]orgRule, error) {
	rows, err := r.db.Query(ctx, `
		SELECT a.id, a.kind, a.campaign_id, a.threshold::float8, a.min_spend::float8, a.notify_slack, a.enabled, a.created_at, a.updated_at,
		       a.organization_id, o.timezone
		FROM alert_rules a JOIN organizations o ON o.id = a.organization_id
		WHERE a.enabled
		ORDER BY a.organization_id, a.created_at, a.id`)
	if err != nil {
		return nil, fmt.Errorf("listing enabled alert rules: %w", err)
	}
	defer rows.Close()
	var out []orgRule
	for rows.Next() {
		var o orgRule
		if err := rows.Scan(&o.ID, &o.Kind, &o.CampaignID, &o.Threshold, &o.MinSpend, &o.NotifySlack, &o.Enabled, &o.CreatedAt, &o.UpdatedAt, &o.OrganizationID, &o.Timezone); err != nil {
			return nil, fmt.Errorf("scanning alert rule: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// names maps ids of one organization's table rows to their names.
func (r *Repository) names(ctx context.Context, table, orgID string, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	column := "name"
	if table == "domains" {
		column = "domain"
	}
	rows, err := r.db.Query(ctx, `SELECT id, `+column+` FROM `+table+` WHERE organization_id = $1 AND id = ANY($2)`, orgID, ids)
	if err != nil {
		return nil, fmt.Errorf("reading %s names: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

type pausedOffer struct {
	ID, Name  string
	ChangedAt time.Time
}

// pausedOffers lists orgID's offers paused since `since`.
func (r *Repository) pausedOffers(ctx context.Context, orgID string, since time.Time) ([]pausedOffer, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, status_changed_at FROM offers
		WHERE organization_id = $1 AND status = 'paused' AND status_changed_at >= $2`, orgID, since)
	if err != nil {
		return nil, fmt.Errorf("listing paused offers: %w", err)
	}
	defer rows.Close()
	var out []pausedOffer
	for rows.Next() {
		var p pausedOffer
		if err := rows.Scan(&p.ID, &p.Name, &p.ChangedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type expiringDomain struct {
	ID, Domain string
	ExpiresAt  time.Time
}

// expiringDomains lists orgID's domains whose certificate expires before
// `before` (already expired included).
func (r *Repository) expiringDomains(ctx context.Context, orgID string, before time.Time) ([]expiringDomain, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, domain, ssl_expires_at FROM domains
		WHERE organization_id = $1 AND ssl_expires_at IS NOT NULL AND ssl_expires_at < $2`, orgID, before)
	if err != nil {
		return nil, fmt.Errorf("listing expiring domains: %w", err)
	}
	defer rows.Close()
	var out []expiringDomain
	for rows.Next() {
		var d expiringDomain
		if err := rows.Scan(&d.ID, &d.Domain, &d.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// fire records a notification unless one with the same dedup key exists,
// and reports whether it was new.
func (r *Repository) fire(ctx context.Context, id, orgID string, rule Rule, dedupKey string, params map[string]any) (bool, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return false, fmt.Errorf("encoding notification params: %w", err)
	}
	slack := "none"
	if rule.NotifySlack {
		slack = "pending"
	}
	tag, err := r.db.Exec(ctx, `
		INSERT INTO `+r.notifications+` (id, organization_id, rule_id, kind, dedup_key, params, slack_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (organization_id, dedup_key) DO NOTHING`,
		id, orgID, rule.ID, rule.Kind, dedupKey, body, slack)
	if err != nil {
		return false, fmt.Errorf("recording notification: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

type slackJob struct {
	ID, OrganizationID string
	Kind               Kind
	Params             map[string]any
	Attempts           int
}

// claimSlack takes up to limit notifications due for Slack delivery and
// pushes their next attempt out by lease, so a crashed worker's claim is
// retried after it and two workers never take the same row.
func (r *Repository) claimSlack(ctx context.Context, limit int, lease time.Duration) ([]slackJob, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE `+r.notifications+` SET slack_attempts = slack_attempts + 1, slack_next_at = now() + make_interval(secs => $2)
		WHERE id IN (
			SELECT id FROM `+r.notifications+`
			WHERE slack_status = 'pending' AND slack_next_at <= now()
			ORDER BY slack_next_at LIMIT $1
			FOR UPDATE SKIP LOCKED)
		RETURNING id, organization_id, kind, params, slack_attempts`, limit, lease.Seconds())
	if err != nil {
		return nil, fmt.Errorf("claiming slack notifications: %w", err)
	}
	defer rows.Close()
	var out []slackJob
	for rows.Next() {
		var j slackJob
		var params []byte
		if err := rows.Scan(&j.ID, &j.OrganizationID, &j.Kind, &params, &j.Attempts); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(params, &j.Params); err != nil {
			return nil, fmt.Errorf("decoding notification params: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// finishSlack records one delivery attempt: sent, failed for good, or
// pending again at retryAt.
func (r *Repository) finishSlack(ctx context.Context, orgID, id, status, errText string, retryAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE `+r.notifications+` SET slack_status = $3, slack_error = $4, slack_next_at = $5
		WHERE organization_id = $1 AND id = $2`, orgID, id, status, errText, retryAt)
	if err != nil {
		return fmt.Errorf("recording slack delivery: %w", err)
	}
	return nil
}
