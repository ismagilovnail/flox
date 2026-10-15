// Command devpanel is FLOX's local development panel: a dashboard of how
// far the work is, and a task tracker for findings, bugs and tasks — first
// filled from docs/audit-product-2026-10-15.md. It keeps its state in a
// SQLite file next to the product and is deliberately outside the apps/ Go
// module, so its dependencies never reach the product's go.mod. Customers
// never see it; it is not part of FLOX.
//
//	make devpanel                      # http://127.0.0.1:8099
//	cd tools/devpanel && go run . -addr 127.0.0.1:9000 -db data/x.db
//
// On first start (empty database) it seeds the items and the product map
// from seed/*.json. Re-seeding never overwrites what you changed: use
// -reseed to add seed rows that are missing, nothing else.
package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed web
var webFS embed.FS

//go:embed seed/items.json seed/features.json
var seedFS embed.FS

var (
	priorities = map[string]bool{"P0": true, "P1": true, "P2": true, "P3": true}
	severities = map[string]bool{"critical": true, "high": true, "medium": true, "low": true}
	statuses   = map[string]bool{"todo": true, "doing": true, "blocked": true, "done": true, "wontfix": true}
	kinds      = map[string]bool{"security": true, "bug": true, "data": true, "ux": true, "gap": true, "ops": true, "risk": true, "business": true}
	featStates = map[string]bool{"PASS": true, "PARTIAL": true, "FAIL": true, "NOT_IMPLEMENTED": true, "NOT_VERIFIED": true}

	codePattern = regexp.MustCompile(`^[A-Z]{2,8}-[0-9]{1,4}$`)
)

