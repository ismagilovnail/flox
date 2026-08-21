package postback_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/postback"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeStore is an in-memory Store, exercising Deliverer's decision logic
// (success / retry / dead-letter) without a database.
type fakeStore struct {
	mu         sync.Mutex
	deliveries map[string]*postback.Delivery
	claimed    []string // ordered ids, for asserting single-claim behavior
	marks      []string // "success"/"retrying"/"dead" per call, for counting
}

func newFakeStore(deliveries ...postback.Delivery) *fakeStore {
	s := &fakeStore{deliveries: map[string]*postback.Delivery{}}
	for i := range deliveries {
		d := deliveries[i]
		s.deliveries[d.ID] = &d
	}
	return s
}

func (s *fakeStore) Enqueue(_ context.Context, in postback.EnqueueInput) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := "d-" + in.ClickID
	s.deliveries[id] = &postback.Delivery{
		ID: id, OrganizationID: in.OrganizationID, NetworkID: in.NetworkID,
		SourcePostbackID: in.SourcePostbackID, ClickID: in.ClickID, Status: in.Status, URL: in.URL,
	}
	return id, nil
}

func (s *fakeStore) ClaimDue(_ context.Context, limit int) ([]postback.Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []postback.Delivery
	for _, d := range s.deliveries {
		if len(out) >= limit {
			break
		}
		d.AttemptCount++
		out = append(out, *d)
		s.claimed = append(s.claimed, d.ID)
	}
	return out, nil
}

func (s *fakeStore) MarkSuccess(_ context.Context, id string, code int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.marks = append(s.marks, "success")
	delete(s.deliveries, id)
	return nil
}

func (s *fakeStore) MarkRetrying(_ context.Context, id string, code int, message string, next time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.marks = append(s.marks, "retrying")
	delete(s.deliveries, id) // simulate "no longer due" for this test's single-pass assertions
	return nil
}

func (s *fakeStore) MarkDead(_ context.Context, id string, code int, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.marks = append(s.marks, "dead")
	delete(s.deliveries, id)
	return nil
}

func (s *fakeStore) markCount(kind string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, m := range s.marks {
		if m == kind {
			n++
		}
	}
	return n
}

type fakeHTTPClient struct {
	respond func(req *http.Request) (*http.Response, error)
}

func (c *fakeHTTPClient) Do(req *http.Request) (*http.Response, error) { return c.respond(req) }

type fakeAttemptLogger struct {
	mu   sync.Mutex
	recs []postback.AttemptRecord
}

func (l *fakeAttemptLogger) LogAttempt(_ context.Context, rec postback.AttemptRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.recs = append(l.recs, rec)
}

func statusResponse(code int) (*http.Response, error) {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func TestDeliverer2xxMarksSuccess(t *testing.T) {
	store := newFakeStore(postback.Delivery{ID: "d1", URL: "https://net.example/pb", AttemptCount: 0})
	client := &fakeHTTPClient{respond: func(*http.Request) (*http.Response, error) { return statusResponse(200) }}
	d := postback.NewDeliverer(store, client, &fakeAttemptLogger{}, quietLogger())

	n, err := d.RunOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 1 {
		t.Fatalf("claimed = %d, want 1", n)
	}
	if store.markCount("success") != 1 {
		t.Fatalf("success marks = %d, want 1", store.markCount("success"))
	}
}

func TestDelivererNon2xxRetriesUnderMaxAttempts(t *testing.T) {
	store := newFakeStore(postback.Delivery{ID: "d1", URL: "https://net.example/pb", AttemptCount: 0}) // ClaimDue bumps to 1
	client := &fakeHTTPClient{respond: func(*http.Request) (*http.Response, error) { return statusResponse(500) }}
	d := postback.NewDeliverer(store, client, &fakeAttemptLogger{}, quietLogger())

	if _, err := d.RunOnce(context.Background(), 10); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if store.markCount("retrying") != 1 {
		t.Fatalf("retrying marks = %d, want 1 (attempt 1 of %d)", store.markCount("retrying"), postback.MaxAttempts)
	}
	if store.markCount("dead") != 0 {
		t.Fatal("must not dead-letter before MaxAttempts")
	}
}

func TestDelivererDeadLettersAtMaxAttempts(t *testing.T) {
	store := newFakeStore(postback.Delivery{ID: "d1", URL: "https://net.example/pb", AttemptCount: postback.MaxAttempts - 1})
	client := &fakeHTTPClient{respond: func(*http.Request) (*http.Response, error) { return statusResponse(500) }}
	d := postback.NewDeliverer(store, client, &fakeAttemptLogger{}, quietLogger())

	if _, err := d.RunOnce(context.Background(), 10); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if store.markCount("dead") != 1 {
		t.Fatalf("dead marks = %d, want 1 at MaxAttempts (%d)", store.markCount("dead"), postback.MaxAttempts)
	}
}

func TestDelivererNetworkErrorRetriesLikeNon2xx(t *testing.T) {
	store := newFakeStore(postback.Delivery{ID: "d1", URL: "https://net.example/pb", AttemptCount: 0})
	client := &fakeHTTPClient{respond: func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	}}
	d := postback.NewDeliverer(store, client, &fakeAttemptLogger{}, quietLogger())

	if _, err := d.RunOnce(context.Background(), 10); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if store.markCount("retrying") != 1 {
		t.Fatal("a network-level error must retry, not dead-letter immediately, before MaxAttempts")
	}
}

