package tiktokads_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/tiktokads"
)

func newFakeServer(t *testing.T, reportPages ...string) *httptest.Server {
	t.Helper()
	call := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Access-Token") != "test-token" {
			t.Errorf("Access-Token header = %q, want test-token", r.Header.Get("Access-Token"))
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/advertiser/info/":
			_, _ = w.Write([]byte(`{"code":0,"message":"OK","data":{"list":[{"advertiser_id":"123","currency":"USD"}]}}`))
		case "/report/integrated/get/":
			if call >= len(reportPages) {
				t.Fatalf("report endpoint called %d times, only %d fake pages configured", call+1, len(reportPages))
			}
			page := reportPages[call]
			call++
			_, _ = w.Write([]byte(page))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
}

func TestDailySpendByCampaign(t *testing.T) {
	server := newFakeServer(t, `{
		"code": 0,
		"message": "OK",
		"data": {
			"list": [
				{"dimensions": {"campaign_id": "c1", "stat_time_day": "2026-01-01 00:00:00"}, "metrics": {"spend": "12.50"}},
				{"dimensions": {"campaign_id": "c2", "stat_time_day": "2026-01-01 00:00:00"}, "metrics": {"spend": "3.75"}}
			],
			"page_info": {"page": 1, "total_page": 1}
		}
	}`)
	defer server.Close()

	adapter := &tiktokads.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	records, err := adapter.DailySpendByCampaign(context.Background(), adaccount.Credentials{AdAccountID: "123", AccessToken: "test-token"}, from, from)
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
}

func TestDailySpendByCampaignFollowsPagination(t *testing.T) {
	server := newFakeServer(t,
		`{"code":0,"message":"OK","data":{"list":[{"dimensions":{"campaign_id":"c1","stat_time_day":"2026-01-01 00:00:00"},"metrics":{"spend":"10"}}],"page_info":{"page":1,"total_page":2}}}`,
		`{"code":0,"message":"OK","data":{"list":[{"dimensions":{"campaign_id":"c2","stat_time_day":"2026-01-02 00:00:00"},"metrics":{"spend":"20"}}],"page_info":{"page":2,"total_page":2}}}`,
	)
	defer server.Close()

	adapter := &tiktokads.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	records, err := adapter.DailySpendByCampaign(context.Background(), adaccount.Credentials{AdAccountID: "123", AccessToken: "test-token"}, from, to)
	if err != nil {
		t.Fatalf("DailySpendByCampaign: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2 across both pages", len(records))
	}
}

func TestDailySpendByCampaignPropagatesReportAPIError(t *testing.T) {
	server := newFakeServer(t, `{"code": 40001, "message": "Invalid access token"}`)
	defer server.Close()

	adapter := &tiktokads.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := adapter.DailySpendByCampaign(context.Background(), adaccount.Credentials{AdAccountID: "123", AccessToken: "test-token"}, time.Now(), time.Now())
	if err == nil {
		t.Fatal("DailySpendByCampaign returned no error for a non-zero code report response, want an error")
	}
}

func TestDailySpendByCampaignPropagatesAdvertiserInfoError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code": 40100, "message": "Advertiser not found"}`))
	}))
	defer server.Close()

	adapter := &tiktokads.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := adapter.DailySpendByCampaign(context.Background(), adaccount.Credentials{AdAccountID: "bogus", AccessToken: "test-token"}, time.Now(), time.Now())
	if err == nil {
		t.Fatal("DailySpendByCampaign returned no error when advertiser/info/ failed, want an error")
	}
}

func TestDailySpendByCampaignRejectsMalformedSpend(t *testing.T) {
	server := newFakeServer(t, `{"code":0,"message":"OK","data":{"list":[{"dimensions":{"campaign_id":"c1","stat_time_day":"2026-01-01 00:00:00"},"metrics":{"spend":"not-a-number"}}],"page_info":{"page":1,"total_page":1}}}`)
	defer server.Close()

	adapter := &tiktokads.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := adapter.DailySpendByCampaign(context.Background(), adaccount.Credentials{AdAccountID: "123", AccessToken: "test-token"}, time.Now(), time.Now())
	if err == nil {
		t.Fatal("DailySpendByCampaign accepted a non-numeric spend value, want an error")
	}
}
