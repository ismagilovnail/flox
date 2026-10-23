package retention_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/retention"
)

// Audit DATA-6: raw clicks are kept as long as the organization's plan
// says. A shorter plan applies only after the waiting period, a longer one
// at once; either way the organization's rows in all three tables are
// rewritten once every worker writes the new value, new rows carry it,
// and the TTL then deletes what is older — click_lookup excepted, which
// keeps a click 13 months whatever the plan.
func TestRawRetentionFollowsThePlan(t *testing.T) {
	pg, ch := pgAndCH(t)
	ctx := context.Background()
	orgID := idgen.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pg.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO organizations (id, name) VALUES ($1, 'raw retention test')`, orgID)
	t.Cleanup(func() { _, _ = pg.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID) })

	plans := &retention.Plans{DB: pg, CH: ch, ShortenAfter: time.Hour, Only: orgID,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	store := chstore.NewEventStore(ch).WithRetention(plans.Of)
	click := func(at time.Time) {
		t.Helper()
		id := idgen.New()
		if err := store.InsertBatch(ctx, []event.Event{
			{ID: idgen.New(), Type: event.SourceClick, EventAt: at, OrganizationID: orgID, ClickID: id},
			{ID: idgen.New(), Type: event.LandView, EventAt: at, OrganizationID: orgID, ClickID: id},
		}); err != nil {
			t.Fatal(err)
		}
	}
	state := func() (months, target int, applied bool) {
		t.Helper()
		if err := pg.QueryRow(ctx, `SELECT months, target, applied_at IS NOT NULL FROM organization_retention WHERE organization_id = $1`,
			orgID).Scan(&months, &target, &applied); err != nil {
			t.Fatal(err)
		}
		return
	}
	// wantRows: the retention every row of the organization carries in each
	// table.
	wantRows := func(step string, want uint8) {
		t.Helper()
		for _, table := range []string{"click_events", "tracking_events", "click_lookup"} {
			var bad, all uint64
			if err := ch.QueryRow(ctx, fmt.Sprintf(`SELECT countIf(retention_months != ?), count() FROM %s WHERE organization_id = ?`, table),
				want, orgID).Scan(&bad, &all); err != nil {
				t.Fatal(err)
			}
			if bad != 0 || all == 0 {
				t.Errorf("%s: %s has %d of %d rows not at %d months", step, table, bad, all, want)
			}
		}
	}
	run := func() {
		t.Helper()
		if err := plans.Run(ctx); err != nil {
			t.Fatalf("Run: %v", err)
		}
	}
	// The time a step waits, in the past.
	backdate := func(column string, d time.Duration) {
		t.Helper()
		exec(`UPDATE organization_retention SET `+column+` = now() - make_interval(secs => $2) WHERE organization_id = $1`, orgID, d.Seconds())
	}

	now := time.Now().UTC()
	click(now) // before any pass: the column default, 0

	// First pass: a Free organization starts at 24 months, what its rows
	// mean already; the plan's 3 waits.
	run()
	if m, target, applied := state(); m != 24 || target != 3 || !applied {
		t.Fatalf("first pass: months %d, target %d, applied %v; want 24, 3, applied", m, target, applied)
	}
	if err := plans.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if got := plans.Of(orgID); got != 24 {
		t.Fatalf("Of = %d, want 24", got)
	}

	// Still within the waiting period: nothing changes.
	run()
	if m, _, _ := state(); m != 24 {
		t.Fatalf("within the wait: months %d, want 24", m)
	}

	// The wait is over: new rows get 3 at once, old rows once the caches
	// have it.
	backdate("target_since", 2*time.Hour)
	run()
	if m, _, applied := state(); m != 3 || applied {
		t.Fatalf("after the wait: months %d, applied %v; want 3, not yet applied", m, applied)
	}
	if got := plans.Of(orgID); got != 3 {
		t.Fatalf("Of = %d, want 3", got)
	}
	click(now.Add(time.Minute))
	backdate("updated_at", time.Hour)
	run()
	if _, _, applied := state(); !applied {
		t.Fatal("rows not rewritten after the settle time")
	}
	wantRows("downgraded", 3)

	// An upgrade applies at once, without waiting.
	exec(`INSERT INTO organization_plans (id, organization_id, plan_code) VALUES ($1, $2, 'business')`, idgen.New(), orgID)
	run()
	if m, target, _ := state(); m != 24 || target != 24 {
		t.Fatalf("upgrade: months %d, target %d; want 24 at once", m, target)
	}
	backdate("updated_at", time.Hour)
	run()
	wantRows("upgraded", 24)
}

// The TTL deletes a raw click past its organization's retention at the
// next merge — the rows, not only some columns — while click_lookup keeps
// it for late postbacks; a row without a retention counts as 24 months.
func TestRawRetentionTTLDeletesOldRows(t *testing.T) {
	_, ch := pgAndCH(t)
	ctx := context.Background()
	orgID := idgen.New()
	at := time.Now().UTC().AddDate(0, -4, 0)
	short, kept := idgen.New(), idgen.New()
	for _, c := range []struct {
		id     string
		months uint8
	}{{short, 3}, {kept, 0}} {
		store := chstore.NewEventStore(ch).WithRetention(func(string) uint8 { return c.months })
		if err := store.InsertBatch(ctx, []event.Event{{ID: idgen.New(), Type: event.SourceClick, EventAt: at, OrganizationID: orgID, ClickID: c.id}}); err != nil {
			t.Fatal(err)
		}
	}
	partition := at.Format("200601")
	for _, table := range []string{"click_events", "click_lookup"} {
		if err := ch.Exec(ctx, fmt.Sprintf(`OPTIMIZE TABLE %s PARTITION %s FINAL`, table, partition)); err != nil {
			t.Fatal(err)
		}
	}
	has := func(table, clickID string) bool {
		t.Helper()
		var n uint64
		if err := ch.QueryRow(ctx, `SELECT count() FROM `+table+` WHERE organization_id = ? AND click_id = ?`, orgID, clickID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n > 0
	}
	if has("click_events", short) {
		t.Error("a 4-month-old click of a 3-month plan is still in click_events")
	}
	if !has("click_events", kept) {
		t.Error("a 4-month-old click without a retention (24 months) was deleted")
	}
	if !has("click_lookup", short) {
		t.Error("click_lookup lost a 4-month-old click: late postbacks would not find it")
	}
}

func pgAndCH(t *testing.T) (*pgxpool.Pool, driver.Conn) {
	t.Helper()
	dsn, chURL := os.Getenv("DATABASE_URL"), os.Getenv("CLICKHOUSE_URL")
	if dsn == "" || chURL == "" {
		t.Skip("needs DATABASE_URL and CLICKHOUSE_URL")
	}
	ctx := context.Background()
	pg, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)
	ch, err := chconn.NewConn(ctx, config.ClickHouseConfig{URL: chURL, Database: envOr("CLICKHOUSE_DATABASE", "flox"),
		User: envOr("CLICKHOUSE_USER", "flox"), Password: os.Getenv("CLICKHOUSE_PASSWORD")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ch.Close() })
	if err := chstore.Migrate(ctx, ch); err != nil {
		t.Fatal(err)
	}
	return pg, ch
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
