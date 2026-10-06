package errtrack_test

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/errtrack"
)

// fakeTracker is a Sentry-protocol endpoint: it records every envelope the
// SDK posts.
type fakeTracker struct {
	mu        sync.Mutex
	envelopes []string
}

func (f *fakeTracker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.envelopes = append(f.envelopes, r.URL.Path+"\n"+string(body))
	f.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (f *fakeTracker) all() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.envelopes, "\n---\n")
}

func newReporter(t *testing.T) (errtrack.Reporter, *fakeTracker) {
	t.Helper()
	fake := &fakeTracker{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	dsn := strings.Replace(srv.URL, "http://", "http://publickey@", 1) + "/42"
	r, err := errtrack.New(errtrack.Config{DSN: dsn, Environment: "test", Service: "tracker"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r, fake
}

func TestErrorLogRecordsAreReported(t *testing.T) {
	r, fake := newReporter(t)
	logger := slog.New(errtrack.Handler(slog.NewTextHandler(io.Discard, nil), r)).With("campaign_id", "c-1")

	logger.Info("info-record-zq1")
	logger.Warn("warn-record-zq2")
	logger.Error("resolving tracking link", "error", errors.New("connection refused"), "slug", "go")
	r.Flush(5 * time.Second)

	got := fake.all()
	for _, want := range []string{"/api/42/envelope/", "resolving tracking link", "connection refused", `"slug":"go"`, `"campaign_id":"c-1"`, `"service":"tracker"`, `"environment":"test"`} {
		if !strings.Contains(got, want) {
			t.Errorf("tracker did not receive %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, "info-record-zq1") || strings.Contains(got, "warn-record-zq2") {
		t.Errorf("info/warn records were reported:\n%s", got)
	}
}

func TestPanickingHandlerIsReportedAnd500(t *testing.T) {
	r, fake := newReporter(t)
	h := errtrack.Middleware(r, slog.New(slog.NewTextHandler(io.Discard, nil)))(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("nil map") }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t/go", nil))
	r.Flush(5 * time.Second)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	got := fake.all()
	for _, want := range []string{"panic in http handler", "nil map", "/t/go", "errtrack_test.go"} {
		if !strings.Contains(got, want) {
			t.Errorf("tracker did not receive %q (the stack must point at the panic):\n%s", want, got)
		}
	}
}

func TestWithoutDSNNothingChanges(t *testing.T) {
	r, err := errtrack.New(errtrack.Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inner := slog.NewTextHandler(io.Discard, nil)
	if errtrack.Handler(inner, r) != slog.Handler(inner) {
		t.Fatal("without a DSN the log handler must be returned unchanged")
	}
}

func TestGoReportsThenRepanics(t *testing.T) {
	r, fake := newReporter(t)
	defer func() {
		if p := recover(); p != "loop broke" {
			t.Fatalf("recovered %v, want the original panic re-raised", p)
		}
		if !strings.Contains(fake.all(), "panic in event flush") {
			t.Fatalf("panic was not reported before re-raising:\n%s", fake.all())
		}
	}()
	errtrack.Go(r, "event flush", func() { panic("loop broke") })
}
