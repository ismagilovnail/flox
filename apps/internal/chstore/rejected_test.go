package chstore_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
)

func TestRejected(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{errors.New(`[HTTP 500] response body: "Code: 16. DB::Exception: No such column x in table flox.click_events. (NO_SUCH_COLUMN_IN_TABLE)"`), true},
		{errors.New(`[HTTP 500] response body: "Code: 53. DB::Exception: Type mismatch. (TYPE_MISMATCH)"`), true},
		{fmt.Errorf("chstore: sending: %w", errors.New(`Code: 27. DB::Exception: Cannot parse input`)), true},
		{errors.New(`[HTTP 500] response body: "Code: 241. DB::Exception: Memory limit exceeded. (MEMORY_LIMIT_EXCEEDED)"`), false},
		{errors.New(`[HTTP 500] response body: "Code: 252. DB::Exception: Too many parts. (TOO_MANY_PARTS)"`), false},
		{errors.New(`dial tcp 10.99.0.4:8123: connect: connection refused`), false},
		{context.DeadlineExceeded, false},
		{nil, false},
	} {
		if got := chstore.Rejected(tc.err); got != tc.want {
			t.Errorf("Rejected(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

// The classifier must match what the real client returns for a server-side
// rejection, not just strings written in a test.
func TestRejectedMatchesARealServerRejection(t *testing.T) {
	conn := mustConn(t)
	err := conn.Exec(context.Background(), `INSERT INTO click_events (no_such_column) VALUES (1)`)
	if err == nil || !chstore.Rejected(err) {
		t.Fatalf("Rejected(%v) = false, want true for a real NO_SUCH_COLUMN rejection", err)
	}
}
