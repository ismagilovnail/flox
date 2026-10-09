package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

const (
	pgDir = "postgres"
	// drillPort is the scratch server's port; it listens on a socket in
	// the drill directory only.
	drillPort = "5433"
	// archiveWait bounds the wait for the drill's WAL segment to reach the
	// bucket.
	archiveWait = 5 * time.Minute
	// recoveryWait bounds the replay of the archived WAL.
	recoveryWait = 2 * time.Hour
)

// Postgres drives WAL-G against the live server and its data directory.
type Postgres struct {
	dsn      string
	pgdata   string
	drillDir string
	env      []string // WAL-G's own: storage and connection
}

func NewPostgres(dsn, pgdata, drillDir string, t Target) (*Postgres, error) {
	u, err := url.Parse(dsn)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("backup: database url must be postgres://user:pass@host:port/db")
	}
	pass, _ := u.User.Password()
	env := []string{
		"WALG_S3_PREFIX=s3://" + t.Bucket + "/" + pgDir,
		"AWS_ENDPOINT=" + t.Endpoint,
		"AWS_REGION=" + t.Region,
		"AWS_S3_FORCE_PATH_STYLE=true",
		"AWS_ACCESS_KEY_ID=" + t.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY=" + t.SecretAccessKey,
		"PGHOST=" + u.Hostname(),
		"PGPORT=" + firstNonEmpty(u.Port(), "5432"),
		"PGUSER=" + u.User.Username(),
		"PGPASSWORD=" + pass,
		"PGDATABASE=" + strings.TrimPrefix(u.Path, "/"),
		"PGSSLMODE=" + firstNonEmpty(u.Query().Get("sslmode"), "prefer"),
	}
	return &Postgres{dsn: dsn, pgdata: pgdata, drillDir: drillDir, env: env}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// walg runs one WAL-G command, returning its stdout.
func (p *Postgres) walg(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "wal-g", args...)
	cmd.Env = append(os.Environ(), p.env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("backup: wal-g %s: %w: %s", args[0], err, tail(stderr.String(), 800))
	}
	return stdout.Bytes(), nil
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}

// Backup takes a base backup of the live data directory.
func (p *Postgres) Backup(ctx context.Context) error {
	_, err := p.walg(ctx, "backup-push", p.pgdata)
	return err
}

// PGBackup is one base backup as WAL-G lists it.
type PGBackup struct {
	Name             string    `json:"backup_name"`
	StartTime        time.Time `json:"start_time"`
	FinishTime       time.Time `json:"finish_time"`
	CompressedSize   int64     `json:"compressed_size"`
	UncompressedSize int64     `json:"uncompressed_size"`
}

// List is every base backup, oldest first.
func (p *Postgres) List(ctx context.Context) ([]PGBackup, error) {
	out, err := p.walg(ctx, "backup-list", "--json", "--detail")
	if err != nil {
		return nil, err
	}
	out = bytes.TrimSpace(out)
	if len(out) == 0 || !bytes.HasPrefix(out, []byte("[")) {
		return nil, nil // "No backups found"
	}
	var list []PGBackup
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("backup: reading wal-g backup-list: %w", err)
	}
	return list, nil
}

// Prune keeps the newest keep base backups and the WAL they need.
func (p *Postgres) Prune(ctx context.Context, keep int) error {
	_, err := p.walg(ctx, "delete", "retain", "FULL", fmt.Sprint(keep), "--confirm")
	return err
}

// Archiver is pg_stat_archiver: whether WAL reaches the bucket.
type Archiver struct {
	LastArchived, LastFailed time.Time
	LastArchivedWAL          string
	Failed                   int64
}

// Failing: the most recent attempt failed. An idle database archives
// nothing for hours, so the age of the last success alone says nothing.
func (a Archiver) Failing() bool { return a.LastFailed.After(a.LastArchived) }

