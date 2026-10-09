package backup

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/minio/minio-go/v7"

	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// The Postgres half needs wal-g and pg_ctl next to the data directory, so it
// is exercised in the sidecar (`make backup-drill`), not here.

func testTarget(t *testing.T) Target {
	t.Helper()
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" || os.Getenv("CLICKHOUSE_URL") == "" {
		t.Skip("needs CLICKHOUSE_URL and S3_ENDPOINT")
	}
	tg := Target{Endpoint: endpoint, Region: os.Getenv("S3_REGION"), Bucket: "flox-backup-test-" + strings.ToLower(idgen.New()),
		AccessKeyID: os.Getenv("S3_ACCESS_KEY_ID"), SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY")}
	b, err := newBucket(tg)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dropBucket(b.c, tg.Bucket) })
	return tg
}

func dropBucket(c *minio.Client, name string) {
	ctx := context.Background()
	b := &bucket{c: c, name: name}
	_ = b.removePrefix(ctx, "")
	_ = c.RemoveBucket(ctx, name)
}

func testClickHouse(t *testing.T) driver.Conn {
	t.Helper()
	conn, err := chconn.NewConn(context.Background(), config.ClickHouseConfig{
		URL: os.Getenv("CLICKHOUSE_URL"), Database: "default", User: os.Getenv("CLICKHOUSE_USER")})
	if err != nil {
		t.Fatalf("clickhouse: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestClickHouseBackupDrillAndPrune(t *testing.T) {
	tg := testTarget(t)
	conn := testClickHouse(t)
	ctx := context.Background()
	db := "flox_backup_test_" + strings.ToLower(idgen.New())
	for _, q := range []string{
		"CREATE DATABASE " + db,
		"CREATE TABLE " + db + ".events (id UInt32) ENGINE = MergeTree ORDER BY id",
		"CREATE TABLE " + db + ".daily (id UInt32, n UInt64) ENGINE = SummingMergeTree ORDER BY id",
		"CREATE MATERIALIZED VIEW " + db + ".daily_mv TO " + db + ".daily AS SELECT id, count() AS n FROM " + db + ".events GROUP BY id",
		"INSERT INTO " + db + ".events SELECT number FROM numbers(50)",
	} {
		if err := conn.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	t.Cleanup(func() {
		_ = conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+db+" SYNC")
		_ = conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+db+"_drill SYNC")
	})
	// The ClickHouse server reaches MinIO by its compose name.
	ch, err := NewClickHouse(conn, db, tg, envOr("BACKUP_TEST_CLICKHOUSE_S3_ENDPOINT", "http://minio:9000"))
	if err != nil {
		t.Fatal(err)
	}

	first := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	m, err := ch.Backup(ctx, first)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if m.Tables["events"] != (RowsBetween{50, 50}) || m.Tables["daily"].After == 0 {
		t.Fatalf("manifest = %+v", m.Tables)
	}
	if _, ok := m.Tables["daily_mv"]; ok {
		t.Error("a view has no rows to count")
	}

	if failed := failedChecks(ch.Drill(ctx)); failed != "" {
		t.Fatalf("drill on a good backup failed: %s", failed)
	}
	var drillLeft uint64
	if err := conn.QueryRow(ctx, "SELECT count() FROM system.databases WHERE name = ?", db+"_drill").Scan(&drillLeft); err != nil || drillLeft != 0 {
		t.Errorf("drill database left behind (%d, %v)", drillLeft, err)
	}

	// A manifest that disagrees with what was backed up fails the drill.
	m.Tables["events"] = RowsBetween{60, 60}
	if err := ch.bucket.putJSON(ctx, chManifestsDir+"/"+m.Name+".json", m); err != nil {
		t.Fatal(err)
	}
	if failed := failedChecks(ch.Drill(ctx)); !strings.Contains(failed, "events: 50 rows restored, backup saw 60–60") {
		t.Fatalf("drill failures = %q, want the events count caught", failed)
	}

	// Prune keeps the newest.
	for i := 1; i <= 2; i++ {
		if _, err := ch.Backup(ctx, first.AddDate(0, 0, i)); err != nil {
			t.Fatalf("Backup %d: %v", i, err)
		}
	}
	gone, err := ch.Prune(ctx, 2)
	if err != nil || len(gone) != 1 || gone[0] != m.Name {
		t.Fatalf("Prune = %v, %v; want only %s", gone, err, m.Name)
	}
	left, _ := ch.List(ctx)
	if len(left) != 2 || left[0] != name(first.AddDate(0, 0, 1)) {
		t.Errorf("kept = %v", left)
	}
	var files int
	for o := range ch.bucket.c.ListObjects(ctx, tg.Bucket, minio.ListObjectsOptions{Prefix: chDir + "/" + m.Name + "/", Recursive: true}) {
		if o.Err == nil {
			files++
		}
	}
	if files != 0 {
		t.Errorf("%d files of the pruned backup left", files)
	}
}

func TestObjectsMirrorAndDrill(t *testing.T) {
	tg := testTarget(t)
	ctx := context.Background()
	src := config.S3Config{Endpoint: tg.Endpoint, Region: tg.Region, Bucket: "flox-backup-src-" + strings.ToLower(idgen.New()),
		AccessKeyID: tg.AccessKeyID, SecretAccessKey: tg.SecretAccessKey}
	o, err := NewObjects(src, tg)
	if err != nil {
		t.Fatal(err)
	}
	if err := o.src.MakeBucket(ctx, src.Bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dropBucket(o.src, src.Bucket) })
	put := func(key, body string) {
		t.Helper()
		if _, err := o.src.PutObject(ctx, src.Bucket, key, bytes.NewReader([]byte(body)), int64(len(body)), minio.PutObjectOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	put("a/1.png", "one")
	put("a/2.png", "two")
	put("b/3.png", "three")

	m, err := o.Mirror(ctx, time.Now())
	if err != nil || m.Objects != 3 || m.Copied != 3 || m.Bytes != 11 {
		t.Fatalf("first mirror = %+v, %v", m, err)
	}
	time.Sleep(1100 * time.Millisecond) // object times have second precision
	m, err = o.Mirror(ctx, time.Now())
	if err != nil || m.Copied != 0 {
		t.Fatalf("second mirror copied %d, want nothing new (%v)", m.Copied, err)
	}
	if failed := failedChecks(o.Drill(ctx)); failed != "" {
		t.Fatalf("drill on a good mirror failed: %s", failed)
	}

	put("a/2.png", "two, changed")
	m, err = o.Mirror(ctx, time.Now())
	if err != nil || m.Copied != 1 {
		t.Fatalf("mirror after a change copied %d, want 1 (%v)", m.Copied, err)
	}

	// A file the mirror lost fails the drill.
	if err := o.bucket.c.RemoveObject(ctx, tg.Bucket, objectsDir+"/b/3.png", minioRemove); err != nil {
		t.Fatal(err)
	}
	if failed := failedChecks(o.Drill(ctx)); !strings.Contains(failed, "b/3.png") {
		t.Fatalf("drill failures = %q, want b/3.png missing", failed)
	}
}

func failedChecks(checks []Check) string {
	var out []string
	for _, c := range checks {
		if !c.OK {
			out = append(out, c.Name+": "+c.Detail)
		}
	}
	return strings.Join(out, "; ")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
