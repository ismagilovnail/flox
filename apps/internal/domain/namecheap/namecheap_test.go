package namecheap_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/domain/namecheap"
)

func testFields() map[string]string {
	return map[string]string{"apiUser": "u", "apiKey": "k", "clientIp": "1.2.3.4"}
}

func TestDomainInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Command") != "namecheap.domains.getInfo" {
			t.Fatalf("Command = %q, want namecheap.domains.getInfo", r.URL.Query().Get("Command"))
		}
		if r.URL.Query().Get("DomainName") != "example.com" {
			t.Fatalf("DomainName = %q, want example.com", r.URL.Query().Get("DomainName"))
		}
		w.Write([]byte(`<ApiResponse Status="OK"><CommandResponse Type="namecheap.domains.getInfo">
			<DomainGetInfoResult Status="Ok" DomainName="example.com">
				<DomainDetails><CreatedDate>3/16/2015</CreatedDate><ExpiredDate>3/16/2027</ExpiredDate></DomainDetails>
			</DomainGetInfoResult>
		</CommandResponse></ApiResponse>`))
	}))
	defer server.Close()

	adapter := &namecheap.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	info, err := adapter.DomainInfo(context.Background(), testFields(), "example.com")
	if err != nil {
		t.Fatalf("DomainInfo: %v", err)
	}
	want := time.Date(2027, 3, 16, 0, 0, 0, 0, time.UTC)
	if !info.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", info.ExpiresAt, want)
	}
}

func TestDomainInfoPropagatesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<ApiResponse Status="ERROR"><Errors><Error Number="1011102">API Key is invalid</Error></Errors></ApiResponse>`))
	}))
	defer server.Close()

	adapter := &namecheap.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := adapter.DomainInfo(context.Background(), testFields(), "example.com")
	if err == nil {
		t.Fatal("DomainInfo with an API error succeeded, want an error")
	}
}

func TestRenew(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Command") != "namecheap.domains.renew" {
			t.Fatalf("Command = %q, want namecheap.domains.renew", r.URL.Query().Get("Command"))
		}
		if r.URL.Query().Get("Years") != "2" {
			t.Fatalf("Years = %q, want 2", r.URL.Query().Get("Years"))
		}
		w.Write([]byte(`<ApiResponse Status="OK"><CommandResponse Type="namecheap.domains.renew">
			<DomainRenewResult DomainName="example.com" Renew="true">
				<DomainDetails><ExpiredDate>3/16/2029</ExpiredDate></DomainDetails>
			</DomainRenewResult>
		</CommandResponse></ApiResponse>`))
	}))
	defer server.Close()

	adapter := &namecheap.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	info, err := adapter.Renew(context.Background(), testFields(), "example.com", 2)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	want := time.Date(2029, 3, 16, 0, 0, 0, 0, time.UTC)
	if !info.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt after renew = %v, want %v", info.ExpiresAt, want)
	}
}

func TestRenewPropagatesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<ApiResponse Status="ERROR"><Errors><Error Number="2011294">Insufficient funds</Error></Errors></ApiResponse>`))
	}))
	defer server.Close()

	adapter := &namecheap.Adapter{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := adapter.Renew(context.Background(), testFields(), "example.com", 1)
	if err == nil {
		t.Fatal("Renew with an API error succeeded, want an error")
	}
}
