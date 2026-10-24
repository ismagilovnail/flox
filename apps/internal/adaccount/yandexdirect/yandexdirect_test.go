package yandexdirect_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/yandexdirect"
)

var creds = adaccount.Credentials{AdAccountID: "shop-client", AccessToken: "tok"}

func day(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// fakeDirect answers clients.get with RUB and the report with 201, 202,
// then 200 — Direct's offline mode.
func fakeDirect(t *testing.T, polls *int32) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("Client-Login") != "shop-client" {
			t.Errorf("%s: Authorization %q, Client-Login %q", r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Client-Login"))
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/json/v5/clients":
			if body["method"] != "get" {
				t.Errorf("clients: method %v", body["method"])
			}
			_, _ = w.Write([]byte(`{"result": {"Clients": [{"Login": "shop-client", "Currency": "RUB"}]}}`))
		case "/json/v501/reports":
			if r.Header.Get("returnMoneyInMicros") != "false" || r.Header.Get("skipReportHeader") != "true" || r.Header.Get("skipReportSummary") != "true" {
				t.Errorf("report headers: %v", r.Header)
			}
			p := body["params"].(map[string]any)
			sel := p["SelectionCriteria"].(map[string]any)
			if p["ReportType"] != "CAMPAIGN_PERFORMANCE_REPORT" || p["DateRangeType"] != "CUSTOM_DATE" || p["Format"] != "TSV" ||
				p["IncludeVAT"] != "NO" || sel["DateFrom"] != "2026-10-01" || sel["DateTo"] != "2026-10-02" ||
				p["ReportName"] != "flox-spend-2026-10-01-2026-10-02" {
				t.Errorf("report params: %v", p)
			}
			switch atomic.AddInt32(polls, 1) {
			case 1:
				w.Header().Set("retryIn", "7")
				w.WriteHeader(http.StatusCreated)
			case 2:
				w.Header().Set("retryIn", "3")
				w.WriteHeader(http.StatusAccepted)
			default:
				_, _ = w.Write([]byte("Date\tCampaignId\tCost\n2026-10-01\t555\t1234.56\n2026-10-02\t555\t0.00\n2026-10-02\t777\t99.5\n"))
			}
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

// Audit UNI-3 part 1: the documented requests (Bearer token, Client-Login,
// money in units, no VAT), waiting as long as Direct says for an offline
// report, and the TSV read into per-day, per-campaign spend in the
// account's currency.
func TestDailySpendWaitsForTheOfflineReport(t *testing.T) {
	var polls int32
	var waited []time.Duration
	a := &yandexdirect.Adapter{BaseURL: fakeDirect(t, &polls).URL, Wait: func(_ context.Context, d time.Duration) error {
		waited = append(waited, d)
		return nil
	}}
	got, err := a.DailySpendByCampaign(context.Background(), creds, day("2026-10-01"), day("2026-10-02"))
	if err != nil {
		t.Fatal(err)
	}
	if polls != 3 || len(waited) != 2 || waited[0] != 7*time.Second || waited[1] != 3*time.Second {
		t.Fatalf("polls %d, waited %v", polls, waited)
	}
	want := []adaccount.DailyCampaignSpendRecord{
		{Date: day("2026-10-01"), ExternalCampaignID: "555", Amount: 1234.56, Currency: "RUB"},
		{Date: day("2026-10-02"), ExternalCampaignID: "555", Amount: 0, Currency: "RUB"},
		{Date: day("2026-10-02"), ExternalCampaignID: "777", Amount: 99.5, Currency: "RUB"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if !got[i].Date.Equal(want[i].Date) || got[i].ExternalCampaignID != want[i].ExternalCampaignID ||
			got[i].Amount != want[i].Amount || got[i].Currency != want[i].Currency {
			t.Fatalf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// Direct's error body (on 200 from clients.get, on 400 from reports)
// becomes an error with its code and text.
func TestErrorsCarryDirectsMessage(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/v5/clients" {
			_, _ = w.Write([]byte(`{"error": {"request_id": "1", "error_code": 53, "error_string": "Authorization error", "error_detail": "Invalid OAuth token"}}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": {"request_id": "2", "error_code": "4000", "error_string": "Invalid request parameters", "error_detail": "bad field"}}`))
	}))
	defer s.Close()
	a := &yandexdirect.Adapter{BaseURL: s.URL}
	_, err := a.DailySpendByCampaign(context.Background(), creds, day("2026-10-01"), day("2026-10-01"))
	var de *yandexdirect.Error
	if err == nil || !strings.Contains(err.Error(), "Invalid OAuth token") {
		t.Fatalf("clients error: %v", err)
	}
	if ok := errorAs(err, &de); !ok || de.Code != 53 {
		t.Fatalf("clients error: %#v", err)
	}
}

func errorAs(err error, target **yandexdirect.Error) bool {
	e, ok := err.(*yandexdirect.Error)
	if ok {
		*target = e
	}
	return ok
}

// A report Direct keeps building past the cap is an error, not a hang.
func TestAReportNeverReadyGivesUp(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/v5/clients" {
			_, _ = w.Write([]byte(`{"result": {"Clients": [{"Currency": "RUB"}]}}`))
			return
		}
		w.Header().Set("retryIn", "60")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer s.Close()
	clock := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	waits := 0
	a := &yandexdirect.Adapter{BaseURL: s.URL, Now: func() time.Time { return clock },
		Wait: func(_ context.Context, d time.Duration) error {
			waits++
			clock = clock.Add(d)
			return nil
		}}
	_, err := a.DailySpendByCampaign(context.Background(), creds, day("2026-10-01"), day("2026-10-01"))
	if err == nil || !strings.Contains(err.Error(), "not ready") || waits != 5 {
		t.Fatalf("err %v after %d waits, want not ready after 5 one-minute waits", err, waits)
	}
}
