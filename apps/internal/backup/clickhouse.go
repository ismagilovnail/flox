package backup

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

const (
	chDir          = "clickhouse"
	chManifestsDir = "clickhouse-manifests"
)

// ClickHouseManifest describes one ClickHouse backup. Each table's rows
// are counted right before and right after its BACKUP: inserts during the
// backup land on either side, so a restore must fall between the two.
type ClickHouseManifest struct {
	Name       string                 `json:"name"`
	Database   string                 `json:"database"`
	StartedAt  time.Time              `json:"startedAt"`
	FinishedAt time.Time              `json:"finishedAt"`
	Tables     map[string]RowsBetween `json:"tables"`
}

// RowsBetween is a table's row count before and after the backup.
type RowsBetween struct {
	Before uint64 `json:"before"`
	After  uint64 `json:"after"`
}

// Holds reports whether n rows can be what the backup captured.
func (r RowsBetween) Holds(n uint64) bool {
	return n >= min(r.Before, r.After) && n <= max(r.Before, r.After)
}

// ClickHouse backs up one database (BACKUP DATABASE ... TO S3).
type ClickHouse struct {
	conn     driver.Conn
	database string
	bucket   *bucket
	// s3Endpoint, accessKeyID, secretAccessKey: how the ClickHouse
	// server itself reaches the bucket.
	s3Endpoint, accessKeyID, secretAccessKey string
}

func NewClickHouse(conn driver.Conn, database string, t Target, s3Endpoint string) (*ClickHouse, error) {
	b, err := newBucket(t)
	if err != nil {
		return nil, err
	}
	return &ClickHouse{conn: conn, database: database, bucket: b,
		s3Endpoint: strings.TrimSuffix(s3Endpoint, "/"), accessKeyID: t.AccessKeyID, secretAccessKey: t.SecretAccessKey}, nil
}

// s3 is the S3(...) argument of BACKUP/RESTORE for one backup.
func (c *ClickHouse) s3(backupName string) string {
	return fmt.Sprintf("S3(%s, %s, %s)",
		quote(c.s3Endpoint+"/"+c.bucket.name+"/"+chDir+"/"+backupName), quote(c.accessKeyID), quote(c.secretAccessKey))
}

// quote is a ClickHouse string literal.
func quote(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

func ident(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }

// Backup takes one backup and writes its manifest.
func (c *ClickHouse) Backup(ctx context.Context, at time.Time) (ClickHouseManifest, error) {
	m := ClickHouseManifest{Name: name(at), Database: c.database, StartedAt: at.UTC()}
	before, err := c.counts(ctx, c.database)
	if err != nil {
		return m, err
	}
	if err := c.conn.Exec(ctx, "BACKUP DATABASE "+ident(c.database)+" TO "+c.s3(m.Name)); err != nil {
		return m, fmt.Errorf("backup: clickhouse BACKUP: %w", err)
	}
	after, err := c.counts(ctx, c.database)
	if err != nil {
		return m, err
	}
	m.FinishedAt = time.Now().UTC()
	m.Tables = map[string]RowsBetween{}
	for t, n := range before {
		m.Tables[t] = RowsBetween{Before: n, After: after[t]}
	}
	return m, c.bucket.putJSON(ctx, chManifestsDir+"/"+m.Name+".json", m)
}

// counts is every data table's row count (views hold no rows).
func (c *ClickHouse) counts(ctx context.Context, database string) (map[string]uint64, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT name FROM system.tables
		WHERE database = ? AND engine NOT LIKE '%View' AND NOT is_temporary`, database)
	if err != nil {
		return nil, fmt.Errorf("backup: listing clickhouse tables: %w", err)
	}
	var tables []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return nil, err
		}
		tables = append(tables, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make(map[string]uint64, len(tables))
	for _, t := range tables {
		var n uint64
		if err := c.conn.QueryRow(ctx, "SELECT count() FROM "+ident(database)+"."+ident(t)).Scan(&n); err != nil {
			return nil, fmt.Errorf("backup: counting %s.%s: %w", database, t, err)
		}
		out[t] = n
	}
	return out, nil
}

// Latest is the newest manifest.
func (c *ClickHouse) Latest(ctx context.Context) (ClickHouseManifest, error) {
	var m ClickHouseManifest
	_, err := c.bucket.latest(ctx, chManifestsDir, &m)
	return m, err
}

// List is every kept backup's name, oldest first.
func (c *ClickHouse) List(ctx context.Context) ([]string, error) {
	return c.bucket.names(ctx, chManifestsDir)
}

// Prune deletes all but the newest keep backups. The manifest goes last,
// so a prune cut short leaves a listed backup to delete next time rather
// than unlisted files.
func (c *ClickHouse) Prune(ctx context.Context, keep int) ([]string, error) {
	all, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	gone := oldest(all, keep)
	for _, n := range gone {
		if err := c.bucket.removePrefix(ctx, chDir+"/"+n+"/"); err != nil {
			return nil, err
		}
		if err := c.bucket.c.RemoveObject(ctx, c.bucket.name, chManifestsDir+"/"+n+".json", minioRemove); err != nil {
			return nil, fmt.Errorf("backup: removing manifest %s: %w", n, err)
		}
	}
	return gone, nil
}

// Drill restores the newest backup as <database>_drill, checks every
// table's rows against the manifest and drops the copy.
func (c *ClickHouse) Drill(ctx context.Context) []Check {
	m, err := c.Latest(ctx)
	if err != nil {
		return []Check{fail(StoreClickHouse, "latest backup", err)}
	}
	checks := []Check{pass(StoreClickHouse, "latest backup", fmt.Sprintf("%s, %s old", m.Name, time.Since(m.StartedAt).Round(time.Minute)))}

	drill := m.Database + "_drill"
	if err := c.conn.Exec(ctx, "DROP DATABASE IF EXISTS "+ident(drill)+" SYNC"); err != nil {
		return append(checks, fail(StoreClickHouse, "restore", err))
	}
	defer func() { _ = c.conn.Exec(context.WithoutCancel(ctx), "DROP DATABASE IF EXISTS "+ident(drill)+" SYNC") }()
	started := time.Now()
	if err := c.conn.Exec(ctx, "RESTORE DATABASE "+ident(m.Database)+" AS "+ident(drill)+" FROM "+c.s3(m.Name)); err != nil {
		return append(checks, fail(StoreClickHouse, "restore", err))
	}
	checks = append(checks, pass(StoreClickHouse, "restore", fmt.Sprintf("into %s in %s", drill, time.Since(started).Round(time.Second))))

	got, err := c.counts(ctx, drill)
	if err != nil {
		return append(checks, fail(StoreClickHouse, "row counts", err))
	}
	return append(checks, compareCounts(m.Tables, got)...)
}

// compareCounts checks a restored database's tables against a manifest.
func compareCounts(want map[string]RowsBetween, got map[string]uint64) []Check {
	names := make([]string, 0, len(want))
	for t := range want {
		names = append(names, t)
	}
	sort.Strings(names)
	var checks []Check
	for _, t := range names {
		w := want[t]
		n, ok := got[t]
		switch {
		case !ok:
			checks = append(checks, Check{Store: StoreClickHouse, Name: t, Detail: "table missing from the restore"})
		case !w.Holds(n):
			checks = append(checks, Check{Store: StoreClickHouse, Name: t,
				Detail: fmt.Sprintf("%d rows restored, backup saw %d–%d", n, min(w.Before, w.After), max(w.Before, w.After))})
		default:
			checks = append(checks, pass(StoreClickHouse, t, fmt.Sprintf("%d rows", n)))
		}
	}
	return checks
}
