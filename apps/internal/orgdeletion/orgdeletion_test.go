package orgdeletion_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/orgdeletion"
	"github.com/ismagilovnail/flox/apps/internal/rediscache"
	"github.com/ismagilovnail/flox/apps/internal/storage"
	"github.com/ismagilovnail/flox/apps/internal/testpg"
)

// The test needs every store the deletion touches; it skips without them
// (memory: run go test with the full env line).
func requireEnv(t *testing.T, vars ...string) {
	t.Helper()
	for _, v := range vars {
		if os.Getenv(v) == "" {
			t.Skipf("needs %s", v)
		}
	}
}

type mails struct {
	mu   sync.Mutex
	sent []string
}

func (m *mails) Send(_ context.Context, to, subject, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, to+": "+subject)
	return nil
}

const password = "correct horse battery staple 1"

type fixture struct {
	orgA, orgB             string
	ownerA, adminA, shared string
	ownerB                 string
	nameA                  string
	clickA, clickB         string
	objectsA, objectsB     []string
	redisKeyA, redisKeyB   string
}

func seed(t *testing.T, ctx context.Context, pool *pgxpool.Pool) fixture {
	t.Helper()
	f := fixture{orgA: idgen.New(), orgB: idgen.New(), ownerA: idgen.New(), adminA: idgen.New(), shared: idgen.New(), ownerB: idgen.New()}
	f.nameA = "Deletion Test " + f.orgA
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO organizations (id, name) VALUES ($1, $2), ($3, $4)`, f.orgA, f.nameA, f.orgB, "Deletion Control "+f.orgB)
	t.Cleanup(func() {
		for _, org := range []string{f.orgA, f.orgB} {
			_ = testpg.DeleteOrganization(context.Background(), pool, org)
			_, _ = pool.Exec(context.Background(), `DELETE FROM organization_deletions WHERE organization_id = $1`, org)
		}
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1)`, []string{f.ownerA, f.adminA, f.shared, f.ownerB})
	})
	for _, u := range []string{f.ownerA, f.adminA, f.shared, f.ownerB} {
		exec(`INSERT INTO users (id, email, name, password_hash, email_verified_at) VALUES ($1, $2, 'Deletion User', $3, now())`,
			u, "del-"+strings.ToLower(u)+"@example.test", string(hash))
	}
	member := func(org, user, role string) {
		exec(`INSERT INTO memberships (id, organization_id, user_id, role_id, status)
		      VALUES ($1, $2, $3, (SELECT id FROM roles WHERE key = $4), 'active')`, idgen.New(), org, user, role)
	}
	member(f.orgA, f.ownerA, "Owner")
	member(f.orgA, f.adminA, "Admin")
	member(f.orgA, f.shared, "Admin")
	member(f.orgB, f.shared, "Owner")
	member(f.orgB, f.ownerB, "Owner")
	for _, org := range []string{f.orgA, f.orgB} {
		exec(`INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, 'Deletion Network')`, idgen.New(), org)
		exec(`INSERT INTO traffic_sources (id, organization_id, name, type) VALUES ($1, $2, 'Deletion Source', 'Facebook')`, idgen.New(), org)
	}
	return f
}

