package reconcile_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/reconcile"
)

func TestCompareFindsLostDuplicatedAndMisSummedConversions(t *testing.T) {
	dsn, chURL := os.Getenv("DATABASE_URL"), os.Getenv("CLICKHOUSE_URL")
	if dsn == "" || chURL == "" {
		t.Skip("needs DATABASE_URL and CLICKHOUSE_URL")
	}
	ctx := context.Background()
	pg, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	t.Cleanup(pg.Close) // not defer: the org cleanup below runs in t.Cleanup and needs the pool
	ch, err := chconn.NewConn(ctx, config.ClickHouseConfig{URL: chURL, Database: "flox", User: "flox"})
	if err != nil {
		t.Fatalf("clickhouse: %v", err)
	}
	t.Cleanup(func() { _ = ch.Close() })
	if err := chstore.Migrate(ctx, ch); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	orgID, networkID := idgen.New(), idgen.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pg.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO organizations (id, name) VALUES ($1, 'reconcile test')`, orgID)
	t.Cleanup(func() { _, _ = pg.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID) })
	exec(`INSERT INTO networks (id, organization_id, name, accept_duplicates) VALUES ($1, $2, 'n', true)`, networkID, orgID)

	day := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	// Recorded in Postgres (the source of truth)...
	postback := func(status string, at time.Time, usd, reversal *float64) {
		t.Helper()
		exec(`INSERT INTO postbacks (id, organization_id, network_id, click_id, status, direction, result,
		        network_accepts_duplicates, usd_value, reversal_usd_value, event_at, created_at)
		      VALUES ($1, $2, $3, $4, $5, 'incoming', 'success', true, $6, $7, $8::timestamptz, $8::timestamptz + interval '5 ms')`,
			idgen.New(), orgID, networkID, idgen.New(), status, usd, reversal, at)
	}
	// ...and copied (or not) into ClickHouse.
	var chEvents []event.Event
	copyToCH := func(status event.Type, at time.Time, usd float64) {
		chEvents = append(chEvents, event.Event{Type: status, EventAt: at, OrganizationID: orgID, ClickID: idgen.New(),
			USDValue: usd, HasUSDValue: usd != 0})
	}
	usd := func(v float64) *float64 { return &v }

	// Matching accept, and a chargeback (reversal 25 → -25 in ClickHouse).
	postback("CPA_ACCEPT", day.Add(10*time.Hour), usd(25), nil)
	copyToCH(event.CpaAccept, day.Add(10*time.Hour), 25)
	postback("CPA_DECLINE", day.Add(11*time.Hour), nil, usd(25))
	copyToCH(event.CpaDecline, day.Add(11*time.Hour), -25)
	// Received at 23:59:59.998, written 5 ms later — the next day by
	// created_at. Bucketed by event_at, both sides agree.
	postback("CPA_HOLD", day.Add(24*time.Hour-2*time.Millisecond), nil, nil)
	copyToCH(event.CpaHold, day.Add(24*time.Hour-2*time.Millisecond), 0)
	// Lost: a redeposit that never reached ClickHouse.
	postback("CPA_REDEP", day.Add(24*time.Hour+time.Hour), usd(10), nil)
	// Duplicated: one trash recorded, two in ClickHouse.
	postback("CPA_TRASH", day.Add(24*time.Hour+2*time.Hour), nil, nil)
	copyToCH(event.CpaTrash, day.Add(24*time.Hour+2*time.Hour), 0)
	copyToCH(event.CpaTrash, day.Add(24*time.Hour+2*time.Hour), 0)
	// Mis-summed: same count, different USD.
	postback("CPA_ACCEPT", day.Add(48*time.Hour), usd(30), nil)
	copyToCH(event.CpaAccept, day.Add(48*time.Hour), 3)

	if err := chstore.NewEventStore(ch).InsertBatch(chstore.WithBatchID(ctx, idgen.New()), chEvents); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	r := reconcile.New(pg, ch, slog.New(slog.NewTextHandler(io.Discard, nil)))
	all, err := r.Compare(ctx, day, day.Add(72*time.Hour))
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	got := map[string]reconcile.Mismatch{}
	for _, m := range all {
		if m.OrganizationID == orgID {
			got[m.Day+" "+m.Status] = m
		}
	}
	want := map[string][2]reconcile.Totals{
		"2026-09-11 CPA_REDEP":  {{Count: 1, USD: 10}, {}},
		"2026-09-11 CPA_TRASH":  {{Count: 1}, {Count: 2}},
		"2026-09-12 CPA_ACCEPT": {{Count: 1, USD: 30}, {Count: 1, USD: 3}},
	}
	if len(got) != len(want) {
		t.Fatalf("mismatches = %v, want exactly the lost redep, the duplicated trash and the mis-summed accept", got)
	}
	for k, w := range want {
		m, ok := got[k]
		if !ok || m.Postgres != w[0] || m.ClickHouse != w[1] {
			t.Errorf("%s: %+v, want postgres %+v clickhouse %+v", k, m, w[0], w[1])
		}
	}
}