type Item struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	Priority  string `json:"priority"`
	Severity  string `json:"severity"`
	Kind      string `json:"kind"`
	Area      string `json:"area"`
	Title     string `json:"title"`
	Evidence  string `json:"evidence"`
	Fix       string `json:"fix"`
	Impact    string `json:"impact"`
	Status    string `json:"status"`
	Notes     string `json:"notes"`
	Order     int    `json:"order"`
	Source    string `json:"source"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type Feature struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Evidence  string `json:"evidence"`
	Limits    string `json:"limits"`
	Order     int    `json:"order"`
	UpdatedAt string `json:"updatedAt"`
}

type Change struct {
	ItemID string `json:"itemId"`
	Field  string `json:"field"`
	Old    string `json:"old"`
	New    string `json:"new"`
	At     string `json:"at"`
}

const schema = `
CREATE TABLE IF NOT EXISTS items (
  id TEXT PRIMARY KEY,
  code TEXT NOT NULL,
  priority TEXT NOT NULL,
  severity TEXT NOT NULL,
  kind TEXT NOT NULL,
  area TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  evidence TEXT NOT NULL DEFAULT '',
  fix TEXT NOT NULL DEFAULT '',
  impact TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  notes TEXT NOT NULL DEFAULT '',
  ord INTEGER NOT NULL DEFAULT 0,
  source TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS features (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  status TEXT NOT NULL,
  evidence TEXT NOT NULL DEFAULT '',
  limits TEXT NOT NULL DEFAULT '',
  ord INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS item_changes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  item_id TEXT NOT NULL,
  field TEXT NOT NULL,
  old TEXT NOT NULL,
  new TEXT NOT NULL,
  at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS item_changes_item ON item_changes (item_id, id);
`

// openDB opens (creating) the SQLite file and applies the schema.
func openDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one writer; SQLite serializes anyway
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	return db, nil
}

// newHandler is the whole HTTP surface: the panel's static files and its
// JSON API, behind the loopback-only guard.
func newHandler(db *sql.DB) http.Handler {
	s := &server{db: db, now: time.Now, loc: time.Local}
	mux := http.NewServeMux()
	static, _ := fs.Sub(webFS, "web")
	files := http.FileServerFS(static)
	mux.Handle("GET /{$}", noStore(files))
	mux.Handle("GET /app.js", noStore(files))
	mux.Handle("GET /app.css", noStore(files))
	mux.HandleFunc("GET /api/stats", s.stats)
	mux.HandleFunc("GET /api/items", s.listItems)
	mux.HandleFunc("POST /api/items", s.createItem)
	mux.HandleFunc("PATCH /api/items/{id}", s.patchItem)
	mux.HandleFunc("DELETE /api/items/{id}", s.deleteItem)
	mux.HandleFunc("GET /api/items/{id}/changes", s.itemChanges)
	mux.HandleFunc("GET /api/features", s.listFeatures)
	mux.HandleFunc("PATCH /api/features/{id}", s.patchFeature)
	return localOnly(mux)
}

// noStore keeps the browser from holding on to an old app.js after an
// update; everything is local, so caching buys nothing.
func noStore(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8099", "listen address (keep it on loopback: there is no login)")
	dbPath := flag.String("db", "data/tracker.db", "SQLite file")
	reseed := flag.Bool("reseed", false, "insert seed rows that are missing (never overwrites existing rows)")
	flag.Parse()

	db, err := openDB(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM items`).Scan(&n); err != nil {
		log.Fatal(err)
	}
	if n == 0 || *reseed {
		added, err := seed(db)
		if err != nil {
			log.Fatalf("seed: %v", err)
		}
		log.Printf("seed: %d rows added", added)
	}

	srv := &http.Server{Addr: *addr, Handler: newHandler(db), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("FLOX dev panel: http://%s  (db %s)", *addr, *dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	_ = db.Close()
}

// localOnly refuses requests whose Host is not a loopback name (DNS
// rebinding) and writes that are not same-origin JSON (another site the
// browser has open posting to 127.0.0.1). There is no login, so this is
// the whole access control.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != "localhost" && host != "127.0.0.1" && host != "::1" && host != "[::1]" {
			http.Error(w, "local use only", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if origin := r.Header.Get("Origin"); origin != "" && !strings.HasSuffix(origin, "://"+r.Host) {
				http.Error(w, "cross-origin write refused", http.StatusForbidden)
				return
			}
			if r.Method != http.MethodDelete && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				http.Error(w, "send application/json", http.StatusUnsupportedMediaType)
				return
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func now() string { return time.Now().Format(time.RFC3339) }

func seed(db *sql.DB) (int, error) {
	var items []Item
	var feats []Feature
	for name, dst := range map[string]any{"seed/items.json": &items, "seed/features.json": &feats} {
		b, err := fs.ReadFile(seedFS, name)
		if err != nil {
			return 0, err
		}
		if err := json.Unmarshal(b, dst); err != nil {
			return 0, fmt.Errorf("%s: %w", name, err)
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	added := 0
	for _, i := range items {
		res, err := tx.Exec(`INSERT OR IGNORE INTO items (id, code, priority, severity, kind, area, title, evidence, fix, impact, status, notes, ord, source, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			i.ID, i.Code, i.Priority, i.Severity, i.Kind, i.Area, i.Title, i.Evidence, i.Fix, i.Impact, i.Status, i.Notes, i.Order, i.Source, i.CreatedAt, i.UpdatedAt)
		if err != nil {
			return 0, err
		}
		k, _ := res.RowsAffected()
		added += int(k)
	}
	for _, f := range feats {
		res, err := tx.Exec(`INSERT OR IGNORE INTO features (id, name, status, evidence, limits, ord, updated_at) VALUES (?,?,?,?,?,?,?)`,
			f.ID, f.Name, f.Status, f.Evidence, f.Limits, f.Order, f.UpdatedAt)
		if err != nil {
			return 0, err
		}
		k, _ := res.RowsAffected()
		added += int(k)
	}
	return added, tx.Commit()
}

type server struct {
	db  *sql.DB
	now func() time.Time
	loc *time.Location
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

const itemCols = `id, code, priority, severity, kind, area, title, evidence, fix, impact, status, notes, ord, source, created_at, updated_at`

func scanItem(sc interface{ Scan(...any) error }) (Item, error) {
	var i Item
	err := sc.Scan(&i.ID, &i.Code, &i.Priority, &i.Severity, &i.Kind, &i.Area, &i.Title, &i.Evidence, &i.Fix, &i.Impact, &i.Status, &i.Notes, &i.Order, &i.Source, &i.CreatedAt, &i.UpdatedAt)
	return i, err
}

func (s *server) listItems(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT `+itemCols+` FROM items ORDER BY ord, id`)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		i, err := scanItem(rows)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		out = append(out, i)
	}
	writeJSON(w, 200, out)
}

func (s *server) createItem(w http.ResponseWriter, r *http.Request) {
	var in Item
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		fail(w, 400, "invalid JSON")
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	switch {
	case in.Title == "":
		fail(w, 422, "Введите название.")
		return
	case !priorities[in.Priority], !severities[in.Severity], !kinds[in.Kind]:
		fail(w, 422, "Неизвестный приоритет, серьёзность или тип.")
		return
	}
	var maxOrd int
	_ = s.db.QueryRow(`SELECT coalesce(max(ord), 0) FROM items`).Scan(&maxOrd)
	ts := now()
	// A code in the audit's own scheme (SEC-6, DATA-7…) when the caller
	// gives one; otherwise NEW-n.
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	if in.Code != "" {
		if !codePattern.MatchString(in.Code) {
			fail(w, 422, "Код в формате ABC-12.")
			return
		}
		var taken int
		_ = s.db.QueryRow(`SELECT count(*) FROM items WHERE id = ? OR code = ?`, in.Code, in.Code).Scan(&taken)
		if taken > 0 {
			fail(w, 422, "Код "+in.Code+" уже занят.")
			return
		}
		in.ID = in.Code
	} else {
		var n int
		_ = s.db.QueryRow(`SELECT count(*) FROM items WHERE code LIKE 'NEW-%'`).Scan(&n)
		in.ID = fmt.Sprintf("NEW-%d-%d", n+1, time.Now().UnixMilli())
		in.Code = fmt.Sprintf("NEW-%d", n+1)
	}
	in.Status, in.Order, in.Source, in.CreatedAt, in.UpdatedAt = "todo", maxOrd+1, "manual", ts, ts
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO items (`+itemCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		in.ID, in.Code, in.Priority, in.Severity, in.Kind, strings.TrimSpace(in.Area), in.Title, in.Evidence, strings.TrimSpace(in.Fix), in.Impact, in.Status, in.Notes, in.Order, in.Source, in.CreatedAt, in.UpdatedAt)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, in)
}

// patchItem changes the editable fields given; every change is logged in
// item_changes, which is the "when was this done" history.
func (s *server) patchItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in map[string]string
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		fail(w, 400, "invalid JSON")
		return
	}
	allowed := map[string]map[string]bool{"status": statuses, "priority": priorities, "severity": severities, "notes": nil, "title": nil, "fix": nil, "area": nil}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	cur, err := scanItem(tx.QueryRow(`SELECT `+itemCols+` FROM items WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "Задача не найдена.")
		return
	} else if err != nil {
		fail(w, 500, err.Error())
		return
	}
	old := map[string]string{"status": cur.Status, "priority": cur.Priority, "severity": cur.Severity, "notes": cur.Notes, "title": cur.Title, "fix": cur.Fix, "area": cur.Area}
	ts := now()
	for field, val := range in {
		set, ok := allowed[field]
		if !ok {
			fail(w, 422, "Поле нельзя менять: "+field)
			return
		}
		if set != nil && !set[val] {
			fail(w, 422, "Недопустимое значение для "+field)
			return
		}
		if field == "title" && strings.TrimSpace(val) == "" {
			fail(w, 422, "Название не может быть пустым.")
			return
		}
		if old[field] == val {
			continue
		}
		// field is one of the fixed keys above, never user text.
		if _, err := tx.Exec(`UPDATE items SET `+field+` = ?, updated_at = ? WHERE id = ?`, val, ts, id); err != nil {
			fail(w, 500, err.Error())
			return
		}
		if field != "notes" {
			if _, err := tx.Exec(`INSERT INTO item_changes (item_id, field, old, new, at) VALUES (?,?,?,?,?)`, id, field, old[field], val, ts); err != nil {
				fail(w, 500, err.Error())
				return
			}
		}
	}
	// Read back through tx, then commit even when nothing changed: the pool
	// has one connection (openDB), so a read through s.db while tx still
	// holds it waits forever — and with it every later request.
	i, err := scanItem(tx.QueryRow(`SELECT `+itemCols+` FROM items WHERE id = ?`, id))
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, i)
}

func (s *server) deleteItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, err := s.db.ExecContext(r.Context(), `DELETE FROM items WHERE id = ?`, id)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		fail(w, 404, "Задача не найдена.")
		return
	}
	_, _ = s.db.ExecContext(r.Context(), `DELETE FROM item_changes WHERE item_id = ?`, id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) itemChanges(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT item_id, field, old, new, at FROM item_changes WHERE item_id = ? ORDER BY id DESC LIMIT 50`, r.PathValue("id"))
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []Change{}
	for rows.Next() {
		var c Change
		if err := rows.Scan(&c.ItemID, &c.Field, &c.Old, &c.New, &c.At); err != nil {
			fail(w, 500, err.Error())
			return
		}
		out = append(out, c)
	}
	writeJSON(w, 200, out)
}

func (s *server) listFeatures(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, name, status, evidence, limits, ord, updated_at FROM features ORDER BY ord, id`)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []Feature{}
	for rows.Next() {
		var f Feature
		if err := rows.Scan(&f.ID, &f.Name, &f.Status, &f.Evidence, &f.Limits, &f.Order, &f.UpdatedAt); err != nil {
			fail(w, 500, err.Error())
			return
		}
		out = append(out, f)
	}
	writeJSON(w, 200, out)
}

func (s *server) patchFeature(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil || !featStates[in.Status] {
		fail(w, 422, "Недопустимый статус.")
		return
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE features SET status = ?, updated_at = ? WHERE id = ?`, in.Status, now(), r.PathValue("id"))
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		fail(w, 404, "Строка не найдена.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": in.Status})
}

// stats answers GET /api/stats?days=N (7–180, default 30): the dashboard,
// computed from the items and their change log.
func (s *server) stats(w http.ResponseWriter, r *http.Request) {
	days := 30
	if v := r.URL.Query().Get("days"); v != "" {
		var n int
		if _, err := fmt.Sscan(v, &n); err != nil || n < 7 || n > 180 {
			fail(w, 422, "days: от 7 до 180.")
			return
		}
		days = n
	}
	ctx := r.Context()
	rows, err := s.db.QueryContext(ctx, `SELECT `+itemCols+` FROM items ORDER BY ord, id`)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	var items []Item
	titles := map[string][2]string{}
	for rows.Next() {
		i, err := scanItem(rows)
		if err != nil {
			rows.Close()
			fail(w, 500, err.Error())
			return
		}
		items = append(items, i)
		titles[i.ID] = [2]string{i.Code, i.Title}
	}
	rows.Close()

	crows, err := s.db.QueryContext(ctx, `SELECT item_id, field, old, new, at FROM item_changes ORDER BY id`)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	var changes []Change
	for crows.Next() {
		var c Change
		if err := crows.Scan(&c.ItemID, &c.Field, &c.Old, &c.New, &c.At); err != nil {
			crows.Close()
			fail(w, 500, err.Error())
			return
		}
		changes = append(changes, c)
	}
	crows.Close()

	st := computeStats(items, changes, s.now(), days, s.loc)
	// The feed: the latest changes of items that still exist, newest first.
	for i := len(changes) - 1; i >= 0 && len(st.Recent) < 12; i-- {
		c := changes[i]
		t, ok := titles[c.ItemID]
		if !ok {
			continue
		}
		st.Recent = append(st.Recent, Activity{ItemID: c.ItemID, Code: t[0], Title: t[1], Field: c.Field, Old: c.Old, New: c.New, At: c.At})
	}
	if st.Recent == nil {
		st.Recent = []Activity{}
	}
	writeJSON(w, 200, st)
}
