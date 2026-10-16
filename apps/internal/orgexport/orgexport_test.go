package orgexport_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/orgexport"
	"github.com/ismagilovnail/flox/apps/internal/storage"
	"github.com/ismagilovnail/flox/apps/internal/testpg"
)

func requireEnv(t *testing.T, vars ...string) {
	t.Helper()
	for _, v := range vars {
		if os.Getenv(v) == "" {
			t.Skipf("needs %s", v)
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Audit 2026-10-15, GDPR-1 part 4: an organization's export holds its
// records, team and events of the period — and none of its secrets, and
// nothing of another organization. It is downloaded through the API only,
// and deleted after Retention.
func TestOrganizationExport(t *testing.T) {
	requireEnv(t, "DATABASE_URL", "CLICKHOUSE_URL", "S3_ENDPOINT")
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ch, err := chconn.NewConn(ctx, config.ClickHouseConfig{URL: os.Getenv("CLICKHOUSE_URL"), Database: envOr("CLICKHOUSE_DATABASE", "flox"), User: envOr("CLICKHOUSE_USER", "flox"), Password: os.Getenv("CLICKHOUSE_PASSWORD")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ch.Close() })
	objects, err := storage.NewClient(config.S3Config{Endpoint: os.Getenv("S3_ENDPOINT"), Region: os.Getenv("S3_REGION"), Bucket: os.Getenv("S3_BUCKET"),
		AccessKeyID: os.Getenv("S3_ACCESS_KEY_ID"), SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"), PublicEndpoint: os.Getenv("S3_PUBLIC_ENDPOINT")})
	if err != nil {
		t.Fatal(err)
	}
	if err := objects.EnsureBucket(ctx, os.Getenv("S3_REGION")); err != nil {
		t.Skipf("object storage unreachable: %v", err)
	}

	orgA, orgB, owner := idgen.New(), idgen.New(), idgen.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO organizations (id, name) VALUES ($1, 'Export A'), ($2, 'Export B')`, orgA, orgB)
	t.Cleanup(func() {
		_ = testpg.DeleteOrganization(context.Background(), pool, orgA)
		_ = testpg.DeleteOrganization(context.Background(), pool, orgB)
		_ = objects.DeletePrefix(context.Background(), "exports/"+orgA+"/")
		wait := clickhouse.Context(context.Background(), clickhouse.WithSettings(clickhouse.Settings{"mutations_sync": 2}))
		for _, table := range []string{"click_events", "tracking_events", "conversion_events", "click_lookup", "ltv_events"} {
			_ = ch.Exec(wait, "DELETE FROM "+table+" WHERE organization_id IN ?", []string{orgA, orgB})
		}
	})
	exec(`INSERT INTO users (id, email, name, password_hash, email_verified_at) VALUES ($1, $2, 'Export Owner', 'PWHASH-MARKER', now())`,
		owner, "export-"+strings.ToLower(owner)+"@example.test")
	exec(`INSERT INTO memberships (id, organization_id, user_id, role_id, status) VALUES ($1, $2, $3, (SELECT id FROM roles WHERE key = 'Owner'), 'active')`,
		idgen.New(), orgA, owner)

	// Secrets planted in real secret columns: none may reach the archive.
	secrets := []string{"NETSECRET-MARKER", "APIKEYHASH-MARKER", "PIXELTOKEN-MARKER", "PWHASH-MARKER"}
	exec(`INSERT INTO networks (id, organization_id, name, postback_secret_hash) VALUES ($1, $2, 'Export Network A', 'NETSECRET-MARKER')`, idgen.New(), orgA)
	exec(`INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, 'Export Network B-ONLY')`, idgen.New(), orgB)
	exec(`INSERT INTO api_keys (id, organization_id, name, key_hash, prefix, scope, created_by_user_id)
	      VALUES ($1, $2, 'k', 'APIKEYHASH-MARKER', 'flx_ab', 'read', $3)`, idgen.New(), orgA, owner)
	exec(`INSERT INTO pixels (id, organization_id, name, provider, pixel_id, access_token) VALUES ($1, $2, 'px', 'facebook', '123', 'PIXELTOKEN-MARKER')`, idgen.New(), orgA)

	clickA := idgen.New()
	day := time.Now().UTC().AddDate(0, 0, -2)
	if err := chstore.NewEventStore(ch).InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: day, OrganizationID: orgA, CampaignID: "c", ClickID: clickA, IP: "203.0.113.4", Country: "DE"},
		{Type: event.SourceClick, EventAt: day.AddDate(0, 0, -30), OrganizationID: orgA, CampaignID: "c", ClickID: "outside-the-period"},
		{Type: event.SourceClick, EventAt: day, OrganizationID: orgB, CampaignID: "c", ClickID: "click-of-b"},
	}); err != nil {
		t.Fatal(err)
	}

	svc := orgexport.NewService(pool, objects)
	from, to := day.AddDate(0, 0, -1).Format(time.DateOnly), time.Now().UTC().Format(time.DateOnly)
	var apiErr *apierror.Error
	if _, err := svc.Request(ctx, orgA, owner, to, from); !errors.As(err, &apiErr) || apiErr.Status != 422 {
		t.Fatalf("reversed period: %v, want 422", err)
	}
	if _, err := svc.Request(ctx, orgA, owner, "2024-01-01", to); !errors.As(err, &apiErr) || apiErr.Status != 422 {
		t.Fatalf("period over a year: %v, want 422", err)
	}
	exp, err := svc.Request(ctx, orgA, owner, from, to)
	if err != nil || exp.Status != "pending" {
		t.Fatalf("Request = %+v, %v", exp, err)
	}
	if _, err := svc.Request(ctx, orgA, owner, from, to); !errors.As(err, &apiErr) || apiErr.Status != 409 {
		t.Fatalf("second request while one is pending: %v, want 409", err)
	}
	if _, _, err := svc.Open(ctx, orgA, exp.ID); !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Fatalf("download before it is built: %v, want 404", err)
	}

	runner := &orgexport.Runner{DB: pool, CH: ch, Objects: objects, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Organization: orgA}
	if err := runner.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	list, err := svc.List(ctx, orgA)
	if err != nil || len(list) != 1 || list[0].Status != "done" || list[0].SizeBytes == 0 || list[0].ExpiresAt == nil {
		t.Fatalf("List after the build = %+v, %v (error %q)", list, err, func() string {
			if len(list) > 0 {
				return list[0].Error
			}
			return ""
		}())
	}
	if _, _, err := svc.Open(ctx, orgB, exp.ID); !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Fatalf("another organization downloads it: %v, want 404", err)
	}

	body, _, err := svc.Open(ctx, orgA, exp.ID)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	raw, err := io.ReadAll(body)
	body.Close()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(b)
	}

	// No secret value anywhere, no column that looks like one, nothing of B.
	all := strings.Join(mapValues(files), "\n")
	for _, s := range secrets {
		if strings.Contains(all, s) {
			t.Errorf("the archive contains the secret %s", s)
		}
	}
	if strings.Contains(all, "B-ONLY") || strings.Contains(all, "click-of-b") || strings.Contains(all, orgB) {
		t.Error("the archive contains another organization's data")
	}
	for name, content := range files {
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
			if line == "" {
				continue
			}
			var row map[string]any
			if err := json.Unmarshal([]byte(line), &row); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			for col := range row {
				if orgexport.SecretColumn.MatchString(col) {
					t.Errorf("%s exports the column %s", name, col)
				}
			}
		}
	}

	if !strings.Contains(files["postgres/networks.jsonl"], "Export Network A") || !strings.Contains(files["postgres/pixels.jsonl"], `"pixel_id"`) {
		t.Errorf("records missing: networks %q", files["postgres/networks.jsonl"])
	}
	if !strings.Contains(files["team.jsonl"], "export-") || !strings.Contains(files["team.jsonl"], `"role": "Owner"`) {
		t.Errorf("team = %q", files["team.jsonl"])
	}
	for table := range orgexport.ExcludedTables {
		if _, ok := files["postgres/"+table+".jsonl"]; ok {
			t.Errorf("the excluded table %s is in the archive", table)
		}
	}
	recs, err := csv.NewReader(strings.NewReader(files["clickhouse/click_events.csv"])).ReadAll()
	if err != nil || len(recs) != 2 || !strings.Contains(strings.Join(recs[1], ","), clickA) {
		t.Fatalf("click_events.csv = %v, %v; want the header and the one click of the period", recs, err)
	}
	if !strings.Contains(files["README.txt"], "postback_secret_hash") {
		t.Error("README does not list the columns left out")
	}

	// Expiry deletes the archive.
	exec(`UPDATE organization_exports SET expires_at = now() - interval '1 second' WHERE id = $1`, exp.ID)
	if err := runner.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if list, _ := svc.List(ctx, orgA); len(list) != 1 || list[0].Status != "expired" {
		t.Fatalf("after expiry = %+v", list)
	}
	if _, _, err := svc.Open(ctx, orgA, exp.ID); !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Fatalf("download after expiry: %v, want 404", err)
	}
}

// Every column of a tenant table that looks like a secret really exists
// for the regex to catch — a guard against the pattern drifting from the
// schema: the known secret columns must all match it.
func TestSecretColumnCoversKnownSecrets(t *testing.T) {
	for _, col := range []string{"postback_secret_hash", "key_hash", "access_token", "refresh_token", "client_secret",
		"developer_token", "token_encrypted", "webhook_secret_hash", "encrypted_payload", "invite_token_hash",
		"token_hash", "p256dh", "auth", "endpoint", "link_token", "password_hash", "verification_token"} {
		if !orgexport.SecretColumn.MatchString(col) {
			t.Errorf("%s is not left out of exports", col)
		}
	}
	for _, col := range []string{"name", "pixel_id", "author", "postback_url", "object_key", "status"} {
		if orgexport.SecretColumn.MatchString(col) {
			t.Errorf("%s would be left out, but is not a secret", col)
		}
	}
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
