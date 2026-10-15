package pixelsend_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
	"github.com/ismagilovnail/flox/apps/internal/pixelsend"
	"github.com/ismagilovnail/flox/apps/internal/testpg"
)

// recordingSender remembers what it was asked to send.
type recordingSender struct {
	mu    sync.Mutex
	sent  []pixel.Event
	creds []pixel.Credentials
	out   pixelsend.Outcome
}

func (s *recordingSender) Send(_ context.Context, c pixel.Credentials, ev pixel.Event) pixelsend.Outcome {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, ev)
	s.creds = append(s.creds, c)
	return s.out
}

func fieldError(err error, field string) string {
	var apiErr *apierror.Error
	if errors.As(err, &apiErr) {
		return apiErr.Fields[field]
	}
	return ""
}

func status(err error) int {
	var apiErr *apierror.Error
	if errors.As(err, &apiErr) {
		return apiErr.Status
	}
	return 0
}

// A test event goes through the pixel's sender with a fresh test click id,
// the operator's IP and user agent, a 1 USD value for a conversion; it is
// in the pixel's delivery log, finished, and another organization cannot
// send one.
func TestSendTestEvent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	fb := &recordingSender{}
	tester := pixelsend.NewTester(f.repo, map[pixel.Provider]pixelsend.Sender{pixel.ProviderFacebook: fb}, pixelsend.NewPostgresStore(f.pool))
	p := f.pixel(t, pixel.CreateInput{Name: "FB", Provider: pixel.ProviderFacebook, PixelID: "123", AccessToken: "tok-secret",
		TestEventCode: "TEST123", Events: []string{"CPA_ACCEPT", "PWA_INSTALL"}}, false)

	res, err := tester.Send(ctx, f.org, p.ID, pixelsend.TestInput{IP: "203.0.113.9", UserAgent: "Mozilla/5.0 test"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !res.Delivered || !res.TestMode || res.EventType != "CPA_ACCEPT" || !strings.HasPrefix(res.ClickID, pixelsend.TestEventPrefix) {
		t.Fatalf("result = %+v, want delivered in test mode, the first event, a test click id", res)
	}
	ev, creds := fb.sent[0], fb.creds[0]
	if ev.Type != event.CpaAccept || ev.ClickID != res.ClickID || ev.SourceEventID != res.ClickID || ev.IP != "203.0.113.9" ||
		ev.UserAgent != "Mozilla/5.0 test" || ev.USDValue == nil || *ev.USDValue != 1 || ev.Currency != "USD" {
		t.Fatalf("event = %+v", ev)
	}
	if creds.AccessToken != "tok-secret" || creds.TestEventCode != "TEST123" {
		t.Fatalf("credentials = token %q, test code %q", creds.AccessToken, creds.TestEventCode)
	}

	// A second one gets its own ids (the platform would deduplicate the same).
	res2, err := tester.Send(ctx, f.org, p.ID, pixelsend.TestInput{EventType: "PWA_INSTALL"})
	if err != nil || res2.ClickID == res.ClickID || fb.sent[1].USDValue != nil {
		t.Fatalf("second = %+v, %v; want new ids and no value for an install", res2, err)
	}

	log, err := f.repo.Deliveries(ctx, f.org, p.ID, 10)
	if err != nil || len(log) != 2 {
		t.Fatalf("delivery log = %d rows, %v; want 2", len(log), err)
	}
	for _, d := range log {
		if d.Status != "success" || d.AttemptCount != 1 || !strings.HasPrefix(d.ClickID, pixelsend.TestEventPrefix) || !strings.HasPrefix(d.Message, "test event") {
			t.Fatalf("log row = %+v, want a finished test row", d)
		}
	}
	var payload string
	if err := f.pool.QueryRow(ctx, `SELECT payload::text FROM pixel_deliveries WHERE id = $1`, res.DeliveryID).Scan(&payload); err != nil ||
		strings.Contains(payload, "203.0.113.9") || strings.Contains(payload, "Mozilla") {
		t.Fatalf("payload = %s, %v; want no IP or user agent kept", payload, err)
	}

	if _, err := tester.Send(ctx, idgen.New(), p.ID, pixelsend.TestInput{}); status(err) != http.StatusNotFound {
		t.Fatalf("another organization = %v, want 404", err)
	}
	if _, err := tester.Send(ctx, f.org, p.ID, pixelsend.TestInput{EventType: "CPA_HOLD"}); fieldError(err, "eventType") == "" {
		t.Fatalf("an event the pixel does not report = %v, want an eventType error", err)
	}
}

// Without a test event code (and always for Snapchat and generic) the
// platform counts the event: nothing is sent until the operator confirms.
// A platform's refusal is a result, logged as dead.
func TestSendTestEventNeedsConfirmationWhenReal(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	snap := &recordingSender{out: pixelsend.Outcome{StatusCode: 400, Err: errors.New("Invalid pixel")}}
	fb := &recordingSender{}
	tester := pixelsend.NewTester(f.repo, map[pixel.Provider]pixelsend.Sender{pixel.ProviderSnapchat: snap, pixel.ProviderFacebook: fb}, pixelsend.NewPostgresStore(f.pool))

	for _, in := range []pixel.CreateInput{
		{Name: "Snap", Provider: pixel.ProviderSnapchat, PixelID: "s1", AccessToken: "t", Events: []string{"PWA_INSTALL"}},
		{Name: "FB no code", Provider: pixel.ProviderFacebook, PixelID: "f1", AccessToken: "t", Events: []string{"PWA_INSTALL"}},
	} {
		p := f.pixel(t, in, false)
		before := len(snap.sent) + len(fb.sent)
		if _, err := tester.Send(ctx, f.org, p.ID, pixelsend.TestInput{}); fieldError(err, "confirmReal") == "" {
			t.Fatalf("%s without confirmation = %v, want a confirmReal error", in.Name, err)
		}
		if len(snap.sent)+len(fb.sent) != before {
			t.Fatalf("%s: sent before confirmation", in.Name)
		}
		if in.Provider != pixel.ProviderSnapchat {
			continue
		}
		res, err := tester.Send(ctx, f.org, p.ID, pixelsend.TestInput{ConfirmReal: true})
		if err != nil || res.Delivered || res.TestMode || res.StatusCode != 400 || res.Message != "Invalid pixel" {
			t.Fatalf("confirmed = %+v, %v; want the platform's refusal as a result", res, err)
		}
		log, _ := f.repo.Deliveries(ctx, f.org, p.ID, 10)
		if len(log) != 1 || log[0].Status != "dead" || log[0].ResponseStatusCode == nil || *log[0].ResponseStatusCode != 400 {
			t.Fatalf("log = %+v, want one dead row with 400", log)
		}
	}
}

// The worker never takes a test row: it is written finished.
func TestRecordedTestIsNotClaimed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	table := testpg.PrivateTable(t, f.pool, "pixel_deliveries")
	store := pixelsend.NewPostgresStoreOnTable(f.pool, table)
	p := f.pixel(t, pixel.CreateInput{Name: "FB", Provider: pixel.ProviderFacebook, PixelID: "1", Events: []string{"PWA_INSTALL"}}, false)
	ev := f.event(event.PwaInstall)
	ev.SourceEventID = pixelsend.TestEventPrefix + idgen.New()
	if _, err := store.RecordTest(ctx, f.org, ev, p.ID, pixelsend.Outcome{StatusCode: 200}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimDue(ctx, 10)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("ClaimDue = %d, %v; want nothing", len(claimed), err)
	}
}

