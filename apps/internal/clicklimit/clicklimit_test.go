package clicklimit_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/clicklimit"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

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

// seed is an organization with one campaign.
func seed(t *testing.T, pool *pgxpool.Pool) (orgID, campaignID string) {
	t.Helper()
	ctx := context.Background()
	orgID, campaignID = idgen.New(), idgen.New()
	tsID := idgen.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO organizations (id, name) VALUES ($1, $2)`, []any{orgID, "Limit " + orgID}},
		{`INSERT INTO traffic_sources (id, organization_id, name, type) VALUES ($1, $2, 'src', 'other')`, []any{tsID, orgID}},
		{`INSERT INTO campaigns (id, organization_id, traffic_source_id, name) VALUES ($1, $2, $3, 'c')`, []any{campaignID, orgID, tsID}},
	} {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID) })
	return orgID, campaignID
}

func clicks(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID, campaignID string) clicklimit.Summary {
	t.Helper()
	d, err := clicklimit.Read(ctx, pool, orgID, campaignID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	return d.IPLimited
}

// Counts add up across flushes and days; the warning sums the last 7 UTC
// days; another organization's campaign is not found; an organization
// deleted before the flush does not fail the others' counts.
func TestCountsAreFlushedAndRead(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID, campaignID := seed(t, pool)
	otherOrg, otherCampaign := seed(t, pool)
	gone, goneCampaign := seed(t, pool)
	now := time.Now().UTC()
	c := &clicklimit.Counter{DB: pool}

	for range 3 {
		c.Add(orgID, campaignID, now)
	}
	c.Add(orgID, campaignID, now.AddDate(0, 0, -3))
	c.Add(orgID, campaignID, now.AddDate(0, 0, -10)) // outside the window
	c.Add(otherOrg, otherCampaign, now)
	if err := c.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	c.Add(orgID, campaignID, now)
	if _, err := pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, gone); err != nil {
		t.Fatal(err)
	}
	c.Add(gone, goneCampaign, now)
	if err := c.Flush(ctx); err != nil {
		t.Fatalf("flush with a deleted organization: %v", err)
	}

	s := clicks(t, ctx, pool, orgID, campaignID)
	today := now.Truncate(24 * time.Hour)
	if s.WindowDays != 7 || s.Clicks != 5 || s.LastDay == nil || !s.LastDay.Equal(today) {
		t.Fatalf("summary = %+v, want 5 clicks in 7 days, last today", s)
	}
	if s := clicks(t, ctx, pool, otherOrg, otherCampaign); s.Clicks != 1 {
		t.Fatalf("other organization = %+v, want its own 1", s)
	}
	var apiErr *apierror.Error
	if _, err := clicklimit.Read(ctx, pool, otherOrg, campaignID); !errors.As(err, &apiErr) || apiErr.Code != "not_found" {
		t.Fatalf("another organization's campaign: err = %v, want not found", err)
	}
	if s := clicks(t, ctx, pool, otherOrg, otherCampaign); s.LastDay == nil {
		t.Fatal("other organization: no last day")
	}
}

// A failed write keeps the counts for the next flush.
func TestFailedFlushKeepsTheCounts(t *testing.T) {
	pool := mustPool(t)
	orgID, campaignID := seed(t, pool)
	c := &clicklimit.Counter{DB: pool}
	c.Add(orgID, campaignID, time.Now())
	c.Add(orgID, campaignID, time.Now())

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Flush(cancelled); err == nil {
		t.Fatal("flush with a cancelled context: want an error")
	}
	ctx := context.Background()
	if s := clicks(t, ctx, pool, orgID, campaignID); s.Clicks != 0 || s.LastDay != nil {
		t.Fatalf("after the failed flush = %+v, want nothing stored", s)
	}
	if err := c.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if s := clicks(t, ctx, pool, orgID, campaignID); s.Clicks != 2 {
		t.Fatalf("after the retry = %+v, want both clicks", s)
	}
}

// A nil Counter counts nothing and flushes nothing (a tracker test or a
// handler built without one).
func TestNilCounter(t *testing.T) {
	var c *clicklimit.Counter
	c.Add("org", "campaign", time.Now())
	if err := c.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
}
