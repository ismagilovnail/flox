package cloudflare_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/domain/cloudflare"
)

func testFields() map[string]string { return map[string]string{"apiToken": "tok"} }

func TestZoneStatus(t *testing.T) {
	var gotPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path+"?"+r.URL.RawQuery)
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Fatalf("Authorization = %q, want Bearer tok", r.Header.Get("Authorization"))
		}
		switch {
		case r.URL.Path == "/zones":
			if r.URL.Query().Get("name") != "example.com" {
				t.Fatalf("zone lookup name = %q, want example.com (the apex, not the tracking subdomain)", r.URL.Query().Get("name"))
			}
			w.Write([]byte(`{"success":true,"result":[{"id":"zone123","status":"active"}]}`))
		case r.URL.Path == "/zones/zone123/ssl/verification":
			w.Write([]byte(`{"success":true,"result":[{"certificate_status":"active"}]}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	adapter := &cloudflare.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	status, err := adapter.ZoneStatus(context.Background(), testFields(), "track.example.com")
	if err != nil {
		t.Fatalf("ZoneStatus: %v", err)
	}
	if !status.Active || !status.SSLIssued {
		t.Fatalf("status = %+v, want both Active and SSLIssued true", status)
	}
	if len(gotPaths) != 2 {
		t.Fatalf("server hit %d times, want 2 (zone lookup + ssl verification)", len(gotPaths))
	}
}

func TestZoneStatusPendingCertificate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/zones" {
			w.Write([]byte(`{"success":true,"result":[{"id":"zone123","status":"active"}]}`))
			return
		}
		w.Write([]byte(`{"success":true,"result":[{"certificate_status":"pending_validation"}]}`))
	}))
	defer server.Close()

	adapter := &cloudflare.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	status, err := adapter.ZoneStatus(context.Background(), testFields(), "track.example.com")
	if err != nil {
		t.Fatalf("ZoneStatus: %v", err)
	}
	if !status.Active || status.SSLIssued {
		t.Fatalf("status = %+v, want Active true and SSLIssued false (cert still pending)", status)
	}
}

func TestZoneStatusNoZoneFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"result":[]}`))
	}))
	defer server.Close()

	adapter := &cloudflare.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := adapter.ZoneStatus(context.Background(), testFields(), "track.example.com")
	if err == nil {
		t.Fatal("ZoneStatus with no matching zone succeeded, want an error")
	}
}

func TestEnsureRecordCreatesWhenMissing(t *testing.T) {
	var createBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/zones":
			w.Write([]byte(`{"success":true,"result":[{"id":"zone123","status":"active"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/zones/zone123/dns_records":
			w.Write([]byte(`{"success":true,"result":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/zones/zone123/dns_records":
			_ = json.NewDecoder(r.Body).Decode(&createBody)
			w.Write([]byte(`{"success":true,"result":{"id":"rec1"}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	adapter := &cloudflare.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	if err := adapter.EnsureRecord(context.Background(), testFields(), "track.example.com", "tracker.floxlink.io"); err != nil {
		t.Fatalf("EnsureRecord: %v", err)
	}
	if createBody["type"] != "CNAME" || createBody["name"] != "track.example.com" || createBody["content"] != "tracker.floxlink.io" {
		t.Fatalf("create body = %+v, unexpected", createBody)
	}
}

func TestEnsureRecordUpdatesWhenPointingElsewhere(t *testing.T) {
	var updateCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/zones":
			w.Write([]byte(`{"success":true,"result":[{"id":"zone123","status":"active"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/zones/zone123/dns_records":
			w.Write([]byte(`{"success":true,"result":[{"id":"rec1","content":"old-target.example.net"}]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/zones/zone123/dns_records/rec1":
			updateCalled = true
			w.Write([]byte(`{"success":true,"result":{"id":"rec1"}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	adapter := &cloudflare.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	if err := adapter.EnsureRecord(context.Background(), testFields(), "track.example.com", "tracker.floxlink.io"); err != nil {
		t.Fatalf("EnsureRecord: %v", err)
	}
	if !updateCalled {
		t.Fatal("PUT was never called to update the existing record")
	}
}

func TestEnsureRecordNoOpWhenAlreadyCorrect(t *testing.T) {
	writeAttempted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/zones":
			w.Write([]byte(`{"success":true,"result":[{"id":"zone123","status":"active"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/zones/zone123/dns_records":
			w.Write([]byte(`{"success":true,"result":[{"id":"rec1","content":"tracker.floxlink.io"}]}`))
		default:
			writeAttempted = true
			t.Fatalf("unexpected write request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	adapter := &cloudflare.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	if err := adapter.EnsureRecord(context.Background(), testFields(), "track.example.com", "tracker.floxlink.io"); err != nil {
		t.Fatalf("EnsureRecord: %v", err)
	}
	if writeAttempted {
		t.Fatal("a write was attempted even though the record already pointed at the right target")
	}
}
