package orgexport

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Interval is how often apps/worker looks for exports to build or expire.
const Interval = time.Minute

// staleAfter: a running export older than this was left by a worker that
// stopped mid-way; it is failed so the organization can ask again.
const staleAfter = 2 * time.Hour

// SecretColumn matches the columns an export leaves out: credentials,
// tokens and their hashes, encrypted payloads, push keys. A push
// subscription's endpoint URL is a sending credential too.
var SecretColumn = regexp.MustCompile(`(?i)(secret|token|password|hash|encrypt|p256dh|^auth$|^endpoint$|private_key)`)

// ExcludedTables are tenant tables an export leaves out entirely, and why.
var ExcludedTables = map[string]string{
	"sessions":               "sign-in state",
	"pending_totp_logins":    "sign-in state",
	"event_queue":            "an internal queue",
	"postback_attempt_queue": "an internal queue",
	"attribution_retries":    "an internal queue",
	"organization_deletions": "has no rows while the organization exists",
	"organization_exports":   "the exports themselves; their object keys are what keeps the archives private",
}

// testCopy matches internal/testpg.PrivateTable's copies, which exist only
// in a test database.
var testCopy = regexp.MustCompile(`_t_[0-9a-z]{26}$`)

// eventTables are the ClickHouse tables an export carries for its period,
// with the column their period is on.
var eventTables = []struct{ name, timeColumn string }{
	{"click_events", "event_at"},
	{"tracking_events", "event_at"},
	{"conversion_events", "event_at"},
	{"postback_events", "event_at"},
	{"cost_events", "date"},
}

// Objects stores and removes archives; *storage.Client is the real one.
type Objects interface {
	PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	DeleteObject(ctx context.Context, key string) error
}

// Mailer tells the requester the export is ready.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// Runner is apps/worker's half. CH nil: the archive has no events and its
// README says so. Mail nil: no email.
type Runner struct {
	DB      *pgxpool.Pool
	CH      driver.Conn
	Objects Objects
	Mail    Mailer
	AppURL  string
	Logger  *slog.Logger
	// Organization limits a run to one organization — for tests.
	Organization string
}

func (r *Runner) RunLoop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if err := r.RunOnce(ctx); err != nil && ctx.Err() == nil {
			r.Logger.Error("organization export run failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RunOnce expires old archives, fails abandoned builds, then builds every
// pending export.
func (r *Runner) RunOnce(ctx context.Context) error {
	if err := r.expire(ctx); err != nil {
		return err
	}
	if _, err := r.DB.Exec(ctx, `
		UPDATE organization_exports SET status = 'failed', error = 'interrupted', finished_at = now()
		WHERE status = 'running' AND started_at < now() - make_interval(secs => $1) AND ($2 = '' OR organization_id = $2)`,
		staleAfter.Seconds(), r.Organization); err != nil {
		return fmt.Errorf("orgexport: failing stale exports: %w", err)
	}
	for {
		job, ok, err := r.claim(ctx)
		if err != nil || !ok {
			return err
		}
		r.run(ctx, job)
	}
}

type job struct {
	id, orgID, requestedBy string
	from, to               time.Time
}

func (r *Runner) claim(ctx context.Context) (job, bool, error) {
	var j job
	var by *string
	err := r.DB.QueryRow(ctx, `
		UPDATE organization_exports SET status = 'running', started_at = now()
		WHERE id = (SELECT id FROM organization_exports
		            WHERE status = 'pending' AND ($1 = '' OR organization_id = $1)
		            ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING id, organization_id, requested_by, from_date, to_date`, r.Organization).Scan(&j.id, &j.orgID, &by, &j.from, &j.to)
	if errors.Is(err, pgx.ErrNoRows) {
		return job{}, false, nil
	}
	if err != nil {
		return job{}, false, fmt.Errorf("orgexport: claiming: %w", err)
	}
	if by != nil {
		j.requestedBy = *by
	}
	return j, true, nil
}

// run builds one export and records the outcome; a failure is stored on
// the row, not returned, so one broken export does not stop the others.
func (r *Runner) run(ctx context.Context, j job) {
	key, size, err := r.build(ctx, j)
	if err != nil {
		r.Logger.Error("organization export failed", "export_id", j.id, "organization_id", j.orgID, "error", err)
		if _, uerr := r.DB.Exec(ctx, `UPDATE organization_exports SET status = 'failed', error = $2, finished_at = now() WHERE id = $1`,
			j.id, err.Error()); uerr != nil {
			r.Logger.Error("organization export: recording failure", "export_id", j.id, "error", uerr)
		}
		return
	}
	expires := time.Now().Add(Retention)
	if _, err := r.DB.Exec(ctx, `
		UPDATE organization_exports SET status = 'done', object_key = $2, size_bytes = $3, finished_at = now(), expires_at = $4
		WHERE id = $1`, j.id, key, size, expires); err != nil {
		r.Logger.Error("organization export: recording success", "export_id", j.id, "error", err)
		return
	}
	r.Logger.Info("organization export ready", "export_id", j.id, "organization_id", j.orgID, "bytes", size)
	r.notify(ctx, j, expires)
}

func (r *Runner) notify(ctx context.Context, j job, expires time.Time) {
	if r.Mail == nil || j.requestedBy == "" {
		return
	}
	var email string
	if err := r.DB.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, j.requestedBy).Scan(&email); err != nil {
		return
	}
	body := fmt.Sprintf("Your FLOX data export (events %s to %s) is ready. Download it from %s/settings until %s.\n",
		j.from.Format(time.DateOnly), j.to.Format(time.DateOnly), r.AppURL, expires.UTC().Format("2 Jan 2006 15:04 MST"))
	if err := r.Mail.Send(ctx, email, "FLOX: your data export is ready", body); err != nil {
		r.Logger.Warn("organization export: notifying", "export_id", j.id, "error", err)
	}
}

// expire deletes the archives past their date.
func (r *Runner) expire(ctx context.Context) error {
	rows, err := r.DB.Query(ctx, `
		SELECT id, object_key FROM organization_exports
		WHERE status = 'done' AND expires_at <= now() AND ($1 = '' OR organization_id = $1)`, r.Organization)
	if err != nil {
		return fmt.Errorf("orgexport: finding expired exports: %w", err)
	}
	type expired struct{ id, key string }
	var list []expired
	for rows.Next() {
		var e expired
		if err := rows.Scan(&e.id, &e.key); err != nil {
			rows.Close()
			return fmt.Errorf("orgexport: reading expired exports: %w", err)
		}
		list = append(list, e)
	}
	rows.Close()
	for _, e := range list {
		if r.Objects != nil {
			if err := r.Objects.DeleteObject(ctx, e.key); err != nil {
				return fmt.Errorf("orgexport: deleting archive of %s: %w", e.id, err)
			}
		}
		if _, err := r.DB.Exec(ctx, `UPDATE organization_exports SET status = 'expired', object_key = '' WHERE id = $1`, e.id); err != nil {
			return fmt.Errorf("orgexport: marking %s expired: %w", e.id, err)
		}
	}
	return nil
}

// build writes the archive to a temporary file and stores it.
func (r *Runner) build(ctx context.Context, j job) (key string, size int64, err error) {
	if r.Objects == nil {
		return "", 0, errors.New("object storage is not configured")
	}
	f, err := os.CreateTemp("", "flox-export-*.zip")
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	defer f.Close()

	zw := zip.NewWriter(f)
	if err := Write(ctx, zw, r.DB, r.CH, j.orgID, j.from, j.to); err != nil {
		return "", 0, err
	}
	if err := zw.Close(); err != nil {
		return "", 0, err
	}
	st, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", 0, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", 0, err
	}
	key = "exports/" + j.orgID + "/" + j.id + "-" + hex.EncodeToString(secret) + ".zip"
	if err := r.Objects.PutObject(ctx, key, f, st.Size(), "application/zip"); err != nil {
		return "", 0, err
	}
	return key, st.Size(), nil
}

