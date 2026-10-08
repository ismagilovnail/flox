package alert

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/analytics"
	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/offercap"
	"github.com/ismagilovnail/flox/apps/internal/testpg"
)

func mustPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connecting to database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", strings.Fields(sql)[0], err)
	}
}

// fixture is one organization with a campaign, an offer and a user.
type fixture struct {
	org, campaign, network, offer, user string
}

func seed(t *testing.T, pool *pgxpool.Pool, timezone string) fixture {
	t.Helper()
	f := fixture{org: idgen.New(), campaign: idgen.New(), network: idgen.New(), offer: idgen.New(), user: idgen.New()}
	exec(t, pool, `INSERT INTO organizations (id, name, timezone) VALUES ($1, $2, $3)`, f.org, "alerts "+f.org, timezone)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, f.org)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, f.user)
	})
	source := idgen.New()
	exec(t, pool, `INSERT INTO traffic_sources (id, organization_id, name, type) VALUES ($1, $2, 'src', 'Facebook')`, source, f.org)
	exec(t, pool, `INSERT INTO campaigns (id, organization_id, traffic_source_id, name, fallback_url) VALUES ($1, $2, $3, 'Camp A', 'https://example.com')`, f.campaign, f.org, source)
	exec(t, pool, `INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, 'net')`, f.network, f.org)
	exec(t, pool, `INSERT INTO offers (id, organization_id, network_id, name, payout) VALUES ($1, $2, $3, 'Offer One', 10)`, f.offer, f.org, f.network)
	exec(t, pool, `INSERT INTO users (id, email, name) VALUES ($1, $2, 'U')`, f.user, f.user+"@example.com")
	return f
}

func privateRepo(t *testing.T, pool *pgxpool.Pool) *Repository {
	r := NewRepository(pool)
	r.notifications = testpg.PrivateTable(t, pool, "notifications")
	return r
}

func ptr[T any](v T) *T { return &v }

func isCode(err error, code string) bool {
	var apiErr *apierror.Error
	return errors.As(err, &apiErr) && apiErr.Code == code
}

func TestNewOrganizationGetsDefaultRules(t *testing.T) {
	pool := mustPool(t)
	f := seed(t, pool, "UTC")
	rules, err := NewService(NewRepository(pool)).ListRules(context.Background(), f.org)
	if err != nil {
		t.Fatal(err)
	}
	got := map[Kind]Rule{}
	for _, r := range rules {
		got[r.Kind] = r
	}
	if len(rules) != 3 || got[KindOfferCapped].ID == "" || got[KindOfferPaused].ID == "" || got[KindSSLExpiring].ID == "" {
		t.Fatalf("default rules = %+v, want offer_capped, offer_paused, ssl_expiring", rules)
	}
	if th := got[KindSSLExpiring].Threshold; th == nil || *th != 14 {
		t.Fatalf("ssl_expiring threshold = %v, want 14 days", th)
	}
}

