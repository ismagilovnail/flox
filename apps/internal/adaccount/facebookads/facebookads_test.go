package facebookads_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/facebookads"
)

func TestDailySpendByCampaign(t *testing.T) {
	var gotPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)

		if r.URL.Query().Get("access_token") != "test-token" {
			t.Errorf("access_token = %q, want test-token", r.URL.Query().Get("access_token"))
		}
		if want := "/act_12345/insights"; r.URL.Path != want {
			t.Errorf("path = %q, want %q", r.URL.Path, want)
		}
		if r.URL.Query().Get("level") != "campaign" {
			t.Errorf("level = %q, want campaign", r.URL.Query().Get("level"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [
				{"campaign_id": "c1", "spend": "12.50", "account_currency": "USD", "date_start": "2026-01-01"},
				{"campaign_id": "c2", "spend": "3.75", "account_currency": "USD", "date_start": "2026-01-01"}
			],
			"paging": {}
		}`))
	}))
	defer server.Close()

	adapter := &facebookads.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from

	records, err := adapter.DailySpendByCampaign(context.Background(), adaccount.Credentials{AdAccountID: "12345", AccessToken: "test-token"}, from, to)
	if err != nil {
		t.Fatalf("DailySpendByCampaign: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if records[0].ExternalCampaignID != "c1" || records[0].Amount != 12.50 || records[0].Currency != "USD" {
		t.Fatalf("records[0] = %+v, unexpected", records[0])
	}
	if !records[0].Date.Equal(from) {
		t.Fatalf("records[0].Date = %v, want %v", records[0].Date, from)
	}
	if len(gotPaths) != 1 {
		t.Fatalf("server was hit %d times, want 1 (no pagination expected)", len(gotPaths))
	}
}

func TestDailySpendByCampaignFollowsPagination(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after") == "" {
			nextURL := "http://" + r.Host + r.URL.Path + "?" + r.URL.Query().Encode() + "&after=page2"
			body, _ := json.Marshal(map[string]any{
				"data": []map[string]string{
					{"campaign_id": "c1", "spend": "10", "account_currency": "USD", "date_start": "2026-01-01"},
				},
				"paging": map[string]string{"next": nextURL},
			})
			_, _ = w.Write(body)
			return
		}
		_, _ = w.Write([]byte(`{
			"data": [{"campaign_id": "c2", "spend": "20", "account_currency": "USD", "date_start": "2026-01-02"}],
			"paging": {}
		}`))
	}))
	defer server.Close()

	adapter := &facebookads.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	records, err := adapter.DailySpendByCampaign(context.Background(), adaccount.Credentials{AdAccountID: "1", AccessToken: "t"}, from, to)
	if err != nil {
		t.Fatalf("DailySpendByCampaign: %v", err)
	}
	if calls != 2 {
		t.Fatalf("server hit %d times, want 2 (should follow paging.next)", calls)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2 across both pages", len(records))
	}
}

func TestDailySpendByCampaignPropagatesGraphAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": {"message": "Invalid OAuth access token.", "type": "OAuthException", "code": 190, "fbtrace_id": "abc123"}}`))
	}))
	defer server.Close()

	adapter := &facebookads.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := adapter.DailySpendByCampaign(context.Background(), adaccount.Credentials{AdAccountID: "1", AccessToken: "bad"}, time.Now(), time.Now())
	if err == nil {
		t.Fatal("DailySpendByCampaign returned no error for a Graph API error body, want an error surfacing the OAuth failure")
	}
}

func TestDailySpendByCampaignRejectsMalformedSpend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"campaign_id": "c1", "spend": "not-a-number", "account_currency": "USD", "date_start": "2026-01-01"}], "paging": {}}`))
	}))
	defer server.Close()

	adapter := &facebookads.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := adapter.DailySpendByCampaign(context.Background(), adaccount.Credentials{AdAccountID: "1", AccessToken: "t"}, time.Now(), time.Now())
	if err == nil {
		t.Fatal("DailySpendByCampaign accepted a non-numeric spend value, want an error")
	}
}

// ensure url.PathEscape usage doesn't accidentally mangle a normal
// numeric ad account id — a quick sanity check the built request URL is
// well-formed, not just "the server didn't crash".
func TestDailySpendByCampaignBuildsWellFormedURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := url.Parse(r.URL.String()); err != nil {
			t.Errorf("request URL not well-formed: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [], "paging": {}}`))
	}))
	defer server.Close()

	adapter := facebookads.New()
	adapter.BaseURL = server.URL
	adapter.HTTPClient = server.Client()

	if _, err := adapter.DailySpendByCampaign(context.Background(), adaccount.Credentials{AdAccountID: "999", AccessToken: "t"}, time.Now(), time.Now()); err != nil {
		t.Fatalf("DailySpendByCampaign: %v", err)
	}
}