// Audit 2026-10-15, GDPR-1 part 2: an owner schedules the deletion, it can
// be cancelled, and when it is due everything of the organization goes —
// every Postgres table, ClickHouse, object storage, Redis — and its users
// who belong nowhere else. Another organization, and a user who also
// belongs to it, stay as they were.
func TestOrganizationDeletion(t *testing.T) {
	requireEnv(t, "DATABASE_URL", "CLICKHOUSE_URL", "S3_ENDPOINT", "REDIS_URL")
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f := seed(t, ctx, pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mail := &mails{}
	svc := orgdeletion.NewService(pool, mail, "http://localhost:3000", logger)

	// --- scheduling ---
	wantErr := func(stage string, err error, status int) {
		t.Helper()
		var apiErr *apierror.Error
		if !errors.As(err, &apiErr) || apiErr.Status != status {
			t.Fatalf("%s: %v, want HTTP %d", stage, err, status)
		}
	}
	_, err = svc.Schedule(ctx, f.orgA, f.adminA, f.nameA, password)
	wantErr("an admin schedules", err, 403)
	_, err = svc.Schedule(ctx, f.orgA, f.ownerB, f.nameA, password)
	wantErr("another organization's owner schedules", err, 403)
	_, err = svc.Schedule(ctx, f.orgA, f.ownerA, "Wrong Name", password)
	wantErr("wrong name", err, 422)
	_, err = svc.Schedule(ctx, f.orgA, f.ownerA, f.nameA, "wrong password")
	wantErr("wrong password", err, 422)

	st, err := svc.Schedule(ctx, f.orgA, f.ownerA, f.nameA, password)
	if err != nil || st.ScheduledAt == nil || st.ScheduledAt.Sub(time.Now()) < orgdeletion.GracePeriod-time.Minute {
		t.Fatalf("Schedule = %+v, %v; want a date a grace period away", st, err)
	}
	if len(mail.sent) != 1 || !strings.Contains(mail.sent[0], "will be deleted") {
		t.Fatalf("mails = %v, want one to the owner", mail.sent)
	}
	_, err = svc.Schedule(ctx, f.orgA, f.ownerA, f.nameA, password)
	wantErr("scheduled twice", err, 409)
	if _, err := svc.Cancel(ctx, f.orgA, f.adminA); err == nil {
		t.Fatal("an admin cancelled")
	}
	if st, err := svc.Cancel(ctx, f.orgA, f.ownerA); err != nil || st.ScheduledAt != nil {
		t.Fatalf("Cancel = %+v, %v", st, err)
	}
	if _, err := svc.Schedule(ctx, f.orgA, f.ownerA, f.nameA, password); err != nil {
		t.Fatalf("Schedule again: %v", err)
	}

	// --- data in the other stores ---
	ch, err := chconn.NewConn(ctx, config.ClickHouseConfig{URL: os.Getenv("CLICKHOUSE_URL"), Database: envOr("CLICKHOUSE_DATABASE", "flox"), User: envOr("CLICKHOUSE_USER", "flox"), Password: os.Getenv("CLICKHOUSE_PASSWORD")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ch.Close() })
	at := time.Now().UTC().Add(-time.Hour)
	var events []event.Event
	for _, org := range []string{f.orgA, f.orgB} {
		click := idgen.New()
		amount := 10.0
		events = append(events,
			event.Event{Type: event.SourceClick, EventAt: at, OrganizationID: org, CampaignID: "c", ClickID: click, IP: "203.0.113.9"},
			event.Event{Type: event.LandView, EventAt: at, OrganizationID: org, CampaignID: "c", ClickID: click},
			event.Event{ID: idgen.New(), Type: event.CpaAccept, EventAt: at, OrganizationID: org, CampaignID: "c", ClickID: click, Revenue: amount, USDValue: amount},
		)
		if org == f.orgA {
			f.clickA = click
		} else {
			f.clickB = click
		}
	}
	if err := chstore.NewEventStore(ch).InsertBatch(ctx, events); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}
	t.Cleanup(func() {
		wait := clickhouse.Context(context.Background(), clickhouse.WithSettings(clickhouse.Settings{"mutations_sync": 2}))
		for _, table := range []string{"click_events", "tracking_events", "conversion_events", "click_lookup", "ltv_events", "click_events_daily_campaign", "click_events_daily_geo", "conversion_events_daily_campaign"} {
			_ = ch.Exec(wait, "DELETE FROM "+table+" WHERE organization_id = ?", f.orgB)
		}
	})

	objects, err := storage.NewClient(config.S3Config{Endpoint: os.Getenv("S3_ENDPOINT"), Region: os.Getenv("S3_REGION"), Bucket: os.Getenv("S3_BUCKET"),
		AccessKeyID: os.Getenv("S3_ACCESS_KEY_ID"), SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"), PublicEndpoint: os.Getenv("S3_PUBLIC_ENDPOINT")})
	if err != nil {
		t.Fatal(err)
	}
	if err := objects.EnsureBucket(ctx, os.Getenv("S3_REGION")); err != nil {
		t.Skipf("object storage unreachable: %v", err)
	}
	f.objectsA = []string{"landings/" + f.orgA + "/l1/v1/style.css", "exports/" + f.orgA + "/e1-x.zip", f.orgA + "/g1.png"}
	f.objectsB = []string{"landings/" + f.orgB + "/l1/v1/style.css", "exports/" + f.orgB + "/e1-x.zip", f.orgB + "/g1.png"}
	for _, key := range append(append([]string{}, f.objectsA...), f.objectsB...) {
		if err := objects.PutObject(ctx, key, strings.NewReader("x"), 1, "text/plain"); err != nil {
			t.Fatalf("PutObject %s: %v", key, err)
		}
	}
	t.Cleanup(func() {
		_ = objects.DeletePrefix(context.Background(), "landings/"+f.orgB+"/")
		_ = objects.DeletePrefix(context.Background(), "exports/"+f.orgB+"/")
		_ = objects.DeletePrefix(context.Background(), f.orgB+"/")
	})

	rdb, err := rediscache.NewClient(ctx, os.Getenv("REDIS_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	f.redisKeyA = "flox:conversion:last-status:" + f.orgA + ":" + f.clickA
	f.redisKeyB = "flox:conversion:last-status:" + f.orgB + ":" + f.clickB
	for _, k := range []string{f.redisKeyA, f.redisKeyB} {
		if err := rdb.Set(ctx, k, "CPA_ACCEPT", time.Hour).Err(); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = rdb.Del(context.Background(), f.redisKeyB).Err() })

	runner := &orgdeletion.Runner{DB: pool, CH: ch, Storage: objects, Redis: rdb, Logger: logger, Organization: f.orgA}

	// --- not due yet: nothing happens ---
	if err := runner.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce before the date: %v", err)
	}
	if n := pgCount(t, ctx, pool, `SELECT count(*) FROM organizations WHERE id = $1`, f.orgA); n != 1 {
		t.Fatal("deleted before its date")
	}

	// --- due: Postgres goes, the rest waits for PurgeDelay ---
	if _, err := pool.Exec(ctx, `UPDATE organizations SET deletion_scheduled_at = now() - interval '1 second' WHERE id = $1`, f.orgA); err != nil {
		t.Fatal(err)
	}
	if err := runner.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce when due: %v", err)
	}
	for _, table := range tablesWithOrganizationID(t, ctx, pool) {
		if n := pgCount(t, ctx, pool, `SELECT count(*) FROM "`+table+`" WHERE organization_id = $1`, f.orgA); n != 0 {
			t.Errorf("Postgres %s still has %d rows of the deleted organization", table, n)
		}
	}
	for user, want := range map[string]int{f.ownerA: 0, f.adminA: 0, f.shared: 1, f.ownerB: 1} {
		if n := pgCount(t, ctx, pool, `SELECT count(*) FROM users WHERE id = $1`, user); n != want {
			t.Errorf("user %s: %d rows, want %d", user, n, want)
		}
	}
	if n := pgCount(t, ctx, pool, `SELECT count(*) FROM networks WHERE organization_id = $1`, f.orgB); n != 1 {
		t.Errorf("the other organization's network: %d rows, want 1", n)
	}
	if n := pgCount(t, ctx, pool, `SELECT count(*) FROM organization_deletions WHERE organization_id = $1 AND purged_at IS NULL AND users_deleted = 2`, f.orgA); n != 1 {
		t.Fatalf("tombstone not written as expected")
	}
	if n := chCount(t, ctx, ch, "click_events", f.orgA); n == 0 {
		t.Fatal("ClickHouse purged before PurgeDelay")
	}

	// --- PurgeDelay later: ClickHouse, objects, Redis ---
	if _, err := pool.Exec(ctx, `UPDATE organization_deletions SET deleted_at = now() - interval '2 hours' WHERE organization_id = $1`, f.orgA); err != nil {
		t.Fatal(err)
	}
	if err := runner.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce after the delay: %v", err)
	}
	if n := pgCount(t, ctx, pool, `SELECT count(*) FROM organization_deletions WHERE organization_id = $1 AND purged_at IS NOT NULL AND last_error = ''`, f.orgA); n != 1 {
		t.Fatal("tombstone not marked purged")
	}
	for _, table := range []string{"click_events", "tracking_events", "conversion_events", "click_lookup", "ltv_events", "click_events_daily_campaign", "conversion_events_daily_campaign"} {
		if n := chCount(t, ctx, ch, table, f.orgA); n != 0 {
			t.Errorf("ClickHouse %s still has %d rows of the deleted organization", table, n)
		}
	}
	for _, table := range []string{"click_events", "conversion_events", "click_lookup"} {
		if n := chCount(t, ctx, ch, table, f.orgB); n == 0 {
			t.Errorf("ClickHouse %s lost the other organization's rows", table)
		}
	}
	// The bucket is public-read (storage.publicReadPolicy): an object is
	// there when its public URL answers 200.
	exists := func(key string) bool {
		t.Helper()
		resp, err := http.Get(objects.PublicURL(key))
		if err != nil {
			t.Fatalf("GET %s: %v", key, err)
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}
	for _, key := range f.objectsA {
		if exists(key) {
			t.Errorf("object %s survived", key)
		}
	}
	for _, key := range f.objectsB {
		if !exists(key) {
			t.Errorf("the other organization's object %s is gone", key)
		}
	}
	if n, _ := rdb.Exists(ctx, f.redisKeyA).Result(); n != 0 {
		t.Error("Redis key of the deleted organization survived")
	}
	if n, _ := rdb.Exists(ctx, f.redisKeyB).Result(); n != 1 {
		t.Error("the other organization's Redis key is gone")
	}
}

func pgCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

// tablesWithOrganizationID is every base table with an organization_id
// column, so a table added later is checked without a change here.
func tablesWithOrganizationID(t *testing.T, ctx context.Context, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT c.table_name FROM information_schema.columns c
		JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name
		WHERE c.table_schema = 'public' AND c.column_name = 'organization_id' AND t.table_type = 'BASE TABLE'
		  AND c.table_name <> 'organization_deletions'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		out = append(out, name)
	}
	if len(out) < 40 {
		t.Fatalf("found %d tables with organization_id, want the whole schema", len(out))
	}
	return out
}

func chCount(t *testing.T, ctx context.Context, conn driver.Conn, table, orgID string) uint64 {
	t.Helper()
	var n uint64
	if err := conn.QueryRow(ctx, "SELECT count() FROM "+table+" WHERE organization_id = ?", orgID).Scan(&n); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return n
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