// Write puts orgID's data into zw: a README, every tenant table as JSON
// lines without its secret columns, the team, and the ClickHouse events
// from from to to (inclusive).
func Write(ctx context.Context, zw *zip.Writer, db *pgxpool.Pool, ch driver.Conn, orgID string, from, to time.Time) error {
	tables, err := tenantTables(ctx, db)
	if err != nil {
		return err
	}
	var notes []string
	for _, t := range tables {
		if err := writeTable(ctx, zw, db, orgID, t); err != nil {
			return err
		}
	}
	if err := writeTeam(ctx, zw, db, orgID); err != nil {
		return err
	}
	if ch == nil {
		notes = append(notes, "ClickHouse was not available: this archive has no events.")
	} else {
		for _, et := range eventTables {
			if err := writeEvents(ctx, zw, ch, orgID, et.name, et.timeColumn, from, to); err != nil {
				return err
			}
		}
	}
	return writeReadme(zw, from, to, tables, notes)
}

type table struct {
	name     string
	excluded []string
}

func tenantTables(ctx context.Context, db *pgxpool.Pool) ([]table, error) {
	rows, err := db.Query(ctx, `
		SELECT c.table_name, array_agg(c2.column_name ORDER BY c2.ordinal_position)
		FROM information_schema.columns c
		JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name AND t.table_type = 'BASE TABLE'
		JOIN information_schema.columns c2 ON c2.table_schema = c.table_schema AND c2.table_name = c.table_name
		WHERE c.table_schema = 'public' AND c.column_name = 'organization_id'
		GROUP BY c.table_name ORDER BY c.table_name`)
	if err != nil {
		return nil, fmt.Errorf("orgexport: listing tables: %w", err)
	}
	defer rows.Close()
	var out []table
	for rows.Next() {
		var name string
		var cols []string
		if err := rows.Scan(&name, &cols); err != nil {
			return nil, fmt.Errorf("orgexport: listing tables: %w", err)
		}
		if _, skip := ExcludedTables[name]; skip || testCopy.MatchString(name) {
			continue
		}
		t := table{name: name, excluded: []string{}}
		for _, c := range cols {
			if SecretColumn.MatchString(c) {
				t.excluded = append(t.excluded, c)
			}
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func writeTable(ctx context.Context, zw *zip.Writer, db *pgxpool.Pool, orgID string, t table) error {
	w, err := zw.Create("postgres/" + t.name + ".jsonl")
	if err != nil {
		return err
	}
	rows, err := db.Query(ctx, `SELECT (to_jsonb(x) - $2::text[])::text FROM "`+t.name+`" x WHERE organization_id = $1`, orgID, t.excluded)
	if isDropped(err) {
		return nil // dropped since it was listed (a test's temporary table)
	}
	if err != nil {
		return fmt.Errorf("orgexport: reading %s: %w", t.name, err)
	}
	defer rows.Close()
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return fmt.Errorf("orgexport: reading %s: %w", t.name, err)
		}
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			return err
		}
	}
	if isDropped(rows.Err()) {
		return nil
	}
	return rows.Err()
}

