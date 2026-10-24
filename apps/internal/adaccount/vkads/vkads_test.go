package vkads_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/vkads"
)

func day(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// fakeVK: an OAuth endpoint and the three reads. valid is the access
// token it takes; refreshOK whether a refresh is honoured; plans how many
// ad plans the account has (ids 1000…).
type fakeVK struct {
	mu        sync.Mutex
	valid     string
	refreshOK bool
	plans     int
	grants    []string
	statIDs   []int
	agency    string
}

func (f *fakeVK) server(t *testing.T) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path == "/api/v2/oauth2/token.json" {
			_ = r.ParseForm()
			if r.FormValue("client_id") != "cid" || r.FormValue("client_secret") != "csecret" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error": "invalid_client", "error_description": "bad client"}`))
				return
			}
			grant := r.FormValue("grant_type")
			f.grants = append(f.grants, grant)
			f.agency = r.FormValue("agency_client_name")
			if grant == "refresh_token" && (!f.refreshOK || r.FormValue("refresh_token") != "refresh-1") {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error": "invalid_grant", "error_description": "unknown refresh token"}`))
				return
			}
			f.valid = fmt.Sprintf("access-%d", len(f.grants))
			_, _ = fmt.Fprintf(w, `{"access_token": %q, "token_type": "bearer", "expires_in": "86400", "refresh_token": "refresh-1"}`, f.valid)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+f.valid {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error": {"code": "invalid_token", "message": "Unknown access token"}}`))
			return
		}
		q := r.URL.Query()
		switch r.URL.Path {
		case "/api/v3/user.json":
			_, _ = w.Write([]byte(`{"id": 1, "currency": "RUB", "username": "me"}`))
		case "/api/v2/ad_plans.json":
			if q.Get("_status__in") != "active,blocked,deleted" {
				t.Errorf("ad_plans status filter %q", q.Get("_status__in"))
			}
			limit, _ := strconv.Atoi(q.Get("limit"))
			offset, _ := strconv.Atoi(q.Get("offset"))
			var items []string
			for i := offset; i < min(offset+limit, f.plans); i++ {
				items = append(items, fmt.Sprintf(`{"id": %d, "name": "plan"}`, 1000+i))
			}
			_, _ = fmt.Fprintf(w, `{"count": %d, "offset": %d, "items": [%s]}`, f.plans, offset, strings.Join(items, ","))
		case "/api/v2/statistics/ad_plans/day.json":
			if q.Get("date_from") != "2026-10-01" || q.Get("date_to") != "2026-10-02" || q.Get("metrics") != "base" {
				t.Errorf("statistics query %v", q)
			}
			ids := strings.Split(q.Get("id"), ",")
			if len(ids) > 200 {
				t.Errorf("%d ids in one request", len(ids))
			}
			var items []string
			for _, id := range ids {
				n, _ := strconv.Atoi(id)
				f.statIDs = append(f.statIDs, n)
				items = append(items, fmt.Sprintf(`{"id": %d, "rows": [{"date": "2026-10-01", "base": {"shows": 10, "spent": "150.5"}}, {"date": "2026-10-02", "base": {"spent": "0"}}], "total": {"base": {"spent": "150.5"}}}`, n))
			}
			_, _ = fmt.Fprintf(w, `{"items": [%s], "total": {"base": {"spent": "0"}}}`, strings.Join(items, ","))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error": {"code": "ERR_WRONG_RESOURCE", "message": "no such method"}}`))
		}
	}))
	t.Cleanup(s.Close)
	return s
}

type saved struct{ access, refresh []string }

func credsSaving(s *saved, c adaccount.Credentials) adaccount.Credentials {
	c.SaveTokens = func(_ context.Context, access, refresh string) error {
		s.access = append(s.access, access)
		s.refresh = append(s.refresh, refresh)
		return nil
	}
	return c
}

