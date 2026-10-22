package billing_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/billing"
	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func ptr[T any](v T) *T { return &v }

type fakeStore struct {
	plan      billing.Plan
	counts    billing.Counts
	paidUntil *time.Time
}

func (f fakeStore) Plans(context.Context) ([]billing.Plan, error) { return []billing.Plan{f.plan}, nil }
func (f fakeStore) Subscription(context.Context, string) (billing.Subscription, error) {
	return billing.Subscription{Plan: f.plan, Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), PaidUntil: f.paidUntil}, nil
}
func (f fakeStore) Counts(context.Context, string) (billing.Counts, error) { return f.counts, nil }

type fakeUsage struct {
	clicks int64
	err    error
	got    billing.Period
}

func (f *fakeUsage) Clicks(_ context.Context, _ string, p billing.Period) (int64, error) {
	f.got = p
	return f.clicks, f.err
}

var start = billing.Plan{Code: "start", Name: "Start", MonthlyClicks: ptr(int64(1000)), MaxCampaigns: ptr(2), MaxDomains: ptr(1), MaxMembers: ptr(3)}

func isPlanLimit(err error) bool {
	var e *apierror.Error
	return errors.As(err, &e) && e.Status == http.StatusPaymentRequired && e.Code == "plan_limit"
}

func TestLevels(t *testing.T) {
	for clicks, want := range map[int64]billing.Level{
		0: billing.LevelOK, 799: billing.LevelOK, 800: billing.LevelWarning, 999: billing.LevelWarning,
		1000: billing.LevelExceeded, 1099: billing.LevelExceeded, 1100: billing.LevelBlocked, 50000: billing.LevelBlocked,
	} {
		if got := billing.LevelFor(start, ptr(clicks)); got != want {
			t.Errorf("%d of 1000 clicks = %s, want %s", clicks, got, want)
		}
	}
	if got := billing.LevelFor(start, nil); got != billing.LevelUnknown {
		t.Errorf("unknown clicks = %s, want unknown", got)
	}
	if got := billing.LevelFor(billing.Plan{}, nil); got != billing.LevelOK {
		t.Errorf("unlimited plan = %s, want ok", got)
	}
}

func TestPeriodIsTheUTCCalendarMonth(t *testing.T) {
	// 23:30 on the 31st in UTC-5 is already the next month in UTC.
	p := billing.PeriodAt(time.Date(2026, 10, 31, 23, 30, 0, 0, time.FixedZone("EST", -5*3600)))
	if !p.From.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) || !p.To.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("period = %v – %v, want November", p.From, p.To)
	}
	p = billing.PeriodAt(time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC))
	if !p.To.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("December ends %v, want 2027-01-01", p.To)
	}
}

func TestStatus(t *testing.T) {
	usage := &fakeUsage{clicks: 850}
	now := time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)
	svc := billing.NewService(fakeStore{plan: start, counts: billing.Counts{Campaigns: 1, Domains: 1, Members: 2}}, usage, quiet).
		WithClock(func() time.Time { return now })
	st, err := svc.Status(context.Background(), "org")
	if err != nil {
		t.Fatal(err)
	}
	if st.Period.From != "2026-02-01" || st.Period.To != "2026-02-28" {
		t.Errorf("period = %s – %s, want 2026-02-01 – 2026-02-28", st.Period.From, st.Period.To)
	}
	if !usage.got.From.Equal(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)) || !usage.got.To.Equal(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("clicks read for %v – %v, want February", usage.got.From, usage.got.To)
	}
	if st.Usage.Clicks == nil || *st.Usage.Clicks != 850 || st.Level != billing.LevelWarning || st.Usage.Members != 2 {
		t.Errorf("status = %+v, want 850 clicks, warning, 2 members", st)
	}

	// ClickHouse down: clicks unknown, not zero.
	st, err = billing.NewService(fakeStore{plan: start}, &fakeUsage{err: errors.New("down")}, quiet).Status(context.Background(), "org")
	if err != nil {
		t.Fatal(err)
	}
	if st.Usage.Clicks != nil || st.Level != billing.LevelUnknown {
		t.Errorf("status without clicks = %+v, want null clicks and unknown", st)
	}
	if _, err := billing.NewService(fakeStore{plan: start}, nil, quiet).Status(context.Background(), ""); err == nil {
		t.Error("Status without an organization succeeded")
	}
}

