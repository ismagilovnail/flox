package googleads_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/googleads"
)

func testCreds() adaccount.Credentials {
	return adaccount.Credentials{
		AdAccountID:    "123-456-7890",
		DeveloperToken: "test-dev-token",
		ClientID:       "test-client-id",
		ClientSecret:   "test-client-secret",
		RefreshToken:   "test-refresh-token",
	}
}

func TestDailySpendByCampaign(t *testing.T) {
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parsing form: %v", err)
		}
		if r.FormValue("refresh_token") != "test-refresh-token" {
			t.Errorf("refresh_token = %q, want test-refresh-token", r.FormValue("refresh_token"))
		}
		if r.FormValue("client_id") != "test-client-id" || r.FormValue("client_secret") != "test-client-secret" {
			t.Errorf("client_id/client_secret = %q/%q, want test-client-id/test-client-secret", r.FormValue("client_id"), r.FormValue("client_secret"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token": "test-access-token", "expires_in": 3599}`))
	}))
	defer oauth.Close()

	var gotPath string
	ads := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if want := "Bearer test-access-token"; r.Header.Get("Authorization") != want {
			t.Errorf("Authorization = %q, want %q", r.Header.Get("Authorization"), want)
		}
		if r.Header.Get("developer-token") != "test-dev-token" {
			t.Errorf("developer-token = %q, want test-dev-token", r.Header.Get("developer-token"))
		}
		if want := "/customers/1234567890/googleAds:search"; r.URL.Path != want {
			t.Errorf("path = %q, want %q (dashes stripped from the customer id)", r.URL.Path, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{"campaign": {"id": "c1"}, "customer": {"currencyCode": "USD", "timeZone": "Europe/Berlin"}, "segments": {"date": "2026-01-01"}, "metrics": {"costMicros": "12500000"}},
				{"campaign": {"id": "c2"}, "customer": {"currencyCode": "USD"}, "segments": {"date": "2026-01-01"}, "metrics": {"costMicros": "3750000"}}
			]
		}`))
	}))
	defer ads.Close()

	adapter := &googleads.Adapter{OAuthBaseURL: oauth.URL, AdsBaseURL: ads.URL, HTTPClient: oauth.Client()}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	records, err := adapter.DailySpendByCampaign(context.Background(), testCreds(), from, from)
	if err != nil {
		t.Fatalf("DailySpendByCampaign: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if records[0].Timezone != "Europe/Berlin" {
		t.Fatalf("records[0].Timezone = %q, want customer.time_zone", records[0].Timezone)
	}
	if records[0].ExternalCampaignID != "c1" || records[0].Amount != 12.5 || records[0].Currency != "USD" {
		t.Fatalf("records[0] = %+v, unexpected (costMicros should divide by 1,000,000)", records[0])
	}
	if !records[0].Date.Equal(from) {
		t.Fatalf("records[0].Date = %v, want %v", records[0].Date, from)
	}
	if gotPath == "" {
		t.Fatal("ads server was never called")
	}
}

func TestDailySpendByCampaignFollowsPagination(t *testing.T) {
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token": "test-access-token"}`))
	}))
	defer oauth.Close()

	var calls int
	ads := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		if body["pageToken"] == "" {
			_, _ = w.Write([]byte(`{
				"results": [{"campaign": {"id": "c1"}, "customer": {"currencyCode": "USD"}, "segments": {"date": "2026-01-01"}, "metrics": {"costMicros": "1000000"}}],
				"nextPageToken": "page2"
			}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"results": [{"campaign": {"id": "c2"}, "customer": {"currencyCode": "USD"}, "segments": {"date": "2026-01-01"}, "metrics": {"costMicros": "2000000"}}]
		}`))
	}))
	defer ads.Close()

	adapter := &googleads.Adapter{OAuthBaseURL: oauth.URL, AdsBaseURL: ads.URL, HTTPClient: oauth.Client()}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	records, err := adapter.DailySpendByCampaign(context.Background(), testCreds(), from, from)
	if err != nil {
		t.Fatalf("DailySpendByCampaign: %v", err)
	}
	if calls != 2 {
		t.Fatalf("ads server called %d times, want 2 (should follow nextPageToken)", calls)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
}

func TestDailySpendByCampaignPropagatesTokenRefreshError(t *testing.T) {
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error": "invalid_grant", "error_description": "Token has been expired or revoked."}`))
	}))
	defer oauth.Close()
	ads := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("ads server should never be called when token refresh fails")
	}))
	defer ads.Close()

	adapter := &googleads.Adapter{OAuthBaseURL: oauth.URL, AdsBaseURL: ads.URL, HTTPClient: oauth.Client()}
	_, err := adapter.DailySpendByCampaign(context.Background(), testCreds(), time.Now(), time.Now())
	if err == nil {
		t.Fatal("DailySpendByCampaign with a failing token refresh succeeded, want an error")
	}
}

func TestDailySpendByCampaignPropagatesSearchAPIError(t *testing.T) {
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token": "test-access-token"}`))
	}))
	defer oauth.Close()
	ads := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error": {"code": 400, "message": "Invalid customer ID", "status": "INVALID_ARGUMENT"}}`))
	}))
	defer ads.Close()

	adapter := &googleads.Adapter{OAuthBaseURL: oauth.URL, AdsBaseURL: ads.URL, HTTPClient: oauth.Client()}
	_, err := adapter.DailySpendByCampaign(context.Background(), testCreds(), time.Now(), time.Now())
	if err == nil {
		t.Fatal("DailySpendByCampaign with a search API error succeeded, want an error")
	}
}

func TestDailySpendByCampaignRejectsMalformedCostMicros(t *testing.T) {
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token": "test-access-token"}`))
	}))
	defer oauth.Close()
	ads := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results": [{"campaign": {"id": "c1"}, "customer": {"currencyCode": "USD"}, "segments": {"date": "2026-01-01"}, "metrics": {"costMicros": "not-a-number"}}]}`))
	}))
	defer ads.Close()

	adapter := &googleads.Adapter{OAuthBaseURL: oauth.URL, AdsBaseURL: ads.URL, HTTPClient: oauth.Client()}
	_, err := adapter.DailySpendByCampaign(context.Background(), testCreds(), time.Now(), time.Now())
	if err == nil {
		t.Fatal("DailySpendByCampaign with a malformed costMicros succeeded, want an error")
	}
}