func (p *Postgres) Archiver(ctx context.Context) (Archiver, error) {
	conn, err := pgx.Connect(ctx, p.dsn)
	if err != nil {
		return Archiver{}, fmt.Errorf("backup: connecting to postgres: %w", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	var a Archiver
	var archived, failed *time.Time
	var wal *string
	if err := conn.QueryRow(ctx, `SELECT last_archived_time, last_failed_time, last_archived_wal, failed_count FROM pg_stat_archiver`).
		Scan(&archived, &failed, &wal, &a.Failed); err != nil {
		return Archiver{}, fmt.Errorf("backup: reading pg_stat_archiver: %w", err)
	}
	if archived != nil {
		a.LastArchived = *archived
	}
	if failed != nil {
		a.LastFailed = *failed
	}
	if wal != nil {
		a.LastArchivedWAL = *wal
	}
	return a, nil
}

// Drill restores the newest base backup plus every archived WAL segment
// into a scratch server and checks it against the live database. Just
// before, it writes a mark into the live database and has its WAL segment
// archived: finding the mark proves the restore reaches the present, not
// only last night's base backup.
func (p *Postgres) Drill(ctx context.Context) []Check {
	live, err := pgx.Connect(ctx, p.dsn)
	if err != nil {
		return []Check{fail(StorePostgres, "live database", err)}
	}
	defer live.Close(context.WithoutCancel(ctx))

	mark, err := p.markAndArchive(ctx, live)
	if err != nil {
		return []Check{fail(StorePostgres, "wal archiving", err)}
	}
	checks := []Check{pass(StorePostgres, "wal archiving", "drill mark's segment archived")}

	// A subdirectory: drillDir itself is usually a mount point.
	work := filepath.Join(p.drillDir, "run")
	dir, sock, serverLog := filepath.Join(work, "pg"), filepath.Join(work, "sock"), filepath.Join(work, "server.log")
	_ = os.RemoveAll(work)
	defer func() { _ = os.RemoveAll(work) }()
	if err := os.MkdirAll(sock, 0o700); err != nil {
		return append(checks, fail(StorePostgres, "restore", err))
	}
	started := time.Now()
	if _, err := p.walg(ctx, "backup-fetch", dir, "LATEST"); err != nil {
		return append(checks, fail(StorePostgres, "restore", err))
	}
	if err := p.prepareRecovery(dir, sock); err != nil {
		return append(checks, fail(StorePostgres, "restore", err))
	}
	if err := p.pgctl(ctx, dir, "start", "-w", "-t", "600", "-l", serverLog); err != nil {
		return append(checks, fail(StorePostgres, "restore", fmt.Errorf("%w; server log: %s", err, readTail(serverLog))))
	}
	defer func() { _ = p.pgctl(context.WithoutCancel(ctx), dir, "stop", "-m", "fast", "-w") }()

	u, _ := url.Parse(p.dsn)
	restored, err := pgx.Connect(ctx, fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		sock, drillPort, u.User.Username(), strings.TrimPrefix(u.Path, "/")))
	if err != nil {
		return append(checks, fail(StorePostgres, "restore", err))
	}
	defer restored.Close(context.WithoutCancel(ctx))
	if err := waitRecovered(ctx, restored); err != nil {
		return append(checks, fail(StorePostgres, "restore", fmt.Errorf("%w; server log: %s", err, readTail(serverLog))))
	}
	checks = append(checks, pass(StorePostgres, "restore", fmt.Sprintf("base backup + WAL replayed in %s", time.Since(started).Round(time.Second))))

	var tables []string
	err = restored.QueryRow(ctx, `SELECT tables FROM backup_drill_marks WHERE token = $1`, mark).Scan(&tables)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return append(checks, Check{Store: StorePostgres, Name: "point in time", Detail: "the mark written before the drill is not in the restore: WAL after the base backup was not replayed"})
	case err != nil:
		return append(checks, fail(StorePostgres, "point in time", err))
	}
	checks = append(checks, pass(StorePostgres, "point in time", "restore reaches the moment the drill started"))
	return append(checks, compareDatabases(ctx, live, restored, tables)...)
}

