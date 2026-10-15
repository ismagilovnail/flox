package cloudflare_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/domain/cloudflare"
)

// fakeRegistrarAPI serves /accounts and, per account, registrations
// (the beta endpoint) and domains (the older one) by apex.
type fakeRegistrarAPI struct {
	accounts      []string
	registrations map[string]string // "account/apex" → result JSON
	domains       map[string]string
	accountsCode  int
	regCode       int
	hits          []string
}

func (f *fakeRegistrarAPI) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits = append(f.hits, r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer "+secretToken {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		notFound := func() {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"success":false,"errors":[{"code":7003,"message":"Could not route"}]}`))
		}
		if r.URL.Path == "/accounts" {
			if f.accountsCode != 0 {
				w.WriteHeader(f.accountsCode)
				w.Write([]byte(`{"success":false,"errors":[{"message":"Authentication error"}]}`))
				return
			}
			parts := []string{}
			for _, a := range f.accounts {
				parts = append(parts, `{"id":"`+a+`"}`)
			}
			w.Write([]byte(`{"success":true,"result":[` + strings.Join(parts, ",") + `]}`))
			return
		}
		// /accounts/{account}/registrar/{registrations|domains}/{apex}
		p := strings.Split(strings.TrimPrefix(r.URL.Path, "/accounts/"), "/")
		if len(p) != 4 || p[1] != "registrar" {
			t.Errorf("unexpected path %q", r.URL.Path)
			notFound()
			return
		}
		if f.regCode != 0 {
			w.WriteHeader(f.regCode)
			w.Write([]byte(`{"success":false,"errors":[{"message":"forbidden"}]}`))
			return
		}
		table := f.registrations
		if p[2] == "domains" {
			table = f.domains
		}
		result, ok := table[p[0]+"/"+p[3]]
		if !ok {
			notFound()
			return
		}
		w.Write([]byte(`{"success":true,"result":` + result + `}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// secretToken must never show up in an error.
const secretToken = "cf-secret-7f3a"

func registrarFields() map[string]string { return map[string]string{"apiToken": secretToken} }

func registrarFor(srv *httptest.Server) *cloudflare.Registrar {
	return &cloudflare.Registrar{Adapter: cloudflare.Adapter{BaseURL: srv.URL, HTTPClient: srv.Client()}}
}

// The beta registrations endpoint, in the second of two accounts, for the
// tracking hostname's apex: expiry and auto-renew.
func TestRegistrarDomainInfo(t *testing.T) {
	f := &fakeRegistrarAPI{
		accounts: []string{"acc-a", "acc-b"},
		registrations: map[string]string{
			"acc-b/example.com": `{"domain_name":"example.com","status":"active","expires_at":"2027-03-01T10:00:00Z","auto_renew":false,"locked":true}`,
		},
	}
	info, err := registrarFor(f.server(t)).DomainInfo(context.Background(), registrarFields(), "track.example.com")
	if err != nil {
		t.Fatalf("DomainInfo: %v", err)
	}
	if !info.ExpiresAt.Equal(time.Date(2027, 3, 1, 10, 0, 0, 0, time.UTC)) || info.AutoRenew == nil || *info.AutoRenew {
		t.Fatalf("info = %+v (auto-renew %v), want 2027-03-01 and auto-renew off", info, info.AutoRenew)
	}
}

// An account whose beta endpoint does not know the domain falls back to
// the older domains endpoint, which has no auto_renew.
func TestRegistrarDomainInfoFallsBackToDomainsEndpoint(t *testing.T) {
	f := &fakeRegistrarAPI{
		accounts: []string{"acc-a"},
		domains:  map[string]string{"acc-a/example.com": `{"expires_at":"2026-12-24T00:00:00Z","locked":false,"can_register":false}`},
	}
	info, err := registrarFor(f.server(t)).DomainInfo(context.Background(), registrarFields(), "example.com")
	if err != nil {
		t.Fatalf("DomainInfo: %v", err)
	}
	if info.ExpiresAt.Year() != 2026 || info.AutoRenew != nil {
		t.Fatalf("info = %+v, want the expiry and no auto-renew value", info)
	}
	if strings.Join(f.hits, " ") != "/accounts /accounts/acc-a/registrar/registrations/example.com /accounts/acc-a/registrar/domains/example.com" {
		t.Fatalf("calls = %v", f.hits)
	}
}

func TestRegistrarDomainInfoErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		api  *fakeRegistrarAPI
		want string
	}{
		"not registered here": {&fakeRegistrarAPI{accounts: []string{"acc-a"}}, "not registered with Cloudflare Registrar"},
		"no account":          {&fakeRegistrarAPI{}, "sees no account"},
		"cannot list":         {&fakeRegistrarAPI{accountsCode: http.StatusForbidden}, "Registrar: Read"},
		"cannot read":         {&fakeRegistrarAPI{accounts: []string{"acc-a"}, regCode: http.StatusForbidden}, "Registrar: Read"},
		"still pending": {&fakeRegistrarAPI{accounts: []string{"acc-a"}, registrations: map[string]string{
			"acc-a/example.com": `{"status":"registration_pending","expires_at":null}`}}, "registration_pending"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := registrarFor(tc.api.server(t)).DomainInfo(context.Background(), registrarFields(), "example.com")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one mentioning %q", err, tc.want)
			}
			if strings.Contains(err.Error(), secretToken) {
				t.Fatalf("err %q carries the token", err)
			}
		})
	}
}

// Renew never calls Cloudflare: its API has no renewal.
func TestRegistrarRenewUnsupported(t *testing.T) {
	f := &fakeRegistrarAPI{accounts: []string{"acc-a"}}
	_, err := registrarFor(f.server(t)).Renew(context.Background(), registrarFields(), "example.com", 1)
	if !errors.Is(err, cloudflare.ErrRenewUnsupported) || len(f.hits) != 0 {
		t.Fatalf("Renew = %v after %d calls, want ErrRenewUnsupported and none", err, len(f.hits))
	}
}