// Through the real Facebook sender: the test event code reaches the
// platform with the event, and the token stays out of the result.
func TestSendTestEventReachesFacebookTestTool(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var body map[string]any
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.Write([]byte(`{"events_received":1}`))
	}))
	defer srv.Close()
	senders := map[pixel.Provider]pixelsend.Sender{pixel.ProviderFacebook: &pixelsend.Facebook{Client: srv.Client(), BaseURL: srv.URL}}
	tester := pixelsend.NewTester(f.repo, senders, pixelsend.NewPostgresStore(f.pool))
	p := f.pixel(t, pixel.CreateInput{Name: "FB", Provider: pixel.ProviderFacebook, PixelID: "999", AccessToken: "tok-secret",
		TestEventCode: "TEST42", Events: []string{"CPA_ACCEPT"}}, false)

	res, err := tester.Send(ctx, f.org, p.ID, pixelsend.TestInput{IP: "203.0.113.9", UserAgent: "UA"})
	if err != nil || !res.Delivered || res.StatusCode != 200 {
		t.Fatalf("Send = %+v, %v", res, err)
	}
	if body["test_event_code"] != "TEST42" || !strings.Contains(query, "access_token=tok-secret") {
		t.Fatalf("request = %v ?%s, want the test code and the token", body, query)
	}
	out, _ := json.Marshal(res)
	if strings.Contains(string(out), "tok-secret") {
		t.Fatalf("result %s carries the token", out)
	}
}
