package googleads_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/adaccount/googleads"
)

func fakeOAuth(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("refresh_token") != "test-refresh-token" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "invalid_grant", "error_description": "Bad Request"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token": "test-access-token"}`))
	}))
	t.Cleanup(s.Close)
	return s
}

// Audit UNI-2 part 2: the documented events:ingest request — the
// operating and login accounts without dashes, the conversion action as
// the product destination, one event per conversion with its click id,
// RFC 3339 time, transaction id, value and currency.
func TestIngestSendsTheDocumentedRequest(t *testing.T) {
	var got map[string]any
	dm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/events:ingest" {
			t.Errorf("%s %s, want POST /events:ingest", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-access-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"requestId": "req-1", "fieldWarnings": [{"field": "events[0].consent", "description": "not set"}]}`))
	}))
	defer dm.Close()
	d := &googleads.DataManager{OAuthBaseURL: fakeOAuth(t).URL, BaseURL: dm.URL, HTTPClient: dm.Client()}

	value := 49.9
	at := time.Date(2026, 10, 24, 12, 30, 5, 0, time.FixedZone("MSK", 3*3600))
	res, err := d.Ingest(context.Background(), testCreds(), googleads.Destination{AccountID: "123-456-7890", LoginAccountID: "111-222-3333", ActionID: "987654"},
		[]googleads.Event{
			{GCLID: "Cj0K", At: at, TransactionID: "01POSTBACK", Value: &value, Currency: "RUB"},
			{WBRAID: "W1", At: at, TransactionID: "01LEAD"},
		}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.RequestID != "req-1" || len(res.Warnings) != 1 || res.Warnings[0] != "events[0].consent: not set" {
		t.Fatalf("result = %+v", res)
	}
	raw, _ := json.Marshal(got)
	want := `{"destinations":[{"loginAccount":{"accountId":"1112223333","accountType":"GOOGLE_ADS"},` +
		`"operatingAccount":{"accountId":"1234567890","accountType":"GOOGLE_ADS"},"productDestinationId":"987654"}],` +
		`"events":[{"adIdentifiers":{"gclid":"Cj0K"},"conversionValue":49.9,"currency":"RUB","eventSource":"WEB","eventTimestamp":"2026-10-24T09:30:05Z","transactionId":"01POSTBACK"},` +
		`{"adIdentifiers":{"wbraid":"W1"},"eventSource":"WEB","eventTimestamp":"2026-10-24T09:30:05Z","transactionId":"01LEAD"}]}`
	if string(raw) != want {
		t.Fatalf("request body =\n  %s\nwant\n  %s", raw, want)
	}
}

// A 400 refuses the whole request and cannot succeed again; a rate limit,
// a server error or a refused refresh token can.
func TestIngestErrors(t *testing.T) {
	for _, c := range []struct {
		name      string
		status    int
		body      string
		refresh   string
		permanent bool
	}{
		{"bad event", 400, `{"error": {"code": 400, "status": "INVALID_ARGUMENT", "message": "events[0].adIdentifiers: NO_IDENTIFIERS_PROVIDED"}}`, "test-refresh-token", true},
		{"rate limited", 429, `{"error": {"code": 429, "status": "RESOURCE_EXHAUSTED", "message": "quota"}}`, "test-refresh-token", false},
		{"no scope", 403, `{"error": {"code": 403, "status": "PERMISSION_DENIED", "message": "insufficient scopes"}}`, "test-refresh-token", false},
		{"server", 503, `upstream down`, "test-refresh-token", false},
		{"refresh refused", 200, `{}`, "revoked", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer dm.Close()
			d := &googleads.DataManager{OAuthBaseURL: fakeOAuth(t).URL, BaseURL: dm.URL, HTTPClient: dm.Client()}
			creds := testCreds()
			creds.RefreshToken = c.refresh
			_, err := d.Ingest(context.Background(), creds, googleads.Destination{AccountID: "1234567890", ActionID: "1"},
				[]googleads.Event{{GCLID: "x", At: time.Now(), TransactionID: "t"}}, false)
			if err == nil || googleads.IsPermanent(err) != c.permanent {
				t.Fatalf("err = %v, permanent %v; want an error, permanent %v", err, googleads.IsPermanent(err), c.permanent)
			}
		})
	}
}