func TestAllowCountLimits(t *testing.T) {
	ctx := context.Background()
	full := billing.Counts{Campaigns: 2, Domains: 1, Members: 3}
	svc := billing.NewService(fakeStore{plan: start, counts: full}, &fakeUsage{}, quiet)
	for _, r := range []billing.Resource{billing.ResourceCampaign, billing.ResourceDomain, billing.ResourceMember} {
		if err := svc.Allow(ctx, "org", r); !isPlanLimit(err) {
			t.Errorf("%s at the limit = %v, want plan_limit", r, err)
		}
	}
	// Stream sets have no count limit.
	if err := svc.Allow(ctx, "org", billing.ResourceStreamSet); err != nil {
		t.Errorf("stream set = %v, want allowed", err)
	}

	svc = billing.NewService(fakeStore{plan: start, counts: billing.Counts{Campaigns: 1, Members: 2}}, &fakeUsage{}, quiet)
	for _, r := range []billing.Resource{billing.ResourceCampaign, billing.ResourceDomain, billing.ResourceMember} {
		if err := svc.Allow(ctx, "org", r); err != nil {
			t.Errorf("%s under the limit = %v, want allowed", r, err)
		}
	}

	unlimited := billing.Plan{Code: "business", Name: "Business"}
	svc = billing.NewService(fakeStore{plan: unlimited, counts: billing.Counts{Campaigns: 10000, Domains: 500, Members: 300}}, nil, quiet)
	for _, r := range []billing.Resource{billing.ResourceCampaign, billing.ResourceStreamSet, billing.ResourceDomain, billing.ResourceMember} {
		if err := svc.Allow(ctx, "org", r); err != nil {
			t.Errorf("%s on an unlimited plan = %v, want allowed", r, err)
		}
	}
	if err := svc.Allow(ctx, "org", "pixel"); err == nil || isPlanLimit(err) {
		t.Errorf("unknown resource = %v, want an internal error", err)
	}
}

func TestAllowOverTheClicks(t *testing.T) {
	ctx := context.Background()
	for clicks, blocked := range map[int64]bool{999: false, 1000: false, 1099: false, 1100: true, 5000: true} {
		svc := billing.NewService(fakeStore{plan: start}, &fakeUsage{clicks: clicks}, quiet)
		for _, r := range []billing.Resource{billing.ResourceCampaign, billing.ResourceStreamSet} {
			err := svc.Allow(ctx, "org", r)
			if blocked != isPlanLimit(err) || (!blocked && err != nil) {
				t.Errorf("%s at %d of 1000 clicks = %v, want blocked %v", r, clicks, err, blocked)
			}
		}
		// Clicks never stop adding a domain or a member.
		for _, r := range []billing.Resource{billing.ResourceDomain, billing.ResourceMember} {
			if err := svc.Allow(ctx, "org", r); err != nil {
				t.Errorf("%s at %d clicks = %v, want allowed", r, clicks, err)
			}
		}
	}
	// Unknown clicks never block.
	for _, usage := range []billing.UsageReader{nil, &fakeUsage{err: errors.New("down")}} {
		if err := billing.NewService(fakeStore{plan: start}, usage, quiet).Allow(ctx, "org", billing.ResourceCampaign); err != nil {
			t.Errorf("campaign with unknown clicks = %v, want allowed", err)
		}
	}
}

func mustPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedOrg(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := idgen.New()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, id, "Billing Test "+id); err != nil {
		t.Fatalf("seeding organization: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, id) })
	return id
}

func TestRepository(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	repo := billing.NewRepository(pool)
	orgA, orgB := seedOrg(t, pool), seedOrg(t, pool)

	plans, err := repo.Plans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, p := range plans {
		codes = append(codes, p.Code)
	}
	if len(codes) < 4 || codes[0] != "free" {
		t.Fatalf("plans = %v, want free first", codes)
	}

	// No row: Free since the organization was created.
	p, _, err := repo.Current(ctx, orgA)
	if err != nil || p.Code != billing.FreePlan || p.MonthlyClicks == nil {
		t.Fatalf("current = %+v, %v; want free with a click limit", p, err)
	}
	if _, _, err := repo.Current(ctx, idgen.New()); err == nil {
		t.Error("Current of a missing organization succeeded")
	}

	if err := repo.SetPlan(ctx, orgA, "pro", "trial deal"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetPlan(ctx, orgA, "pro", ""); !isStatus(err, http.StatusConflict) {
		t.Errorf("same plan again = %v, want 409", err)
	}
	if err := repo.SetPlan(ctx, orgA, "nonexistent", ""); !isStatus(err, http.StatusUnprocessableEntity) {
		t.Errorf("unknown plan = %v, want 422", err)
	}
	if err := repo.SetPlan(ctx, orgA, "business", ""); err != nil {
		t.Fatal(err)
	}
	if p, _, _ := repo.Current(ctx, orgA); p.Code != "business" || p.MaxCampaigns != nil {
		t.Errorf("current after two changes = %+v, want business, unlimited campaigns", p)
	}
	history, err := repo.History(ctx, orgA)
	if err != nil || len(history) != 2 || history[0].PlanCode != "business" || history[0].EndedAt != nil ||
		history[1].PlanCode != "pro" || history[1].EndedAt == nil || history[1].Note != "trial deal" {
		t.Errorf("history = %+v, %v", history, err)
	}
	var audited int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE organization_id = $1 AND action = 'billing.plan_changed'`, orgA).Scan(&audited); err != nil || audited != 2 {
		t.Errorf("audit rows = %d, %v; want 2", audited, err)
	}

	// The other organization is untouched and counts only its own rows.
	if p, _, _ := repo.Current(ctx, orgB); p.Code != billing.FreePlan {
		t.Errorf("org B plan = %s, want free", p.Code)
	}
	if h, _ := repo.History(ctx, orgB); len(h) != 0 {
		t.Errorf("org B history = %+v, want none", h)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO traffic_sources (id, organization_id, name, type) VALUES ($1, $2, 'S', 'facebook')`, idgen.New(), orgA); err != nil {
		t.Fatalf("seeding a traffic source: %v", err)
	}
	var sourceID string
	_ = pool.QueryRow(ctx, `SELECT id FROM traffic_sources WHERE organization_id = $1`, orgA).Scan(&sourceID)
	for _, status := range []string{"active", "draft", "archived"} {
		if _, err := pool.Exec(ctx, `INSERT INTO campaigns (id, organization_id, traffic_source_id, name, status, fallback_url)
			VALUES ($1, $2, $3, 'C', $4, 'https://example.com')`, idgen.New(), orgA, sourceID, status); err != nil {
			t.Fatalf("seeding a campaign: %v", err)
		}
	}
	a, err := repo.Counts(ctx, orgA)
	if err != nil || a.Campaigns != 2 {
		t.Errorf("org A counts = %+v, %v; want 2 campaigns (archived not counted)", a, err)
	}
	if b, err := repo.Counts(ctx, orgB); err != nil || b != (billing.Counts{}) {
		t.Errorf("org B counts = %+v, %v; want zero", b, err)
	}
}

// The plan's share of ClickHouse (audit CH-1, migration 00096): Free's
// for an organization without a plan row, the new plan's after a change.
func TestReportLimits(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	repo := billing.NewRepository(pool)
	org := seedOrg(t, pool)

	free, err := repo.ReportLimits(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	if free.Plan != billing.FreePlan || free.MaxConcurrent != 2 || free.MaxThreads != 2 ||
		free.MaxExecution != 15*time.Second || free.MaxMemory != 1<<30 || free.Priority != 4 {
		t.Errorf("free limits = %+v", free)
	}
	if err := repo.SetPlan(ctx, org, "business", "report limits test"); err != nil {
		t.Fatal(err)
	}
	biz, err := repo.ReportLimits(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	if biz.Plan != "business" || biz.MaxConcurrent != 6 || biz.MaxThreads != 16 || biz.MaxExecution != 30*time.Second ||
		biz.MaxMemory != 4<<30 || biz.Priority != 1 {
		t.Errorf("business limits = %+v", biz)
	}
	if _, err := repo.ReportLimits(ctx, idgen.New()); err == nil {
		t.Error("limits of a missing organization were read")
	}
}

func isStatus(err error, status int) bool {
	var e *apierror.Error
	return errors.As(err, &e) && e.Status == status
}

func TestCHUsageCountsClicksAndFilteredOfTheMonth(t *testing.T) {
	url := os.Getenv("CLICKHOUSE_URL")
	if url == "" {
		t.Skip("CLICKHOUSE_URL not set, skipping integration test")
	}
	ctx := context.Background()
	conn, err := chconn.NewConn(ctx, config.ClickHouseConfig{
		URL: url, Database: envOr("CLICKHOUSE_DATABASE", "flox"), User: envOr("CLICKHOUSE_USER", "flox"),
		Password: os.Getenv("CLICKHOUSE_PASSWORD"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := chstore.Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}

	orgA, orgB := idgen.New(), idgen.New()
	t.Cleanup(func() {
		for _, table := range []string{"click_events", "click_events_daily_campaign"} {
			_ = conn.Exec(context.Background(), "DELETE FROM "+table+" WHERE organization_id IN ?", []string{orgA, orgB})
		}
	})
	insert := func(org, typ string, at time.Time) {
		t.Helper()
		if err := conn.Exec(ctx, `INSERT INTO click_events (organization_id, event_at, type, click_id, campaign_id) VALUES (?, ?, ?, ?, 'camp')`,
			org, at, typ, idgen.New()); err != nil {
			t.Fatalf("inserting a click: %v", err)
		}
	}
	oct := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	insert(orgA, "SOURCE_CLICK", oct)                                    // first moment of the month
	insert(orgA, "SOURCE_CLICK", oct.AddDate(0, 0, 15))                  // mid-month
	insert(orgA, "SOURCE_FILTER", oct.AddDate(0, 0, 20))                 // filtered counts
	insert(orgA, "SOURCE_CLICK", oct.AddDate(0, 1, 0).Add(-time.Second)) // last second of the month
	insert(orgA, "SOURCE_CLICK", oct.Add(-time.Second))                  // September
	insert(orgA, "SOURCE_CLICK", oct.AddDate(0, 1, 0))                   // November
	insert(orgB, "SOURCE_CLICK", oct.AddDate(0, 0, 3))                   // another organization

	n, err := billing.NewCHUsage(conn).Clicks(ctx, orgA, billing.PeriodAt(oct))
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("October clicks = %d, want 4", n)
	}
	if n, _ := billing.NewCHUsage(conn).Clicks(ctx, idgen.New(), billing.PeriodAt(oct)); n != 0 {
		t.Errorf("an organization without clicks = %d, want 0", n)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