func TestDelivererMalformedURLRetries(t *testing.T) {
	store := newFakeStore(postback.Delivery{ID: "d1", URL: "://not a url", AttemptCount: 0})
	client := &fakeHTTPClient{respond: func(*http.Request) (*http.Response, error) {
		t.Fatal("client.Do must not be called for an unparseable URL")
		return nil, nil
	}}
	d := postback.NewDeliverer(store, client, &fakeAttemptLogger{}, quietLogger())

	if _, err := d.RunOnce(context.Background(), 10); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if store.markCount("retrying") != 1 {
		t.Fatal("a malformed URL is still logged and retried (or eventually dead-lettered), never silently dropped")
	}
}

func TestNextAttemptDelayIsMonotonicAndCapped(t *testing.T) {
	var last time.Duration
	for attempt := 1; attempt <= postback.MaxAttempts+5; attempt++ {
		got := postback.NextAttemptDelay(attempt)
		if got < last {
			t.Fatalf("NextAttemptDelay(%d) = %v, less than NextAttemptDelay(%d) = %v — must not decrease", attempt, got, attempt-1, last)
		}
		last = got
	}
	// Past the table's end, every further attempt waits the same capped delay.
	tail := postback.NextAttemptDelay(len(postback.Backoff))
	beyond := postback.NextAttemptDelay(len(postback.Backoff) + 10)
	if tail != beyond {
		t.Fatalf("delay past the backoff table's end should stay capped: got %v vs %v", tail, beyond)
	}
}

// TestAttemptLoggedForEveryOutcome is §48's postback_events requirement at
// the Deliverer level: success, retrying, and dead must all report to the
// attempt audit log. One independent Deliverer per outcome, so each
// assertion is unambiguous about which attempt produced which log entry.
func TestAttemptLoggedForEveryOutcome(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := newFakeStore(postback.Delivery{ID: "s1", URL: "https://net.example/pb"})
		client := &fakeHTTPClient{respond: func(*http.Request) (*http.Response, error) { return statusResponse(200) }}
		logger := &fakeAttemptLogger{}
		d := postback.NewDeliverer(store, client, logger, quietLogger())

		if _, err := d.RunOnce(context.Background(), 10); err != nil {
			t.Fatalf("RunOnce: %v", err)
		}
		if len(logger.recs) != 1 || logger.recs[0].Result != postback.StatusSuccess {
			t.Fatalf("logged recs = %+v, want one StatusSuccess", logger.recs)
		}
	})

	t.Run("retrying", func(t *testing.T) {
		store := newFakeStore(postback.Delivery{ID: "r1", URL: "https://net.example/pb"})
		client := &fakeHTTPClient{respond: func(*http.Request) (*http.Response, error) { return statusResponse(500) }}
		logger := &fakeAttemptLogger{}
		d := postback.NewDeliverer(store, client, logger, quietLogger())

		if _, err := d.RunOnce(context.Background(), 10); err != nil {
			t.Fatalf("RunOnce: %v", err)
		}
		if len(logger.recs) != 1 || logger.recs[0].Result != postback.StatusRetrying {
			t.Fatalf("logged recs = %+v, want one StatusRetrying", logger.recs)
		}
	})

	t.Run("dead", func(t *testing.T) {
		store := newFakeStore(postback.Delivery{ID: "d1", URL: "https://net.example/pb", AttemptCount: postback.MaxAttempts - 1})
		client := &fakeHTTPClient{respond: func(*http.Request) (*http.Response, error) { return statusResponse(500) }}
		logger := &fakeAttemptLogger{}
		d := postback.NewDeliverer(store, client, logger, quietLogger())

		if _, err := d.RunOnce(context.Background(), 10); err != nil {
			t.Fatalf("RunOnce: %v", err)
		}
		if len(logger.recs) != 1 || logger.recs[0].Result != postback.StatusDead {
			t.Fatalf("logged recs = %+v, want one StatusDead", logger.recs)
		}
	})
}

func TestClaimedDeliveryCarriesStatus(t *testing.T) {
	store := newFakeStore(postback.Delivery{ID: "d1", URL: "https://net.example/pb", Status: event.CpaAccept})
	claimed, err := store.ClaimDue(context.Background(), 10)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Status != event.CpaAccept {
		t.Fatalf("claimed = %+v, want one CPA_ACCEPT delivery", claimed)
	}
}
