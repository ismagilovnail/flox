package pixelsend_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/macro"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
	"github.com/ismagilovnail/flox/apps/internal/pixelsend"
)

// captured is one request a fake platform received.
type captured struct {
	method, path string
	query        map[string][]string
	header       http.Header
	body         map[string]any
}

func platform(t *testing.T, status int, answer string) (*httptest.Server, *[]captured) {
	t.Helper()
	var got []captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		c := captured{method: r.Method, path: r.URL.Path, query: r.URL.Query(), header: r.Header.Clone()}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &c.body); err != nil {
				t.Errorf("request body is not JSON: %s", raw)
			}
		}
		got = append(got, c)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func sampleEvent() pixel.Event {
	rev, usd := 50.0, 54.25
	return pixel.Event{
		OrganizationID: "org", StreamSetID: "ss", SourceEventID: "01JEVENT",
		Type:    event.CpaAccept,
		EventAt: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC),
		ClickID: "01JCLICK", ClickAt: time.Date(2026, 10, 11, 11, 0, 0, 0, time.UTC),
		Revenue: &rev, Currency: "eur", USDValue: &usd,
		IP: "203.0.113.7", UserAgent: "Mozilla/5.0 test", FBClickID: "fbclid-1", TTClickID: "ttclid-1",
		Macros: macro.Values{ClickID: "01JCLICK", CampaignID: "camp-1", Country: "DE", Subs: [10]string{"s1"}},
	}
}

func dig(m map[string]any, path ...string) any {
	var cur any = m
	for _, p := range path {
		switch v := cur.(type) {
		case map[string]any:
			cur = v[p]
		case []any:
			if len(v) == 0 {
				return nil
			}
			cur = v[0].(map[string]any)[p]
		default:
			return nil
		}
	}
	return cur
}

func TestFacebookSendsConversionsAPIEvent(t *testing.T) {
	srv, got := platform(t, http.StatusOK, `{"events_received":1}`)
	fb := &pixelsend.Facebook{Client: srv.Client(), BaseURL: srv.URL}
	creds := pixel.Credentials{Provider: pixel.ProviderFacebook, PixelID: "1234", AccessToken: "EAAB-token", TestEventCode: "TEST42"}

	out := fb.Send(context.Background(), creds, sampleEvent())
	if out.Err != nil || out.StatusCode != 200 {
		t.Fatalf("Send = %+v, want success", out)
	}
	req := (*got)[0]
	if req.method != http.MethodPost || req.path != "/1234/events" || req.query["access_token"][0] != "EAAB-token" {
		t.Fatalf("request = %s %s %v", req.method, req.path, req.query)
	}
	b := req.body
	checks := map[string]any{
		"event_name":    "Purchase",
		"event_id":      "01JEVENT",
		"action_source": "website",
		"event_time":    float64(sampleEvent().EventAt.Unix()),
	}
	for k, want := range checks {
		if v := dig(b, "data", k); v != want {
			t.Errorf("data.%s = %v, want %v", k, v, want)
		}
	}
	if v, want := dig(b, "data", "user_data", "fbc"), fmt.Sprintf("fb.1.%d.fbclid-1", sampleEvent().ClickAt.UnixMilli()); v != want {
		t.Errorf("fbc = %v", v)
	}
	if v := dig(b, "data", "user_data", "client_ip_address"); v != "203.0.113.7" {
		t.Errorf("client_ip_address = %v", v)
	}
	ext, _ := dig(b, "data", "user_data", "external_id").([]any)
	if len(ext) != 1 || len(ext[0].(string)) != 64 || strings.Contains(ext[0].(string), "01JCLICK") {
		t.Errorf("external_id = %v, want one SHA-256 hex of the click id", ext)
	}
	if dig(b, "data", "custom_data", "value") != 54.25 || dig(b, "data", "custom_data", "currency") != "USD" {
		t.Errorf("custom_data = %v, want the USD value", dig(b, "data", "custom_data"))
	}
	if b["test_event_code"] != "TEST42" {
		t.Errorf("test_event_code = %v", b["test_event_code"])
	}
}

func TestFacebookErrors(t *testing.T) {
	srv, _ := platform(t, http.StatusBadRequest, `{"error":{"message":"Invalid OAuth access token.","code":190}}`)
	fb := &pixelsend.Facebook{Client: srv.Client(), BaseURL: srv.URL}
	creds := pixel.Credentials{Provider: pixel.ProviderFacebook, PixelID: "1234", AccessToken: "secret-tok"}
	out := fb.Send(context.Background(), creds, sampleEvent())
	if out.Err == nil || out.Retry || !strings.Contains(out.Err.Error(), "Invalid OAuth access token") {
		t.Fatalf("bad token: %+v, want a permanent failure with the platform's message", out)
	}

	throttled, _ := platform(t, http.StatusBadRequest, `{"error":{"message":"Application request limit reached","code":4}}`)
	fb.BaseURL = throttled.URL
	if out := fb.Send(context.Background(), creds, sampleEvent()); !out.Retry {
		t.Fatalf("throttled: %+v, want retry", out)
	}

	// A transport failure must not print the URL, which holds the token.
	fb.BaseURL = "http://127.0.0.1:1"
	out = fb.Send(context.Background(), creds, sampleEvent())
	if out.Err == nil || !out.Retry || strings.Contains(out.Err.Error(), "secret-tok") {
		t.Fatalf("unreachable: %+v, want a retryable failure without the token", out)
	}

	out = fb.Send(context.Background(), pixel.Credentials{Provider: pixel.ProviderFacebook, PixelID: "1"}, sampleEvent())
	if out.Err == nil || out.Retry {
		t.Fatalf("no token: %+v, want a permanent configuration failure", out)
	}
}