// Audit UNI-3 part 2: with no token yet one is minted (client
// credentials) and kept; every ad plan is listed page by page, deleted
// ones too, and its daily spend read in batches of at most 200.
func TestDailySpendMintsAndKeepsAToken(t *testing.T) {
	f := &fakeVK{plans: 230}
	a := &vkads.Adapter{BaseURL: f.server(t).URL}
	var s saved
	got, err := a.DailySpendByCampaign(context.Background(), credsSaving(&s, adaccount.Credentials{ClientID: "cid", ClientSecret: "csecret"}),
		day("2026-10-01"), day("2026-10-02"))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.grants) != 1 || f.grants[0] != "client_credentials" || len(s.access) != 1 || s.access[0] != "access-1" || s.refresh[0] != "refresh-1" {
		t.Fatalf("grants %v, saved %+v", f.grants, s)
	}
	if len(f.statIDs) != 230 || len(got) != 460 {
		t.Fatalf("%d plans read, %d records", len(f.statIDs), len(got))
	}
	r := got[0]
	if r.ExternalCampaignID != "1000" || !r.Date.Equal(day("2026-10-01")) || r.Amount != 150.5 || r.Currency != "RUB" {
		t.Fatalf("first record %+v", r)
	}
}

// A stored token VK no longer takes is refreshed once; a refused refresh
// mints a new one; both are kept. An agency's client is minted with
// agency_client_credentials.
func TestATokenVKRefusesIsRenewed(t *testing.T) {
	f := &fakeVK{plans: 1, refreshOK: true}
	a := &vkads.Adapter{BaseURL: f.server(t).URL}
	var s saved
	creds := credsSaving(&s, adaccount.Credentials{ClientID: "cid", ClientSecret: "csecret", AccessToken: "stale", RefreshToken: "refresh-1"})
	if _, err := a.DailySpendByCampaign(context.Background(), creds, day("2026-10-01"), day("2026-10-02")); err != nil {
		t.Fatal(err)
	}
	if strings.Join(f.grants, ",") != "refresh_token" || len(s.access) != 1 {
		t.Fatalf("refresh: grants %v, saved %+v", f.grants, s)
	}

	f2 := &fakeVK{plans: 1, refreshOK: false}
	a2 := &vkads.Adapter{BaseURL: f2.server(t).URL}
	var s2 saved
	creds2 := credsSaving(&s2, adaccount.Credentials{AdAccountID: "client-of-agency", ClientID: "cid", ClientSecret: "csecret",
		AccessToken: "stale", RefreshToken: "dead"})
	if _, err := a2.DailySpendByCampaign(context.Background(), creds2, day("2026-10-01"), day("2026-10-02")); err != nil {
		t.Fatal(err)
	}
	if strings.Join(f2.grants, ",") != "refresh_token,agency_client_credentials" || f2.agency != "client-of-agency" || len(s2.access) != 1 {
		t.Fatalf("re-mint: grants %v, agency %q, saved %+v", f2.grants, f2.agency, s2)
	}
}

// A wrong client secret is VK's refusal with its reason; the token
// limit's 403 says what to do.
func TestRefusalsCarryVKsReason(t *testing.T) {
	f := &fakeVK{plans: 1}
	a := &vkads.Adapter{BaseURL: f.server(t).URL}
	_, err := a.DailySpendByCampaign(context.Background(), adaccount.Credentials{ClientID: "cid", ClientSecret: "wrong"}, day("2026-10-01"), day("2026-10-02"))
	if err == nil || !strings.Contains(err.Error(), "invalid_client") || !strings.Contains(err.Error(), "bad client") {
		t.Fatalf("wrong secret: %v", err)
	}

	limit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error": "access_denied", "error_description": "token limit"}`))
	}))
	defer limit.Close()
	_, err = (&vkads.Adapter{BaseURL: limit.URL}).DailySpendByCampaign(context.Background(), adaccount.Credentials{ClientID: "cid", ClientSecret: "csecret"}, day("2026-10-01"), day("2026-10-02"))
	if err == nil || !strings.Contains(err.Error(), "token/delete") {
		t.Fatalf("token limit: %v", err)
	}
}
