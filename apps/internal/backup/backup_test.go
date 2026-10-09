package backup

import (
	"strings"
	"testing"
	"time"
)

func TestNextRun(t *testing.T) {
	at := 3 * time.Hour
	for _, c := range []struct{ now, want string }{
		{"2026-10-09T02:59:59Z", "2026-10-09T03:00:00Z"},
		{"2026-10-09T03:00:00Z", "2026-10-10T03:00:00Z"}, // strictly after: a run that just fired does not fire again
		{"2026-10-09T23:30:00Z", "2026-10-10T03:00:00Z"},
		{"2026-12-31T05:00:00Z", "2027-01-01T03:00:00Z"},
		{"2026-10-09T05:00:00+05:00", "2026-10-09T03:00:00Z"}, // 00:00 UTC: later the same UTC day
	} {
		now, _ := time.Parse(time.RFC3339, c.now)
		if got := NextRun(now, at).Format(time.RFC3339); got != c.want {
			t.Errorf("NextRun(%s) = %s, want %s", c.now, got, c.want)
		}
	}
}

func TestConfigFromEnv(t *testing.T) {
	base := map[string]string{
		"BACKUP_S3_ENDPOINT": "http://backup:9000", "BACKUP_S3_BUCKET": "flox-backups",
		"BACKUP_S3_ACCESS_KEY_ID": "k", "BACKUP_S3_SECRET_ACCESS_KEY": "s",
		"DATABASE_URL": "postgres://flox:flox@pg:5432/flox", "CLICKHOUSE_URL": "http://ch:8123",
		"S3_ENDPOINT": "http://minio:9000", "S3_BUCKET": "flox",
	}
	set := func(t *testing.T, over map[string]string) {
		for k, v := range base {
			t.Setenv(k, v)
		}
		for k, v := range over {
			t.Setenv(k, v)
		}
	}

	t.Run("defaults", func(t *testing.T) {
		set(t, nil)
		c, err := ConfigFromEnv()
		if err != nil {
			t.Fatal(err)
		}
		if c.At != 3*time.Hour || c.KeepPostgres != 14 || c.KeepClickHouse != 14 || c.DrillDay != 1 ||
			c.ClickHouseS3Endpoint != "http://backup:9000" || c.DatabaseURL != base["DATABASE_URL"] {
			t.Errorf("config = %+v", c)
		}
	})
	t.Run("backup database url wins", func(t *testing.T) {
		set(t, map[string]string{"BACKUP_DATABASE_URL": "postgres://backup@pg/flox"})
		if c, err := ConfigFromEnv(); err != nil || c.DatabaseURL != "postgres://backup@pg/flox" {
			t.Errorf("DatabaseURL = %q, %v", c.DatabaseURL, err)
		}
	})
	for _, c := range []struct {
		name string
		over map[string]string
		want string
	}{
		{"missing bucket", map[string]string{"BACKUP_S3_BUCKET": ""}, "BACKUP_S3_BUCKET"},
		{"bad clock", map[string]string{"BACKUP_AT": "3am"}, "BACKUP_AT"},
		{"keep zero", map[string]string{"BACKUP_KEEP_POSTGRES": "0"}, "BACKUP_KEEP_POSTGRES"},
		{"drill day 31", map[string]string{"BACKUP_DRILL_DAY": "31"}, "BACKUP_DRILL_DAY"},
		{"backup into the files bucket", map[string]string{"BACKUP_S3_ENDPOINT": "http://minio:9000", "BACKUP_S3_BUCKET": "flox"}, "must not be S3_BUCKET"},
	} {
		t.Run(c.name, func(t *testing.T) {
			set(t, c.over)
			if _, err := ConfigFromEnv(); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want one naming %s", err, c.want)
			}
		})
	}
}

func TestRowsBetweenHolds(t *testing.T) {
	r := RowsBetween{Before: 100, After: 110}
	for n, want := range map[uint64]bool{99: false, 100: true, 105: true, 110: true, 111: false} {
		if r.Holds(n) != want {
			t.Errorf("Holds(%d) = %v", n, !want)
		}
	}
	// A delete during the backup (a repair's mutation) counts down.
	if !(RowsBetween{Before: 10, After: 8}).Holds(9) {
		t.Error("a shrinking table must hold counts between after and before")
	}
}

func TestCompareCounts(t *testing.T) {
	checks := compareCounts(
		map[string]RowsBetween{"a": {5, 5}, "b": {1, 3}, "gone": {1, 1}},
		map[string]uint64{"a": 4, "b": 2, "extra": 7},
	)
	got := map[string]bool{}
	for _, c := range checks {
		got[c.Name] = c.OK
	}
	if len(checks) != 3 || got["a"] || !got["b"] || got["gone"] {
		t.Errorf("checks = %+v, want a short (fail), b in range (pass), gone missing (fail), extra ignored", checks)
	}
}

func TestOldest(t *testing.T) {
	names := []string{"1", "2", "3", "4"}
	if got := oldest(names, 2); strings.Join(got, ",") != "1,2" {
		t.Errorf("oldest(keep 2) = %v", got)
	}
	if got := oldest(names, 4); got != nil {
		t.Errorf("oldest(keep all) = %v", got)
	}
}

func TestMissingTables(t *testing.T) {
	restored := map[string]int64{"a": 1, "b": 2}
	if c := missingTables([]string{"a", "b"}, restored); !c.OK {
		t.Errorf("all present: %+v", c)
	}
	if c := missingTables([]string{"a", "b", "c"}, restored); c.OK || !strings.Contains(c.Detail, "c") {
		t.Errorf("c missing: %+v", c)
	}
}

func TestArchiverFailing(t *testing.T) {
	t0 := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		a    Archiver
		want bool
	}{
		{Archiver{}, false},                 // nothing archived yet, nothing failed
		{Archiver{LastArchived: t0}, false}, // idle since: not a failure
		{Archiver{LastArchived: t0, LastFailed: t0.Add(-time.Hour)}, false}, // recovered
		{Archiver{LastArchived: t0, LastFailed: t0.Add(time.Minute)}, true},
		{Archiver{LastFailed: t0}, true},
	} {
		if got := c.a.Failing(); got != c.want {
			t.Errorf("%+v Failing = %v, want %v", c.a, got, c.want)
		}
	}
}

func TestQuote(t *testing.T) {
	if got := quote(`it's \ ok`); got != `'it\'s \\ ok'` {
		t.Errorf("quote = %s", got)
	}
}