// markAndArchive writes the drill's mark, switches to a new WAL segment
// and waits until the one holding the mark is archived.
func (p *Postgres) markAndArchive(ctx context.Context, live *pgx.Conn) (string, error) {
	mark := idgen.New()
	// The tables are taken with the mark: a table made after it (a test's
	// scratch copy) is rightly absent from the restore.
	if _, err := live.Exec(ctx, `
		INSERT INTO backup_drill_marks (token, tables)
		SELECT $1, coalesce(array_agg(tablename::text ORDER BY tablename), '{}') FROM pg_tables WHERE schemaname = 'public'`, mark); err != nil {
		return "", fmt.Errorf("writing the drill mark: %w", err)
	}
	var segment string
	if err := live.QueryRow(ctx, `SELECT pg_walfile_name(pg_switch_wal())`).Scan(&segment); err != nil {
		return "", fmt.Errorf("switching wal: %w", err)
	}
	deadline := time.Now().Add(archiveWait)
	for {
		var last *string
		var failed *time.Time
		if err := live.QueryRow(ctx, `SELECT last_archived_wal, last_failed_time FROM pg_stat_archiver`).Scan(&last, &failed); err != nil {
			return "", err
		}
		// Segment names sort in WAL order within a timeline.
		if last != nil && len(*last) == len(segment) && *last >= segment {
			return mark, nil
		}
		if time.Now().After(deadline) {
			if failed != nil {
				return "", fmt.Errorf("segment %s not archived after %s: archiving fails (last failure %s, last archived %s) — see wal-g wal-push in the postgres log",
					segment, archiveWait, failed.UTC().Format(time.DateTime), deref(last))
			}
			return "", fmt.Errorf("segment %s not archived after %s and no attempt failed — is archive_mode on?", segment, archiveWait)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func deref(s *string) string {
	if s == nil {
		return "none"
	}
	return *s
}

// prepareRecovery makes a fetched data directory replay the archive and
// come up on a private socket.
func (p *Postgres) prepareRecovery(dir, sock string) error {
	if err := os.WriteFile(filepath.Join(dir, "recovery.signal"), nil, 0o600); err != nil {
		return err
	}
	conf := strings.Join([]string{
		"restore_command = 'wal-g wal-fetch %f %p'",
		"recovery_target_timeline = 'latest'",
		"recovery_target_action = 'promote'",
		"archive_mode = off",
		"listen_addresses = ''",
		"port = " + drillPort,
		"unix_socket_directories = '" + sock + "'",
		"hot_standby = on",
		"",
	}, "\n")
	f, err := os.OpenFile(filepath.Join(dir, "postgresql.auto.conf"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString("\n# flox backup drill\n" + conf); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// The live pg_hba.conf may require passwords or TLS on this host; the
	// scratch server takes local socket connections only.
	return os.WriteFile(filepath.Join(dir, "pg_hba.conf"), []byte("local all all trust\n"), 0o600)
}

func (p *Postgres) pgctl(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "pg_ctl", append([]string{"-D", dir}, args...)...)
	cmd.Env = append(os.Environ(), p.env...) // restore_command runs wal-g
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pg_ctl %s: %w: %s", args[0], err, tail(string(out), 400))
	}
	return nil
}

func readTail(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "(none)"
	}
	return tail(string(b), 800)
}

// waitRecovered waits until the scratch server has replayed the archive
// and promoted itself.
func waitRecovered(ctx context.Context, conn *pgx.Conn) error {
	deadline := time.Now().Add(recoveryWait)
	for {
		var inRecovery bool
		if err := conn.QueryRow(ctx, `SELECT pg_is_in_recovery()`).Scan(&inRecovery); err != nil {
			return err
		}
		if !inRecovery {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("still recovering after " + recoveryWait.String())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// compareDatabases checks the restore against the live database: the same
// schema version, and every table there was at the mark. Row counts are
// reported, not compared — the live database has moved on since.
func compareDatabases(ctx context.Context, live, restored *pgx.Conn, atMark []string) []Check {
	var checks []Check
	versions := [2]int64{}
	for i, c := range []*pgx.Conn{live, restored} {
		if err := c.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&versions[i]); err != nil {
			return append(checks, fail(StorePostgres, "schema version", err))
		}
	}
	if versions[0] != versions[1] {
		checks = append(checks, Check{Store: StorePostgres, Name: "schema version",
			Detail: fmt.Sprintf("live %d, restored %d", versions[0], versions[1])})
	} else {
		checks = append(checks, pass(StorePostgres, "schema version", fmt.Sprint(versions[0])))
	}

	counts, err := tableCounts(ctx, restored)
	if err != nil {
		return append(checks, fail(StorePostgres, "tables", err))
	}
	return append(checks, missingTables(atMark, counts))
}

// missingTables checks that every table of the mark was restored.
func missingTables(atMark []string, restored map[string]int64) Check {
	var missing []string
	for _, t := range atMark {
		if _, ok := restored[t]; !ok {
			missing = append(missing, t)
		}
	}
	if len(missing) > 0 {
		return Check{Store: StorePostgres, Name: "tables", Detail: "missing from the restore: " + strings.Join(missing, ", ")}
	}
	var rows int64
	for _, n := range restored {
		rows += n
	}
	return pass(StorePostgres, "tables", fmt.Sprintf("all %d tables, %d rows", len(atMark), rows))
}

// tableCounts is every public table's exact row count.
func tableCounts(ctx context.Context, c *pgx.Conn) (map[string]int64, error) {
	rows, err := c.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname = 'public'`)
	if err != nil {
		return nil, err
	}
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(tables))
	for _, t := range tables {
		var n int64
		if err := c.QueryRow(ctx, `SELECT count(*) FROM `+pgx.Identifier{t}.Sanitize()).Scan(&n); err != nil {
			return nil, fmt.Errorf("counting %s: %w", t, err)
		}
		out[t] = n
	}
	return out, nil
}