func TestRuleValidation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	a, b := seed(t, pool, "UTC"), seed(t, pool, "UTC")
	svc := NewService(NewRepository(pool))

	for name, in := range map[string]RuleInput{
		"unknown kind":             {Kind: "nope"},
		"spend without threshold":  {Kind: KindSpendOver},
		"spend zero":               {Kind: KindSpendOver, Threshold: ptr(0.0)},
		"roi without threshold":    {Kind: KindROIBelow},
		"roi negative min spend":   {Kind: KindROIBelow, Threshold: ptr(0.0), MinSpend: ptr(-1.0)},
		"ssl fractional days":      {Kind: KindSSLExpiring, Threshold: ptr(2.5)},
		"ssl too far":              {Kind: KindSSLExpiring, Threshold: ptr(91.0)},
		"other org's campaign":     {Kind: KindSpendOver, Threshold: ptr(10.0), CampaignID: &b.campaign},
		"campaign that is no ulid": {Kind: KindSpendOver, Threshold: ptr(10.0), CampaignID: ptr("x")},
	} {
		if _, err := svc.CreateRule(ctx, a.org, in); !isCode(err, "validation") {
			t.Errorf("%s: err = %v, want a validation error", name, err)
		}
	}

	r, err := svc.CreateRule(ctx, a.org, RuleInput{Kind: KindOfferPaused, Threshold: ptr(5.0), CampaignID: &a.campaign, NotifySlack: true, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.Threshold != nil || r.CampaignID != nil || !r.NotifySlack {
		t.Fatalf("offer_paused rule = %+v, want threshold and campaign cleared", r)
	}
	r, err = svc.CreateRule(ctx, a.org, RuleInput{Kind: KindROIBelow, Threshold: ptr(-20.0), MinSpend: ptr(50.0), CampaignID: &a.campaign, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.CampaignID == nil || *r.CampaignID != a.campaign || *r.Threshold != -20 || *r.MinSpend != 50 {
		t.Fatalf("roi rule = %+v", r)
	}
}

// Tenant isolation (CLAUDE.md #5): another organization can neither see nor
// change a rule, nor see the notifications.
func TestCrossTenantIsolation(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	a, b := seed(t, pool, "UTC"), seed(t, pool, "UTC")
	repo := privateRepo(t, pool)
	svc := NewService(repo)

	rule, err := svc.CreateRule(ctx, a.org, RuleInput{Kind: KindSpendOver, Threshold: ptr(10.0), Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateRule(ctx, b.org, rule.ID, RuleInput{Kind: KindSpendOver, Threshold: ptr(1.0)}); !isCode(err, "not_found") {
		t.Fatalf("update from another org: %v, want not_found", err)
	}
	if err := svc.DeleteRule(ctx, b.org, rule.ID); !isCode(err, "not_found") {
		t.Fatalf("delete from another org: %v, want not_found", err)
	}
	rulesB, _ := svc.ListRules(ctx, b.org)
	for _, r := range rulesB {
		if r.ID == rule.ID {
			t.Fatal("org B lists org A's rule")
		}
	}

	if _, err := repo.fire(ctx, idgen.New(), a.org, rule, "k1", map[string]any{"x": 1}); err != nil {
		t.Fatal(err)
	}
	listB, err := svc.Notifications(ctx, b.org, b.user)
	if err != nil {
		t.Fatal(err)
	}
	if len(listB.Notifications) != 0 || listB.Unread != 0 {
		t.Fatalf("org B sees %+v", listB)
	}
}

func TestNotificationsReadStateIsPerUser(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	f := seed(t, pool, "UTC")
	other := idgen.New()
	exec(t, pool, `INSERT INTO users (id, email, name) VALUES ($1, $2, 'U2')`, other, other+"@example.com")
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, other) })
	repo := privateRepo(t, pool)
	svc := NewService(repo)
	rules, _ := svc.ListRules(ctx, f.org)
	rule := rules[0]

	for _, k := range []string{"a", "b"} {
		if _, err := repo.fire(ctx, idgen.New(), f.org, rule, k, map[string]any{"offerName": k}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := svc.Notifications(ctx, f.org, f.user)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Notifications) != 2 || list.Unread != 2 || list.Notifications[0].Read {
		t.Fatalf("before read: %+v", list)
	}
	if err := svc.MarkAllRead(ctx, f.org, f.user); err != nil {
		t.Fatal(err)
	}
	list, _ = svc.Notifications(ctx, f.org, f.user)
	if list.Unread != 0 || !list.Notifications[0].Read || !list.Notifications[1].Read {
		t.Fatalf("after read: %+v", list)
	}
	if l, _ := svc.Notifications(ctx, f.org, other); l.Unread != 2 {
		t.Fatalf("other user's unread = %d, want 2 (read state is per user)", l.Unread)
	}
	if _, err := repo.fire(ctx, idgen.New(), f.org, rule, "c", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if l, _ := svc.Notifications(ctx, f.org, f.user); l.Unread != 1 {
		t.Fatalf("unread after a new one = %d, want 1", l.Unread)
	}
	// An API key has no user: everything is unread, nothing breaks.
	if l, err := svc.Notifications(ctx, f.org, ""); err != nil || l.Unread != 3 {
		t.Fatalf("no user: %+v, %v", l, err)
	}
}

type fakeReports map[string][]analytics.ReportRow

func (f fakeReports) Report(_ context.Context, orgID string, in analytics.ReportInput) ([]analytics.ReportRow, error) {
	return f[orgID], nil
}

func campaignRow(id string, cost, revenue *float64) analytics.ReportRow {
	m := analytics.MetricValues{Cost: cost, Revenue: revenue}
	if cost != nil && revenue != nil && *cost > 0 {
		roi := (*revenue - *cost) / *cost * 100
		m.ROI = &roi
	}
	return analytics.ReportRow{DimValues: map[chstore.ReportDimension]string{chstore.DimCampaign: id}, Metrics: m}
}

type firedRow struct {
	kind, key string
	params    string
	slack     string
}

func fired(t *testing.T, pool *pgxpool.Pool, repo *Repository, orgID string) []firedRow {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT kind, dedup_key, params::text, slack_status FROM `+repo.notifications+` WHERE organization_id = $1 ORDER BY dedup_key`, orgID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []firedRow
	for rows.Next() {
		var r firedRow
		if err := rows.Scan(&r.kind, &r.key, &r.params, &r.slack); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func TestEvaluateFiresEachConditionOnce(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	f := seed(t, pool, "Europe/Moscow")
	repo := privateRepo(t, pool)
	svc := NewService(repo)

	// A second campaign, and rules: spend > 100 everywhere (Slack), ROI < 0
	// everywhere, ROI < 0 only past $500 spend, spend > 1 on campaign B only.
	campB := idgen.New()
	exec(t, pool, `INSERT INTO campaigns (id, organization_id, traffic_source_id, name, fallback_url)
		SELECT $1, organization_id, traffic_source_id, 'Camp B', fallback_url FROM campaigns WHERE id = $2`, campB, f.campaign)
	for _, in := range []RuleInput{
		{Kind: KindSpendOver, Threshold: ptr(100.0), NotifySlack: true, Enabled: true},
		{Kind: KindROIBelow, Threshold: ptr(0.0), Enabled: true},
		{Kind: KindROIBelow, Threshold: ptr(0.0), MinSpend: ptr(500.0), Enabled: true},
		{Kind: KindSpendOver, Threshold: ptr(1.0), CampaignID: &campB, Enabled: true},
		{Kind: KindSpendOver, Threshold: ptr(1.0), Enabled: false},
	} {
		if _, err := svc.CreateRule(ctx, f.org, in); err != nil {
			t.Fatal(err)
		}
	}

	// The offer is paused after the default rule exists; a domain expires in
	// 5 days (inside the default 14), another in 30.
	exec(t, pool, `UPDATE offers SET status = 'paused' WHERE id = $1`, f.offer)
	soon, later := idgen.New(), idgen.New()
	exec(t, pool, `INSERT INTO domains (id, organization_id, domain, ssl_expires_at) VALUES ($1, $2, $3, now() + interval '5 days')`, soon, f.org, "soon-"+strings.ToLower(soon)+".example.com")
	exec(t, pool, `INSERT INTO domains (id, organization_id, domain, ssl_expires_at) VALUES ($1, $2, $3, now() + interval '30 days')`, later, f.org, "later-"+strings.ToLower(later)+".example.com")

	// Campaign A: $120 spent, $50 back (ROI -58%). Campaign B: no spend on
	// file — neither spend nor ROI can fire for it.
	reports := fakeReports{f.org: {campaignRow(f.campaign, ptr(120.0), ptr(50.0)), campaignRow(campB, nil, ptr(10.0))}}
	now := time.Now()
	since := offercap.StartOfDay(now, "Europe/Moscow")
	caps := func(context.Context, time.Time) ([]offercap.Status, error) {
		return []offercap.Status{
			{Limit: offercap.Limit{OrganizationID: f.org, OfferID: f.offer, Cap: 3, Since: since}, Count: 3},
			{Limit: offercap.Limit{OrganizationID: f.org, OfferID: idgen.New(), Cap: 3, Since: since}, Count: 2},
		}, nil
	}
	e := &Evaluator{Repo: repo, Reports: reports, Caps: caps, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	for range 2 {
		if _, err := e.Evaluate(ctx, now); err != nil {
			t.Fatal(err)
		}
	}

	got := fired(t, pool, repo, f.org)
	day := now.In(since.Location()).Format(time.DateOnly)
	kinds := map[Kind]int{}
	for _, r := range got {
		kinds[Kind(r.kind)]++
		if !strings.Contains(r.key, day) && (r.kind == string(KindSpendOver) || r.kind == string(KindROIBelow) || r.kind == string(KindOfferCapped)) {
			t.Errorf("key %q is not for the organization's day %s", r.key, day)
		}
	}
	want := map[Kind]int{KindSpendOver: 1, KindROIBelow: 1, KindOfferCapped: 1, KindOfferPaused: 1, KindSSLExpiring: 1}
	for k, n := range want {
		if kinds[k] != n {
			t.Errorf("%s fired %d times, want %d (all: %+v)", k, kinds[k], n, got)
		}
	}
	for _, r := range got {
		switch Kind(r.kind) {
		case KindSpendOver:
			if r.slack != "pending" || !strings.Contains(r.params, `"campaignName": "Camp A"`) || !strings.Contains(r.params, `"spend": 120`) {
				t.Errorf("spend notification = %+v", r)
			}
		case KindROIBelow:
			if r.slack != "none" || !strings.Contains(r.params, `"roi": -58.33`) {
				t.Errorf("roi notification = %+v", r)
			}
		case KindOfferCapped, KindOfferPaused:
			if !strings.Contains(r.params, `"offerName": "Offer One"`) {
				t.Errorf("offer notification = %+v", r)
			}
		case KindSSLExpiring:
			if !strings.Contains(r.params, `"daysLeft": 5`) || !strings.Contains(r.key, soon) {
				t.Errorf("ssl notification = %+v", r)
			}
		}
	}

	// Pausing again later is a new pause and fires again.
	exec(t, pool, `UPDATE offers SET status = 'active' WHERE id = $1`, f.offer)
	exec(t, pool, `UPDATE offers SET status = 'paused' WHERE id = $1`, f.offer)
	if _, err := e.Evaluate(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range fired(t, pool, repo, f.org) {
		if r.kind == string(KindOfferPaused) {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("offer_paused fired %d times after a second pause, want 2", n)
	}
	// Editing a paused offer is not a pause.
	exec(t, pool, `UPDATE offers SET name = 'Renamed' WHERE id = $1`, f.offer)
	if _, err := e.Evaluate(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := len(fired(t, pool, repo, f.org)); got != 6 {
		t.Fatalf("%d notifications after renaming a paused offer, want 6", got)
	}
}

func TestPausesBeforeTheRuleDoNotFire(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	f := seed(t, pool, "UTC")
	repo := privateRepo(t, pool)
	exec(t, pool, `UPDATE offers SET status = 'paused' WHERE id = $1`, f.offer)
	// The rule is switched on (re-created) after the pause.
	exec(t, pool, `DELETE FROM alert_rules WHERE organization_id = $1 AND kind = 'offer_paused'`, f.org)
	if _, err := NewService(repo).CreateRule(ctx, f.org, RuleInput{Kind: KindOfferPaused, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	e := &Evaluator{Repo: repo, Reports: fakeReports{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if _, err := e.Evaluate(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := fired(t, pool, repo, f.org); len(got) != 0 {
		t.Fatalf("fired %+v for a pause older than the rule", got)
	}
}

type fakeSlack struct {
	errs map[string]error // by organization
	sent []string
}

func (s *fakeSlack) PostSlack(_ context.Context, orgID, text string) error {
	if err := s.errs[orgID]; err != nil {
		return err
	}
	s.sent = append(s.sent, text)
	return nil
}

func TestDeliverSlack(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	ok, notConnected, flaky := seed(t, pool, "UTC"), seed(t, pool, "UTC"), seed(t, pool, "UTC")
	repo := privateRepo(t, pool)
	slackRule := Rule{ID: "", Kind: KindOfferPaused, NotifySlack: true}
	for _, org := range []string{ok.org, notConnected.org, flaky.org} {
		rules, _ := repo.ListRules(ctx, org)
		slackRule.ID = rules[0].ID
		if _, err := repo.fire(ctx, idgen.New(), org, slackRule, "k", map[string]any{"offerName": "Offer One"}); err != nil {
			t.Fatal(err)
		}
	}
	// A notification without Slack is never claimed.
	if _, err := repo.fire(ctx, idgen.New(), ok.org, Rule{ID: slackRule.ID, Kind: KindOfferPaused}, "quiet", map[string]any{}); err != nil {
		t.Fatal(err)
	}

	slack := &fakeSlack{errs: map[string]error{notConnected.org: ErrSlackNotConnected, flaky.org: errors.New("503")}}
	e := &Evaluator{Repo: repo, Slack: slack, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	now := time.Now()
	if n, err := e.DeliverSlack(ctx, now); err != nil || n != 3 {
		t.Fatalf("DeliverSlack = %d, %v; want 3 claimed", n, err)
	}
	if len(slack.sent) != 1 || !strings.Contains(slack.sent[0], "Offer One") {
		t.Fatalf("sent %q", slack.sent)
	}
	status := func(org, key string) (string, int, string) {
		var s, e string
		var n int
		if err := pool.QueryRow(ctx, `SELECT slack_status, slack_attempts, slack_error FROM `+repo.notifications+` WHERE organization_id = $1 AND dedup_key = $2`, org, key).Scan(&s, &n, &e); err != nil {
			t.Fatal(err)
		}
		return s, n, e
	}
	if s, _, _ := status(ok.org, "k"); s != "sent" {
		t.Errorf("connected org: %s, want sent", s)
	}
	if s, _, e := status(notConnected.org, "k"); s != "failed" || e == "" {
		t.Errorf("org without slack: %s %q, want failed with the reason", s, e)
	}
	if s, n, _ := status(flaky.org, "k"); s != "pending" || n != 1 {
		t.Errorf("flaky org: %s after %d attempts, want pending for a retry", s, n)
	}
	if s, _, _ := status(ok.org, "quiet"); s != "none" {
		t.Errorf("notification without slack: %s", s)
	}
	// Not due yet: nothing is claimed.
	if n, _ := e.DeliverSlack(ctx, now); n != 0 {
		t.Fatalf("claimed %d before the retry is due", n)
	}
	// The last attempt fails for good.
	exec(t, pool, `UPDATE `+repo.notifications+` SET slack_next_at = now(), slack_attempts = $2 WHERE organization_id = $1`, flaky.org, slackMaxAttempts-1)
	if _, err := e.DeliverSlack(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if s, n, _ := status(flaky.org, "k"); s != "failed" || n != slackMaxAttempts {
		t.Errorf("flaky org after %d attempts: %s, want failed", n, s)
	}
}

func TestCampaignConditionNeedsSpend(t *testing.T) {
	spend := Rule{Kind: KindSpendOver, Threshold: ptr(10.0)}
	roi := Rule{Kind: KindROIBelow, Threshold: ptr(0.0)}
	noCost := analytics.MetricValues{Revenue: ptr(0.0)}
	if _, ok := campaignCondition(spend, noCost); ok {
		t.Error("spend fired without cost on file")
	}
	if _, ok := campaignCondition(roi, noCost); ok {
		t.Error("ROI fired without cost on file")
	}
	zero := analytics.MetricValues{Cost: ptr(0.0), Revenue: ptr(0.0)}
	if _, ok := campaignCondition(roi, zero); ok {
		t.Error("ROI fired on zero spend")
	}
	if _, ok := campaignCondition(spend, analytics.MetricValues{Cost: ptr(10.0)}); ok {
		t.Error("spend equal to the threshold fired; it must be over it")
	}
}

func TestSlackText(t *testing.T) {
	for kind, want := range map[Kind]string{
		KindSpendOver:   "Spend on *Camp* today is $1500000.5, over the $1000000 alert threshold.",
		KindROIBelow:    "ROI of *Camp* today is -12.5%, below 0% (spend $10, revenue $8.75).",
		KindOfferCapped: "Offer *Off* reached its daily cap (3 of 3)",
		KindOfferPaused: "Offer *Off* was paused",
		KindSSLExpiring: "The SSL certificate of *a.example.com* expires in 5 days (2026-10-13).",
	} {
		p := map[string]any{"campaignName": "Camp", "spend": 1500000.5, "threshold": 1000000.0, "offerName": "Off", "count": 3.0, "cap": 3.0,
			"domain": "a.example.com", "daysLeft": 5.0, "expiresAt": "2026-10-13T10:00:00Z", "expired": false}
		if kind == KindROIBelow {
			p["roi"], p["threshold"], p["spend"], p["revenue"] = -12.5, 0.0, 10.0, 8.75
		}
		if got := SlackText(kind, p); !strings.Contains(got, want) {
			t.Errorf("%s: %q does not contain %q", kind, got, want)
		}
	}
	if got := SlackText(KindSSLExpiring, map[string]any{"domain": "a.example.com", "expired": true}); !strings.Contains(got, "has expired") {
		t.Errorf("expired: %q", got)
	}
}
