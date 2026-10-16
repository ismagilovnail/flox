package mappingreplay_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/mappingreplay"
)

type usdOnly struct{}

func (usdOnly) ToUSD(_ context.Context, currency string, amount float64, _ time.Time) (float64, bool, error) {
	return amount, currency == "USD", nil
}

type noAttribution struct{}

func (noAttribution) AttributeConversion(context.Context, attribution.Conversion) (attribution.Attribution, error) {
	return attribution.Attribution{Outcome: attribution.OutcomeUnknownClick}, nil
}

type noAttempts struct{}

func (noAttempts) LogAttempt(context.Context, conversion.AttemptRecord) {}

func mustPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("needs DATABASE_URL")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedOrgNetwork(t *testing.T, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	ctx := context.Background()
	orgID, networkID := idgen.New(), idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, 'mapping replay test')`, orgID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID) })
	if _, err := pool.Exec(ctx, `INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, 'n')`, networkID, orgID); err != nil {
		t.Fatal(err)
	}
	return orgID, networkID
}

// A postback that arrived before its status was mapped is recorded once it
// is — with the conversion's own date, HOLD before the ACCEPT that followed
// it — and only once (audit 2026-10-15, DATA-4).
func TestWaitingPostbacksAreRecordedOnceMapped(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, networkID := seedOrgNetwork(t, pool)
	svc := conversion.NewService(conversion.NewPostgresMapper(pool), conversion.NewPostgresStore(pool),
		usdOnly{}, noAttribution{}, noAttempts{})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	replayer := mappingreplay.New(pool, svc, logger).ForOrganization(orgID)

	clickID := idgen.New()
	holdAt := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Microsecond)
	acceptAt := holdAt.Add(time.Hour)
	amount := 12.5
	// The ACCEPT arrives first in the ledger: the replay must still go by
	// the conversions' own times.
	for _, p := range []conversion.Postback{
		{OrganizationID: orgID, NetworkID: networkID, ClickID: clickID, RawStatus: "Approved", Revenue: &amount, Currency: "USD", OccurredAt: acceptAt},
		{OrganizationID: orgID, NetworkID: networkID, ClickID: clickID, RawStatus: "pending", OccurredAt: holdAt},
	} {
		res, err := svc.Record(ctx, p)
		if err != nil || res.Kind != conversion.ResultError {
			t.Fatalf("Record(%s) = %v, %v; want error (unmapped)", p.RawStatus, res.Kind, err)
		}
	}

	waiting, err := mappingreplay.WaitingStatuses(ctx, pool, orgID)
	if err != nil || len(waiting) != 2 || waiting[0].NetworkID != networkID {
		t.Fatalf("WaitingStatuses = %+v, %v; want two statuses of the network", waiting, err)
	}
	// Another organization sees none of them (CLAUDE.md #5).
	otherOrg, _ := seedOrgNetwork(t, pool)
	if other, err := mappingreplay.WaitingStatuses(ctx, pool, otherOrg); err != nil || len(other) != 0 {
		t.Fatalf("WaitingStatuses from another organization = %+v, %v; want none", other, err)
	}

	if n, err := replayer.RunOnce(ctx, 10); err != nil || n != 0 {
		t.Fatalf("RunOnce before mapping = %d, %v; want 0", n, err)
	}

	for raw, status := range map[string]event.Type{"approved": event.CpaAccept, "pending": event.CpaHold} {
		if _, err := pool.Exec(ctx, `INSERT INTO event_mappings (id, organization_id, network_id, network_status, flox_status) VALUES ($1, $2, $3, $4, $5)`,
			idgen.New(), orgID, networkID, raw, string(status)); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := replayer.RunOnce(ctx, 10); err != nil || n != 2 {
		t.Fatalf("RunOnce after mapping = %d, %v; want 2", n, err)
	}

	rows, err := pool.Query(ctx, `SELECT status, event_at, revenue::float8 FROM postbacks
		WHERE organization_id = $1 AND click_id = $2 AND result = 'success' ORDER BY event_at`, orgID, clickID)
	if err != nil {
		t.Fatal(err)
	}
	type got struct {
		status  string
		at      time.Time
		revenue *float64
	}
	var recorded []got
	for rows.Next() {
		var g got
		if err := rows.Scan(&g.status, &g.at, &g.revenue); err != nil {
			t.Fatal(err)
		}
		recorded = append(recorded, g)
	}
	if len(recorded) != 2 ||
		recorded[0].status != string(event.CpaHold) || !recorded[0].at.Equal(holdAt) ||
		recorded[1].status != string(event.CpaAccept) || !recorded[1].at.Equal(acceptAt) ||
		recorded[1].revenue == nil || *recorded[1].revenue != amount {
		t.Fatalf("recorded = %+v; want HOLD at %v then ACCEPT 12.5 at %v", recorded, holdAt, acceptAt)
	}

	var left, replayed int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE awaiting_mapping), count(*) FILTER (WHERE replayed_at IS NOT NULL)
		FROM postbacks WHERE organization_id = $1 AND result = 'error'`, orgID).Scan(&left, &replayed); err != nil {
		t.Fatal(err)
	}
	if left != 0 || replayed != 2 {
		t.Fatalf("after replay: %d still awaiting, %d replayed; want 0 and 2", left, replayed)
	}
	if n, err := replayer.RunOnce(ctx, 10); err != nil || n != 0 {
		t.Fatalf("second RunOnce = %d, %v; want 0", n, err)
	}
	if waiting, err := mappingreplay.WaitingStatuses(ctx, pool, orgID); err != nil || len(waiting) != 0 {
		t.Fatalf("WaitingStatuses after replay = %+v, %v; want none", waiting, err)
	}
}

// One older than conversion.MappingReplayWindow stays refused.
func TestWaitingPostbacksExpire(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, networkID := seedOrgNetwork(t, pool)
	if _, err := pool.Exec(ctx, `INSERT INTO postbacks (id, organization_id, network_id, click_id, status, direction, result, raw_status, awaiting_mapping, created_at)
		VALUES ($1, $2, $3, $4, '', 'incoming', 'error', 'approved', true, now() - make_interval(secs => $5))`,
		idgen.New(), orgID, networkID, idgen.New(), (conversion.MappingReplayWindow + time.Hour).Seconds()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO event_mappings (id, organization_id, network_id, network_status, flox_status) VALUES ($1, $2, $3, 'approved', $4)`,
		idgen.New(), orgID, networkID, string(event.CpaAccept)); err != nil {
		t.Fatal(err)
	}
	svc := conversion.NewService(conversion.NewPostgresMapper(pool), conversion.NewPostgresStore(pool), usdOnly{}, noAttribution{}, noAttempts{})
	replayer := mappingreplay.New(pool, svc, slog.New(slog.NewTextHandler(io.Discard, nil))).ForOrganization(orgID)
	if n, err := replayer.RunOnce(ctx, 10); err != nil || n != 0 {
		t.Fatalf("RunOnce = %d, %v; want 0 for a postback past the window", n, err)
	}
}
