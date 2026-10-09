// Package backup backs up FLOX's three stores to an S3-compatible bucket
// and proves, by restoring them, that the backups work (tech audit
// 2026-10-06, DO2; docs/backup.md).
//
//   - Postgres: WAL-G. The server archives every WAL segment to the bucket
//     (archive_command, set on the postgres container); this package takes
//     the nightly base backup and prunes old ones. Together they allow a
//     restore to any moment of the kept window (PITR).
//   - ClickHouse: BACKUP DATABASE ... TO S3, nightly, with a manifest of row
//     counts taken around it.
//   - Object storage: the S3_BUCKET files mirrored into the bucket.
//
// The drill restores each store into a scratch copy and checks it: Postgres
// to the latest archived WAL (a mark written just before must be there),
// ClickHouse into <database>_drill (row counts as in the manifest), objects
// by size and a hashed sample. Its report is kept in the bucket.
//
// Everything this package reports — last backup, last drill — is read back
// from the bucket, so a restarted sidecar reports the same as before.
//
// Bucket layout:
//
//	postgres/                    WAL-G (basebackups_005/, wal_005/)
//	clickhouse/<name>/           one BACKUP
//	clickhouse-manifests/<name>.json
//	objects/<key>                the mirror
//	objects-manifests/<name>.json
//	drills/<name>.json
package backup

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/config"
)

// Stores, as the metrics and reports name them.
const (
	StorePostgres   = "postgres"
	StoreClickHouse = "clickhouse"
	StoreObjects    = "objects"
)

// nameLayout names a backup by its start: sortable, path-safe.
const nameLayout = "20060102T150405Z"

func name(t time.Time) string { return t.UTC().Format(nameLayout) }

// Target is the bucket backups go to — off-site in production, never the
// bucket being backed up.
type Target struct {
	Endpoint, Region, Bucket, AccessKeyID, SecretAccessKey string
}

// Config is the sidecar's environment (.env.example, BACKUP_*).
type Config struct {
	Target Target
	// ClickHouseS3Endpoint is Target.Endpoint as the ClickHouse server
	// reaches it: BACKUP ... TO S3 is run by the server, not by us.
	ClickHouseS3Endpoint string

	// DatabaseURL connects as a role that may run pg_backup_start and
	// pg_switch_wal (a superuser, or one granted them).
	DatabaseURL string
	PGData      string // the live data directory, mounted read-only
	DrillDir    string // scratch space for the Postgres drill: about the database's size

	ClickHouse config.ClickHouseConfig
	Objects    config.S3Config // empty Bucket: no objects to mirror

	At             time.Duration // daily run, UTC offset from midnight
	KeepPostgres   int           // base backups kept (the PITR window, in days)
	KeepClickHouse int
	DrillDay       int // day of the month the drill follows the backup; 0 = never

	HTTPAddr string
}

// ConfigFromEnv reads Config.
func ConfigFromEnv() (Config, error) {
	c := Config{
		Target: Target{
			Endpoint:        os.Getenv("BACKUP_S3_ENDPOINT"),
			Region:          env("BACKUP_S3_REGION", "us-east-1"),
			Bucket:          os.Getenv("BACKUP_S3_BUCKET"),
			AccessKeyID:     os.Getenv("BACKUP_S3_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("BACKUP_S3_SECRET_ACCESS_KEY"),
		},
		DatabaseURL: env("BACKUP_DATABASE_URL", os.Getenv("DATABASE_URL")),
		PGData:      env("PGDATA", "/var/lib/postgresql/data"),
		DrillDir:    env("BACKUP_DRILL_DIR", "/var/lib/flox-drill"),
		ClickHouse: config.ClickHouseConfig{
			URL:      os.Getenv("CLICKHOUSE_URL"),
			Database: env("CLICKHOUSE_DATABASE", "flox"),
			User:     os.Getenv("CLICKHOUSE_USER"),
			Password: os.Getenv("CLICKHOUSE_PASSWORD"),
		},
		Objects: config.S3Config{
			Endpoint:        os.Getenv("S3_ENDPOINT"),
			Region:          os.Getenv("S3_REGION"),
			Bucket:          os.Getenv("S3_BUCKET"),
			AccessKeyID:     os.Getenv("S3_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"),
		},
		HTTPAddr: env("BACKUP_HTTP_ADDR", ":8083"),
	}
	c.ClickHouseS3Endpoint = env("BACKUP_CLICKHOUSE_S3_ENDPOINT", c.Target.Endpoint)

	var err error
	if c.At, err = parseClock(env("BACKUP_AT", "03:00")); err != nil {
		return Config{}, fmt.Errorf("BACKUP_AT: %w", err)
	}
	for _, v := range []struct {
		key      string
		def, min int
		dst      *int
	}{
		{"BACKUP_KEEP_POSTGRES", 14, 1, &c.KeepPostgres},
		{"BACKUP_KEEP_CLICKHOUSE", 14, 1, &c.KeepClickHouse},
		{"BACKUP_DRILL_DAY", 1, 0, &c.DrillDay},
	} {
		n, err := strconv.Atoi(env(v.key, strconv.Itoa(v.def)))
		if err != nil || n < v.min || (v.key == "BACKUP_DRILL_DAY" && n > 28) {
			return Config{}, fmt.Errorf("%s: want a whole number ≥ %d%s", v.key, v.min,
				map[bool]string{true: " and ≤ 28", false: ""}[v.key == "BACKUP_DRILL_DAY"])
		}
		*v.dst = n
	}

	var missing []string
	for k, v := range map[string]string{
		"BACKUP_S3_ENDPOINT": c.Target.Endpoint, "BACKUP_S3_BUCKET": c.Target.Bucket,
		"BACKUP_S3_ACCESS_KEY_ID": c.Target.AccessKeyID, "BACKUP_S3_SECRET_ACCESS_KEY": c.Target.SecretAccessKey,
		"BACKUP_DATABASE_URL or DATABASE_URL": c.DatabaseURL, "CLICKHOUSE_URL": c.ClickHouse.URL,
	} {
		if v == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing %s", strings.Join(missing, ", "))
	}
	if c.Objects.Bucket != "" && c.Objects.Bucket == c.Target.Bucket && sameHost(c.Objects.Endpoint, c.Target.Endpoint) {
		return Config{}, fmt.Errorf("BACKUP_S3_BUCKET must not be S3_BUCKET: a backup inside the bucket it backs up")
	}
	return c, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseClock reads "HH:MM".
func parseClock(s string) (time.Duration, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("want HH:MM (UTC), got %q", s)
	}
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute, nil
}

func sameHost(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	return errA == nil && errB == nil && strings.EqualFold(ua.Host, ub.Host)
}

// NextRun is the first daily run strictly after now.
func NextRun(now time.Time, at time.Duration) time.Time {
	now = now.UTC()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	next := day.Add(at)
	if !next.After(now) {
		next = day.AddDate(0, 0, 1).Add(at)
	}
	return next
}
