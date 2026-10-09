package chstore_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
)

// The worker sets the postback log's TTL at every start: only a change
// alters the table. Ends on 13 months, the default the dev worker applies.
func TestSetPostbackLogTTL(t *testing.T) {
	conn := mustConn(t)
	ctx := context.Background()
	t.Cleanup(func() { _, _ = chstore.SetPostbackLogTTL(context.Background(), conn, 13) })
	engine := func() string {
		t.Helper()
		var e string
		if err := conn.QueryRow(ctx, `SELECT engine_full FROM system.tables WHERE database = currentDatabase() AND name = 'postback_events'`).Scan(&e); err != nil {
			t.Fatal(err)
		}
		return e
	}

	for _, step := range []struct {
		months      int
		wantChanged bool
		wantTTL     string // "" = none
	}{
		{0, true, ""},
		{0, false, ""},
		{13, true, "toIntervalMonth(13)"},
		{13, false, "toIntervalMonth(13)"},
		{1, true, "toIntervalMonth(1)"}, // a different period is a change, not a match on "(1"
		{13, true, "toIntervalMonth(13)"},
	} {
		changed, err := chstore.SetPostbackLogTTL(ctx, conn, step.months)
		if err != nil {
			t.Fatalf("months %d: %v", step.months, err)
		}
		if changed != step.wantChanged {
			t.Errorf("months %d: changed = %v, want %v", step.months, changed, step.wantChanged)
		}
		e := engine()
		if step.wantTTL == "" && strings.Contains(e, " TTL ") || step.wantTTL != "" && !strings.Contains(e, step.wantTTL) {
			t.Errorf("months %d: engine %q", step.months, e)
		}
	}
}