// isDropped reports a table that no longer exists (undefined_table).
func isDropped(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42P01"
}

func writeTeam(ctx context.Context, zw *zip.Writer, db *pgxpool.Pool, orgID string) error {
	w, err := zw.Create("team.jsonl")
	if err != nil {
		return err
	}
	rows, err := db.Query(ctx, `
		SELECT jsonb_build_object('name', u.name, 'email', u.email, 'role', r.key, 'status', m.status,
		                          'invitedAt', m.invited_at, 'lastActiveAt', m.last_active_at)::text
		FROM memberships m JOIN users u ON u.id = m.user_id JOIN roles r ON r.id = m.role_id
		WHERE m.organization_id = $1 ORDER BY m.created_at`, orgID)
	if err != nil {
		return fmt.Errorf("orgexport: reading the team: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return fmt.Errorf("orgexport: reading the team: %w", err)
		}
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			return err
		}
	}
	return rows.Err()
}

// writeEvents writes one ClickHouse table's rows of the period as CSV,
// every column as ClickHouse prints it (toString), in schema order.
func writeEvents(ctx context.Context, zw *zip.Writer, ch driver.Conn, orgID, name, timeColumn string, from, to time.Time) error {
	crows, err := ch.Query(ctx, `SELECT name FROM system.columns WHERE database = currentDatabase() AND table = ? ORDER BY position`, name)
	if err != nil {
		return fmt.Errorf("orgexport: reading %s columns: %w", name, err)
	}
	var cols []string
	for crows.Next() {
		var c string
		if err := crows.Scan(&c); err != nil {
			crows.Close()
			return fmt.Errorf("orgexport: reading %s columns: %w", name, err)
		}
		cols = append(cols, c)
	}
	crows.Close()
	if len(cols) == 0 {
		return nil
	}
	exprs := make([]string, len(cols))
	for i, c := range cols {
		exprs[i] = "toString(`" + c + "`)"
	}
	rows, err := ch.Query(ctx, "SELECT "+strings.Join(exprs, ", ")+" FROM `"+name+"`"+
		" WHERE organization_id = ? AND toDate(`"+timeColumn+"`) >= ? AND toDate(`"+timeColumn+"`) <= ?"+
		" ORDER BY `"+timeColumn+"`", orgID, from.Format(time.DateOnly), to.Format(time.DateOnly))
	if err != nil {
		return fmt.Errorf("orgexport: reading %s: %w", name, err)
	}
	defer rows.Close()
	w, err := zw.Create("clickhouse/" + name + ".csv")
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(cols); err != nil {
		return err
	}
	vals := make([]string, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return fmt.Errorf("orgexport: reading %s: %w", name, err)
		}
		if err := cw.Write(vals); err != nil {
			return err
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return err
	}
	return rows.Err()
}

func writeReadme(zw *zip.Writer, from, to time.Time, tables []table, notes []string) error {
	w, err := zw.Create("README.txt")
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "FLOX data export\n\nEvents from %s to %s (inclusive), exported %s.\n\n",
		from.Format(time.DateOnly), to.Format(time.DateOnly), time.Now().UTC().Format(time.RFC3339))
	b.WriteString("postgres/*.jsonl  every record of the organization, one JSON object per line.\n")
	b.WriteString("team.jsonl        members: name, email, role, status.\n")
	b.WriteString("clickhouse/*.csv  clicks, visits, conversions, postbacks and costs of the period.\n\n")
	b.WriteString("Left out: credentials, tokens and their hashes, encrypted payloads, push keys and endpoints, and\n")
	b.WriteString("internal state (sessions, queues). Uploaded files (landings, gallery) are not included.\n")
	b.WriteString("Visitors' IP addresses, user agents and referrers are deleted after the retention period (90 days\n")
	b.WriteString("by default); older events have them empty.\n\nColumns left out per table:\n")
	for _, t := range tables {
		if len(t.excluded) > 0 {
			fmt.Fprintf(&b, "  %s: %s\n", t.name, strings.Join(t.excluded, ", "))
		}
	}
	for _, n := range notes {
		b.WriteString("\n" + n + "\n")
	}
	_, err = io.WriteString(w, b.String())
	return err
}