func TestTikTokReadsCodeFromBody(t *testing.T) {
	srv, got := platform(t, http.StatusOK, `{"code":0,"message":"OK"}`)
	tt := &pixelsend.TikTok{Client: srv.Client(), Endpoint: srv.URL}
	creds := pixel.Credentials{Provider: pixel.ProviderTikTok, PixelID: "CPIX", AccessToken: "tt-token",
		EventNames: map[string]string{"CPA_ACCEPT": "Purchase"}}

	if out := tt.Send(context.Background(), creds, sampleEvent()); out.Err != nil {
		t.Fatalf("Send = %+v, want success", out)
	}
	req := (*got)[0]
	if req.header.Get("Access-Token") != "tt-token" {
		t.Fatalf("Access-Token header = %q", req.header.Get("Access-Token"))
	}
	b := req.body
	if b["event_source_id"] != "CPIX" || b["event_source"] != "web" {
		t.Errorf("source = %v/%v", b["event_source"], b["event_source_id"])
	}
	if dig(b, "data", "event") != "Purchase" || dig(b, "data", "user", "ttclid") != "ttclid-1" {
		t.Errorf("data = %v, want the override name and the ttclid", b["data"])
	}

	bad, _ := platform(t, http.StatusOK, `{"code":40001,"message":"Access token invalid"}`)
	tt.Endpoint = bad.URL
	if out := tt.Send(context.Background(), creds, sampleEvent()); out.Err == nil || out.Retry {
		t.Fatalf("code 40001: %+v, want a permanent failure", out)
	}
	limited, _ := platform(t, http.StatusOK, `{"code":40100,"message":"Too many requests"}`)
	tt.Endpoint = limited.URL
	if out := tt.Send(context.Background(), creds, sampleEvent()); out.Err == nil || !out.Retry {
		t.Fatalf("code 40100: %+v, want retry", out)
	}
}

func TestSnapchatAndNoValue(t *testing.T) {
	srv, got := platform(t, http.StatusOK, `{"status":"VALID"}`)
	sc := &pixelsend.Snapchat{Client: srv.Client(), BaseURL: srv.URL}
	ev := sampleEvent()
	ev.Revenue, ev.USDValue, ev.Currency = nil, nil, ""
	creds := pixel.Credentials{Provider: pixel.ProviderSnapchat, PixelID: "snap-px", AccessToken: "snap-token"}
	if out := sc.Send(context.Background(), creds, ev); out.Err != nil {
		t.Fatalf("Send = %+v", out)
	}
	req := (*got)[0]
	if req.path != "/snap-px/events" || req.query["access_token"][0] != "snap-token" {
		t.Fatalf("request = %s %v", req.path, req.query)
	}
	if dig(req.body, "data", "event_name") != "PURCHASE" || dig(req.body, "data", "action_source") != "WEB" {
		t.Errorf("data = %v", req.body["data"])
	}
	if dig(req.body, "data", "custom_data") != nil {
		t.Errorf("custom_data = %v, want none without a known value (never a made-up 0)", dig(req.body, "data", "custom_data"))
	}
}

func TestGenericFillsMacros(t *testing.T) {
	srv, got := platform(t, http.StatusNoContent, ``)
	g := &pixelsend.Generic{Client: srv.Client()}
	creds := pixel.Credentials{Provider: pixel.ProviderGeneric,
		URL: srv.URL + "/px?cid={click_id}&e={status}&rev={revenue}&cur={currency}&c={campaign_id}&s1={sub1}"}
	if out := g.Send(context.Background(), creds, sampleEvent()); out.Err != nil {
		t.Fatalf("Send = %+v", out)
	}
	q := (*got)[0].query
	want := map[string]string{"cid": "01JCLICK", "e": "CPA_ACCEPT", "rev": "50", "cur": "eur", "c": "camp-1", "s1": "s1"}
	for k, v := range want {
		if q[k][0] != v {
			t.Errorf("%s = %q, want %q", k, q[k][0], v)
		}
	}

	failing, _ := platform(t, http.StatusNotFound, ``)
	creds.URL = failing.URL + "/missing"
	if out := g.Send(context.Background(), creds, sampleEvent()); out.Err == nil || out.Retry {
		t.Fatalf("404: %+v, want a permanent failure", out)
	}
	down, _ := platform(t, http.StatusBadGateway, ``)
	creds.URL = down.URL
	if out := g.Send(context.Background(), creds, sampleEvent()); out.Err == nil || !out.Retry {
		t.Fatalf("502: %+v, want retry", out)
	}
}

func TestSendersCoverEveryProvider(t *testing.T) {
	senders := pixelsend.Senders(http.DefaultClient)
	for _, p := range []pixel.Provider{pixel.ProviderFacebook, pixel.ProviderTikTok, pixel.ProviderSnapchat, pixel.ProviderGeneric} {
		if _, ok := senders[p]; !ok || !p.Valid() {
			t.Errorf("provider %s: has sender %v, valid %v", p, ok, p.Valid())
		}
	}
	if len(senders) != 4 || pixel.Provider("twitter").Valid() {
		t.Errorf("senders = %d, twitter valid %v; want 4 and no twitter", len(senders), pixel.Provider("twitter").Valid())
	}
}
