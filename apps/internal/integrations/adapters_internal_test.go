package integrations

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// These test each adapter's real HTTP request/response handling against a
// fake server standing in for the real API — an internal (white-box) test
// file since the adapter types are unexported implementation details of
// Service, not part of this package's public API.

func TestNamecheapAdapterVerify(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Command") != "namecheap.users.getBalances" {
			t.Fatalf("Command = %q, want namecheap.users.getBalances", r.URL.Query().Get("Command"))
		}
		if r.URL.Query().Get("ApiKey") == "bad-key" {
			w.Write([]byte(`<ApiResponse Status="ERROR"><Errors><Error Number="1011102">API Key is invalid</Error></Errors></ApiResponse>`))
			return
		}
		w.Write([]byte(`<ApiResponse Status="OK"><CommandResponse></CommandResponse></ApiResponse>`))
	}))
	defer srv.Close()

	a := &namecheapAdapter{BaseURL: srv.URL, HTTPClient: srv.Client()}

	if err := a.Verify(context.Background(), map[string]string{"apiUser": "u", "apiKey": "good-key", "clientIp": "1.2.3.4"}); err != nil {
		t.Fatalf("Verify (valid credentials) = %v, want nil", err)
	}
	err := a.Verify(context.Background(), map[string]string{"apiUser": "u", "apiKey": "bad-key", "clientIp": "1.2.3.4"})
	if err == nil {
		t.Fatal("Verify (invalid key) succeeded, want an error")
	}
}

func TestCloudflareAdapterVerify(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-token" {
			w.Write([]byte(`{"success":false,"errors":[{"message":"Invalid API Token"}]}`))
			return
		}
		w.Write([]byte(`{"success":true,"result":{"status":"active"},"errors":[]}`))
	}))
	defer srv.Close()

	a := &cloudflareAdapter{BaseURL: srv.URL, HTTPClient: srv.Client()}

	if err := a.Verify(context.Background(), map[string]string{"apiToken": "good-token"}); err != nil {
		t.Fatalf("Verify (valid token) = %v, want nil", err)
	}
	err := a.Verify(context.Background(), map[string]string{"apiToken": "wrong-token"})
	if err == nil {
		t.Fatal("Verify (invalid token) succeeded, want an error")
	}
}

func TestSlackAdapterVerify(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	a := &slackAdapter{HTTPClient: srv.Client()}
	if err := a.post(context.Background(), srv.URL); err != nil {
		t.Fatalf("post = %v, want nil", err)
	}
}

func TestSlackAdapterVerifyRejectsUnsafeURL(t *testing.T) {
	a := &slackAdapter{HTTPClient: http.DefaultClient}
	err := a.Verify(context.Background(), map[string]string{"webhookUrl": "http://169.254.169.254/latest/meta-data/"})
	if err == nil {
		t.Fatal("Verify against a metadata-service URL succeeded, want it rejected by urlsafety")
	}
}

func TestSlackAdapterVerifyFailureResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("channel_not_found"))
	}))
	defer srv.Close()

	a := &slackAdapter{HTTPClient: srv.Client()}
	err := a.post(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("post against a failing webhook succeeded, want an error")
	}
}
