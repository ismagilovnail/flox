package apierror_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"fmt"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chconn"
)

// A report ClickHouse stopped at the API's limits is the request's size,
// not a server fault: 422 with a code the UI translates (tech audit CH4).
func TestWriteQueryTooHeavy(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for err, want := range map[error]struct {
		status int
		code   string
	}{
		errors.New("chstore: report: ClickHouse exception: Code: 159. DB::Exception: Timeout exceeded. (TIMEOUT_EXCEEDED)"): {http.StatusUnprocessableEntity, "query_too_heavy"},
		errors.New("chstore: report: ClickHouse exception: Code: 241. (MEMORY_LIMIT_EXCEEDED)"):                             {http.StatusUnprocessableEntity, "query_too_heavy"},
		errors.New("pgx: connection refused"): {http.StatusInternalServerError, "internal"},
	} {
		w := httptest.NewRecorder()
		apierror.Write(w, logger, err)
		var body struct{ Code string }
		_ = json.NewDecoder(w.Body).Decode(&body)
		if w.Code != want.status || body.Code != want.code {
			t.Errorf("%v: %d %s, want %d %s", err, w.Code, body.Code, want.status, want.code)
		}
	}
}

// An organization over its plan's concurrent reports (audit CH-1): 429
// with a code the UI translates and a Retry-After, even wrapped.
func TestWriteReportsBusy(t *testing.T) {
	w := httptest.NewRecorder()
	apierror.Write(w, slog.New(slog.NewTextHandler(io.Discard, nil)), fmt.Errorf("chstore: report: %w", chconn.ErrBusy))
	var body struct{ Code string }
	_ = json.NewDecoder(w.Body).Decode(&body)
	if w.Code != http.StatusTooManyRequests || body.Code != "reports_busy" || w.Header().Get("Retry-After") == "" {
		t.Errorf("busy = %d %s Retry-After %q, want 429 reports_busy with Retry-After", w.Code, body.Code, w.Header().Get("Retry-After"))
	}
}
