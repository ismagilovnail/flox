package domain_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/domain"
	"github.com/ismagilovnail/flox/apps/internal/domain/tlsprobe"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/integrations"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
)

// fakeCredentialSource/fakeRegistrar/fakeDNS let tests control Verify/
// Renew/SyncDNS's real-provider outcomes without a real Namecheap/
// Cloudflare account — this project has none to exercise the real
// adapters against.
type fakeCredentialSource struct {
	creds map[integrations.Provider]map[string]string
}

func (f *fakeCredentialSource) CredentialsFor(_ context.Context, _ string, provider integrations.Provider) (map[string]string, bool, error) {
	c, ok := f.creds[provider]
	return c, ok, nil
}

type fakeRegistrar struct {
	info       domain.RegistrarInfo
	err        error
	renewCalls int
}

func (f *fakeRegistrar) DomainInfo(context.Context, map[string]string, string) (domain.RegistrarInfo, error) {
	return f.info, f.err
}

func (f *fakeRegistrar) Renew(context.Context, map[string]string, string, int) (domain.RegistrarInfo, error) {
	f.renewCalls++
	return f.info, f.err
}

type fakeDNS struct {
	status      domain.DNSZoneStatus
	err         error
	ensureCalls int
	lastTarget  string
}

func (f *fakeDNS) ZoneStatus(context.Context, map[string]string, string) (domain.DNSZoneStatus, error) {
	return f.status, f.err
}

func (f *fakeDNS) EnsureRecord(_ context.Context, _ map[string]string, _, target string) error {
	f.ensureCalls++
	f.lastTarget = target
	return f.err
}

func newTestService(pool *pgxpool.Pool, creds *fakeCredentialSource, registrar *fakeRegistrar, dns *fakeDNS, trackerDNSTarget string) *domain.Service {
	if creds == nil {
		creds = &fakeCredentialSource{creds: map[integrations.Provider]map[string]string{}}
	}
	if registrar == nil {
		registrar = &fakeRegistrar{}
	}
	if dns == nil {
		dns = &fakeDNS{}
	}
	return domain.NewService(domain.NewRepository(pool), creds, registrar, dns, trackerDNSTarget, &fakeProber{})
}

// fakeProber answers each host with a canned TLS probe result.
type fakeProber struct {
	results map[string]tlsprobe.Result
	hosts   []string
}

func (f *fakeProber) Probe(_ context.Context, host string) tlsprobe.Result {
	f.hosts = append(f.hosts, host)
	if r, ok := f.results[host]; ok {
		return r
	}
	return tlsprobe.Result{Outcome: tlsprobe.None, Err: "no such host"}
}

func TestCreateGetUpdateDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := newTestService(pool, nil, nil, nil, "")

	created, err := svc.Create(ctx, orgID, domain.CreateInput{
		Domain:      "Track.Example.com",
		Purpose:     []string{"tracking"},
		Registrar:   domain.RegistrarUnmanaged,
		DNSProvider: domain.DNSProviderUnmanaged,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Domain != "track.example.com" {
		t.Fatalf("Domain = %q, want lowercased", created.Domain)
	}
	if created.Status != domain.StatusPending || created.OwnershipVerifiedAt != nil {
		t.Fatalf("Status = %q verified %v, want pending and unverified until its DNS is proved (T6)", created.Status, created.OwnershipVerifiedAt)
	}
	if created.SSL != domain.SSLNone {
		t.Fatalf("SSL = %q, want none (nothing issues certs yet)", created.SSL)
	}
	if created.VerifiedAt != nil || created.ExpiresAt != nil {
		t.Fatalf("VerifiedAt/ExpiresAt should stay nil, got %+v", created)
	}

	got, err := svc.Get(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("Get returned a different domain")
	}

	status := domain.StatusExpired
	updated, err := svc.Update(ctx, orgID, created.ID, domain.UpdateInput{Status: &status})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Status != domain.StatusExpired {
		t.Fatalf("Status after update = %q, want expired", updated.Status)
	}

	if err := svc.Delete(ctx, orgID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, orgID, created.ID); !isNotFound(err) {
		t.Fatalf("Get after delete: err = %v, want not_found", err)
	}
}

// fakeTXT answers TXT lookups from a map.
type fakeTXT map[string][]string

func (f fakeTXT) LookupTXT(_ context.Context, name string) ([]string, error) { return f[name], nil }

func isCode(err error, code string) bool {
	apiErr, ok := err.(*apierror.Error)
	return ok && apiErr.Code == code
}

// A pending entry blocks nobody: any organization may add a name, only the
// one that proves its DNS gets it active (tech audit T6). Before, the
// first to type a name in held it and its owner could not add it at all.
func TestOwnershipDecidesWhoGetsADomain(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	squatter, owner := seedOrg(t, ctx, pool), seedOrg(t, ctx, pool)
	dns := fakeTXT{}
	svc := newTestService(pool, nil, nil, nil, "").WithTXTResolver(dns)
	name := "shop-" + strings.ToLower(idgen.New()) + ".com"

	squatted, err := svc.Create(ctx, squatter, domain.CreateInput{Domain: name, Purpose: []string{"tracking"}})
	if err != nil {
		t.Fatalf("squatter Create: %v", err)
	}
	owned, err := svc.Create(ctx, owner, domain.CreateInput{Domain: name, Purpose: []string{"tracking"}})
	if err != nil {
		t.Fatalf("owner Create while a squatter holds it pending: %v", err)
	}
	if _, err := svc.Create(ctx, owner, domain.CreateInput{Domain: strings.ToUpper(name), Purpose: []string{"tracking"}}); !isCode(err, "conflict") {
		t.Fatalf("same org twice: err = %v, want conflict", err)
	}
	if squatted.OwnershipRecord.Value == owned.OwnershipRecord.Value || owned.OwnershipRecord.Name != "_flox-verify."+name {
		t.Fatalf("records %+v / %+v: want one name, a token per row", squatted.OwnershipRecord, owned.OwnershipRecord)
	}

	// No record yet.
	if _, err := svc.VerifyOwnership(ctx, owner, owned.ID); !isCode(err, "validation") {
		t.Fatalf("verify without a record: err = %v", err)
	}
	// The owner publishes its token; the squatter's token is not there.
	dns[owned.OwnershipRecord.Name] = []string{"v=spf1 -all", " " + owned.OwnershipRecord.Value + " "}
	if _, err := svc.VerifyOwnership(ctx, squatter, squatted.ID); !isCode(err, "validation") {
		t.Fatalf("squatter verify against the owner's record: err = %v, want refused", err)
	}
	verified, err := svc.VerifyOwnership(ctx, owner, owned.ID)
	if err != nil {
		t.Fatalf("owner verify: %v", err)
	}
	if verified.Status != domain.StatusActive || verified.OwnershipVerifiedAt == nil {
		t.Fatalf("after verify: %+v, want active and verified", verified)
	}
	// Even with its own record, the squatter cannot take an active name.
	dns[owned.OwnershipRecord.Name] = append(dns[owned.OwnershipRecord.Name], squatted.OwnershipRecord.Value)
	if _, err := svc.VerifyOwnership(ctx, squatter, squatted.ID); !isCode(err, "conflict") {
		t.Fatalf("second verified org: err = %v, want conflict", err)
	}
}

func TestActiveNeedsVerifiedOwnership(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	org := seedOrg(t, ctx, pool)
	dns := fakeTXT{}
	svc := newTestService(pool, nil, nil, nil, "").WithTXTResolver(dns)
	active := domain.StatusActive

	if _, err := svc.Create(ctx, org, domain.CreateInput{Domain: "x-" + strings.ToLower(idgen.New()) + ".com", Purpose: []string{"tracking"}, Status: active}); !isCode(err, "validation") {
		t.Fatalf("Create active: err = %v, want refused", err)
	}
	d, err := svc.Create(ctx, org, domain.CreateInput{Domain: "x-" + strings.ToLower(idgen.New()) + ".com", Purpose: []string{"tracking"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, org, d.ID, domain.UpdateInput{Status: &active}); !isCode(err, "validation") {
		t.Fatalf("PATCH active unverified: err = %v, want refused", err)
	}

	dns[d.OwnershipRecord.Name] = []string{d.OwnershipRecord.Value}
	if _, err := svc.VerifyOwnership(ctx, org, d.ID); err != nil {
		t.Fatal(err)
	}
	// Verified: it may go off and on again.
	pending := domain.StatusPending
	if _, err := svc.Update(ctx, org, d.ID, domain.UpdateInput{Status: &pending}); err != nil {
		t.Fatal(err)
	}
	if back, err := svc.Update(ctx, org, d.ID, domain.UpdateInput{Status: &active}); err != nil || back.Status != domain.StatusActive {
		t.Fatalf("re-activate verified: %v %v", back.Status, err)
	}

	// A rename is a new domain: unverified, pending, a fresh token.
	other := "y-" + strings.ToLower(idgen.New()) + ".com"
	renamed, err := svc.Update(ctx, org, d.ID, domain.UpdateInput{Domain: &other})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Status != domain.StatusPending || renamed.OwnershipVerifiedAt != nil || renamed.OwnershipRecord.Value == d.OwnershipRecord.Value {
		t.Fatalf("renamed: %+v, want pending, unverified, new token", renamed)
	}
}

// A name under a reserved TLD (.test, .localhost, …) cannot be owned or
// squatted by anyone: it is verified as soon as it is added.
func TestReservedNamesAreVerifiedAtOnce(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	org := seedOrg(t, ctx, pool)
	svc := newTestService(pool, nil, nil, nil, "")
	d, err := svc.Create(ctx, org, domain.CreateInput{Domain: "t-" + strings.ToLower(idgen.New()) + ".test", Purpose: []string{"tracking"}})
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != domain.StatusActive || d.OwnershipVerifiedAt == nil {
		t.Fatalf("reserved: %+v, want active and verified", d)
	}
	example := "e-" + strings.ToLower(idgen.New()) + ".example.com"
	if r, err := svc.Update(ctx, org, d.ID, domain.UpdateInput{Domain: &example}); err != nil || r.Status != domain.StatusPending {
		t.Fatalf("renamed to a public name: %v %v, want pending", r.Status, err)
	}
}

func TestCreateRejectsInvalidDomainAndEmptyPurpose(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := newTestService(pool, nil, nil, nil, "")

	if _, err := svc.Create(ctx, orgID, domain.CreateInput{Domain: "not a domain", Purpose: []string{"tracking"}}); err == nil {
		t.Fatal("expected an error for an invalid domain string")
	}
	if _, err := svc.Create(ctx, orgID, domain.CreateInput{Domain: "valid.example.com", Purpose: nil}); err == nil {
		t.Fatal("expected an error for empty purpose")
	}
}

func TestListReturnsOrgsOwnDomainsOnly(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgA := seedOrg(t, ctx, pool)
	orgB := seedOrg(t, ctx, pool)
	svc := newTestService(pool, nil, nil, nil, "")

	if _, err := svc.Create(ctx, orgA, domain.CreateInput{Domain: "a.example.com", Purpose: []string{"tracking"}}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Create(ctx, orgB, domain.CreateInput{Domain: "b.example.com", Purpose: []string{"tracking"}}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	domains, _, err := svc.List(ctx, orgA, pagination.Page{Limit: pagination.MaxLimit})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(domains) != 1 || domains[0].Domain != "a.example.com" {
		t.Fatalf("domains = %+v, want only org A's own", domains)
	}
}

func TestVerifyNamecheapAndCloudflare(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	creds := &fakeCredentialSource{creds: map[integrations.Provider]map[string]string{
		integrations.ProviderNamecheap:  {"apiUser": "u"},
		integrations.ProviderCloudflare: {"apiToken": "t"},
	}}
	expiresAt := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	registrar := &fakeRegistrar{info: domain.RegistrarInfo{ExpiresAt: expiresAt}}
	dns := &fakeDNS{status: domain.DNSZoneStatus{Active: true, SSLIssued: true}}
	svc := newTestService(pool, creds, registrar, dns, "")

	d, err := svc.Create(ctx, orgID, domain.CreateInput{
		Domain: "verify.example.com", Purpose: []string{"tracking"},
		Registrar: domain.RegistrarNamecheap, DNSProvider: domain.DNSProviderCloudflare,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	verified, err := svc.Verify(ctx, orgID, d.ID)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if verified.ExpiresAt == nil || !verified.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("ExpiresAt = %v, want %v (from the real Namecheap check)", verified.ExpiresAt, expiresAt)
	}
	if verified.SSL != domain.SSLIssued {
		t.Fatalf("SSL = %q, want issued (from the real Cloudflare check)", verified.SSL)
	}
	if verified.VerifiedAt == nil {
		t.Fatal("VerifiedAt is nil, want it stamped by Verify")
	}
}

func TestVerifyRequiresConnectedIntegration(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := newTestService(pool, nil, nil, nil, "") // no credentials connected

	d, err := svc.Create(ctx, orgID, domain.CreateInput{
		Domain: "noconn.example.com", Purpose: []string{"tracking"}, Registrar: domain.RegistrarNamecheap,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Verify(ctx, orgID, d.ID); err == nil {
		t.Fatal("Verify with no connected Namecheap integration succeeded, want a validation error")
	}
}

// TestVerifyRunsWhicheverHalfIsConnected is a regression test for a real
// bug found via live curl verification, not by inspection: the original
// implementation short-circuited on the FIRST applicable-but-
// unconnected provider, so a domain with registrar=namecheap
// (disconnected) and dnsProvider=cloudflare (connected) never even
// attempted the real Cloudflare check.
func TestVerifyRunsWhicheverHalfIsConnected(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	// Only Cloudflare is connected — Namecheap is not.
	creds := &fakeCredentialSource{creds: map[integrations.Provider]map[string]string{
		integrations.ProviderCloudflare: {"apiToken": "t"},
	}}
	dns := &fakeDNS{status: domain.DNSZoneStatus{Active: true, SSLIssued: true}}
	svc := newTestService(pool, creds, nil, dns, "")

	d, err := svc.Create(ctx, orgID, domain.CreateInput{
		Domain: "partial.example.com", Purpose: []string{"tracking"},
		Registrar: domain.RegistrarNamecheap, DNSProvider: domain.DNSProviderCloudflare,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	verified, err := svc.Verify(ctx, orgID, d.ID)
	if err != nil {
		t.Fatalf("Verify: %v, want it to succeed via the connected Cloudflare half despite Namecheap being disconnected", err)
	}
	if verified.SSL != domain.SSLIssued {
		t.Fatalf("SSL = %q, want issued (the Cloudflare half should have run)", verified.SSL)
	}
	if verified.ExpiresAt != nil {
		t.Fatalf("ExpiresAt = %v, want nil (the Namecheap half was correctly skipped, not attempted)", verified.ExpiresAt)
	}
}

func TestVerifyRejectsUnsupportedProviders(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	svc := newTestService(pool, nil, nil, nil, "")

	d, err := svc.Create(ctx, orgID, domain.CreateInput{
		Domain: "unmanaged.example.com", Purpose: []string{"tracking"},
		Registrar: domain.RegistrarGoDaddy, DNSProvider: domain.DNSProviderRoute53,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Verify(ctx, orgID, d.ID); err == nil {
		t.Fatal("Verify on a GoDaddy/Route53 domain (no real adapter for either) succeeded, want a validation error")
	}
}

func TestRenewRequiresNamecheapRegistrar(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	registrar := &fakeRegistrar{}
	svc := newTestService(pool, &fakeCredentialSource{creds: map[integrations.Provider]map[string]string{
		integrations.ProviderNamecheap: {"apiUser": "u"},
	}}, registrar, nil, "")

	d, err := svc.Create(ctx, orgID, domain.CreateInput{
		Domain: "godaddy.example.com", Purpose: []string{"tracking"}, Registrar: domain.RegistrarGoDaddy,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Renew(ctx, orgID, d.ID, 1); err == nil {
		t.Fatal("Renew on a GoDaddy domain succeeded, want a validation error")
	}
	if registrar.renewCalls != 0 {
		t.Fatal("the real (fake) Renew adapter was called despite the registrar mismatch — a real account would have been billed")
	}
}

func TestRenewValidatesYearsAndCallsTheRealAdapterExactlyOnce(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	newExpiry := time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)
	registrar := &fakeRegistrar{info: domain.RegistrarInfo{ExpiresAt: newExpiry}}
	svc := newTestService(pool, &fakeCredentialSource{creds: map[integrations.Provider]map[string]string{
		integrations.ProviderNamecheap: {"apiUser": "u"},
	}}, registrar, nil, "")

	d, err := svc.Create(ctx, orgID, domain.CreateInput{
		Domain: "renew.example.com", Purpose: []string{"tracking"}, Registrar: domain.RegistrarNamecheap,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.Renew(ctx, orgID, d.ID, 0); err == nil {
		t.Fatal("Renew with 0 years succeeded, want a validation error")
	}
	if registrar.renewCalls != 0 {
		t.Fatal("the real (fake) Renew adapter was called for an invalid years value — a real account would have been billed")
	}

	renewed, err := svc.Renew(ctx, orgID, d.ID, 1)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if registrar.renewCalls != 1 {
		t.Fatalf("Renew adapter called %d times, want exactly 1", registrar.renewCalls)
	}
	if renewed.ExpiresAt == nil || !renewed.ExpiresAt.Equal(newExpiry) {
		t.Fatalf("ExpiresAt after renew = %v, want %v", renewed.ExpiresAt, newExpiry)
	}
}

func TestRenewPropagatesARealRegistrarFailureWithoutCorruptingState(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	registrar := &fakeRegistrar{err: errors.New("insufficient account balance")}
	svc := newTestService(pool, &fakeCredentialSource{creds: map[integrations.Provider]map[string]string{
		integrations.ProviderNamecheap: {"apiUser": "u"},
	}}, registrar, nil, "")

	d, err := svc.Create(ctx, orgID, domain.CreateInput{
		Domain: "failedrenew.example.com", Purpose: []string{"tracking"}, Registrar: domain.RegistrarNamecheap,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Renew(ctx, orgID, d.ID, 1); err == nil {
		t.Fatal("Renew succeeded despite the real adapter reporting failure")
	}

	got, err := svc.Get(ctx, orgID, d.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ExpiresAt != nil {
		t.Fatalf("ExpiresAt = %v after a failed renewal, want nil (nothing was actually renewed)", got.ExpiresAt)
	}
}

func TestSyncDNSRequiresTrackerTargetConfigured(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	dns := &fakeDNS{}
	svc := newTestService(pool, &fakeCredentialSource{creds: map[integrations.Provider]map[string]string{
		integrations.ProviderCloudflare: {"apiToken": "t"},
	}}, nil, dns, "") // trackerDNSTarget deliberately empty

	d, err := svc.Create(ctx, orgID, domain.CreateInput{
		Domain: "nodns.example.com", Purpose: []string{"tracking"}, DNSProvider: domain.DNSProviderCloudflare,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.SyncDNS(ctx, orgID, d.ID); err == nil {
		t.Fatal("SyncDNS with no TRACKER_DNS_TARGET configured succeeded, want a validation error")
	}
	if dns.ensureCalls != 0 {
		t.Fatal("the real (fake) DNS adapter was called despite no configured target — a real customer's DNS would have been mutated with a fabricated value")
	}
}

func TestSyncDNSCallsTheRealAdapterWithTheConfiguredTarget(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	dns := &fakeDNS{}
	svc := newTestService(pool, &fakeCredentialSource{creds: map[integrations.Provider]map[string]string{
		integrations.ProviderCloudflare: {"apiToken": "t"},
	}}, nil, dns, "tracker.floxlink.io")

	d, err := svc.Create(ctx, orgID, domain.CreateInput{
		Domain: "dns.example.com", Purpose: []string{"tracking"}, DNSProvider: domain.DNSProviderCloudflare,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.SyncDNS(ctx, orgID, d.ID); err != nil {
		t.Fatalf("SyncDNS: %v", err)
	}
	if dns.ensureCalls != 1 {
		t.Fatalf("EnsureRecord called %d times, want exactly 1", dns.ensureCalls)
	}
	if dns.lastTarget != "tracker.floxlink.io" {
		t.Fatalf("EnsureRecord target = %q, want the configured tracker.floxlink.io", dns.lastTarget)
	}
}

// Marketer audit 2026-10-06, #4: SSL status comes from a real TLS
// connection, is tenant-scoped, and a failed check keeps the last expiry.
func TestCheckSSLRecordsTheProbeAndStaysInItsOrg(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	otherOrg := seedOrg(t, ctx, pool)
	host := "ssl-" + strings.ToLower(idgen.New()) + ".example.com"
	expiry := time.Now().Add(80 * 24 * time.Hour).Truncate(time.Second)
	prober := &fakeProber{results: map[string]tlsprobe.Result{host: {Outcome: tlsprobe.Issued, ExpiresAt: &expiry}}}
	svc := domain.NewService(domain.NewRepository(pool), nil, nil, nil, "", prober)

	d, err := svc.Create(ctx, orgID, domain.CreateInput{Domain: host, Purpose: []string{"tracking"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.SSL != domain.SSLNone || d.SSLCheckedAt != nil {
		t.Fatalf("new domain: ssl=%s checked=%v, want none and never checked", d.SSL, d.SSLCheckedAt)
	}

	if _, err := svc.CheckSSL(ctx, otherOrg, d.ID); !isNotFound(err) {
		t.Fatalf("CheckSSL from another org: err=%v, want not found", err)
	}
	if len(prober.hosts) != 0 {
		t.Fatalf("another org's CheckSSL probed %v", prober.hosts)
	}

	checked, err := svc.CheckSSL(ctx, orgID, d.ID)
	if err != nil {
		t.Fatalf("CheckSSL: %v", err)
	}
	if checked.SSL != domain.SSLIssued || checked.SSLExpiresAt == nil || !checked.SSLExpiresAt.Equal(expiry) || checked.SSLError != "" || checked.SSLCheckedAt == nil {
		t.Fatalf("after a valid probe: %+v, want issued expiring %s", checked, expiry)
	}

	prober.results[host] = tlsprobe.Result{Outcome: tlsprobe.None, Err: "dial tcp: connection refused"}
	failed, err := svc.CheckSSL(ctx, orgID, d.ID)
	if err != nil {
		t.Fatalf("CheckSSL: %v", err)
	}
	if failed.SSL != domain.SSLNone || failed.SSLError != "dial tcp: connection refused" || failed.SSLExpiresAt == nil || !failed.SSLExpiresAt.Equal(expiry) {
		t.Fatalf("after a failed probe: %+v, want none with the error and the last expiry kept", failed)
	}
}

func TestClaimSSLChecksPicksOnlyDueDomains(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	repo := domain.NewRepository(pool)
	svc := newTestService(pool, nil, nil, nil, "")

	mk := func(ssl string, checkedAgo time.Duration) string {
		d, err := svc.Create(ctx, orgID, domain.CreateInput{Domain: "due-" + strings.ToLower(idgen.New()) + ".example.com", Purpose: []string{"tracking"}})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if checkedAgo > 0 {
			if _, err := pool.Exec(ctx, `UPDATE domains SET ssl = $1, ssl_checked_at = now() - make_interval(secs => $2::float8) WHERE id = $3`, ssl, checkedAgo.Seconds(), d.ID); err != nil {
				t.Fatalf("aging: %v", err)
			}
		}
		return d.ID
	}
	never := mk("", 0)
	issuedRecent := mk("issued", time.Hour)
	issuedStale := mk("issued", 7*time.Hour)
	pendingRecent := mk("pending", 5*time.Minute)
	pendingStale := mk("pending", 15*time.Minute)

	claimed := func() map[string]bool {
		due, err := repo.ClaimSSLChecks(ctx, 100000, 6*time.Hour, 10*time.Minute)
		if err != nil {
			t.Fatalf("ClaimSSLChecks: %v", err)
		}
		out := map[string]bool{}
		for _, d := range due {
			if d.OrganizationID == orgID {
				out[d.ID] = true
			}
		}
		return out
	}
	got := claimed()
	for id, want := range map[string]bool{never: true, issuedRecent: false, issuedStale: true, pendingRecent: false, pendingStale: true} {
		if got[id] != want {
			t.Fatalf("domain %s claimed=%v, want %v (claimed: %v)", id, got[id], want, got)
		}
	}
	if again := claimed(); len(again) != 0 {
		t.Fatalf("second claim right away took %v, want nothing (the first claim stamps them)", again)
	}
}

func isNotFound(err error) bool {
	apiErr, ok := err.(*apierror.Error)
	return ok && apiErr.Code == "not_found"
}

func mustPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connecting to database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedOrg(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, id, "Test Org "+id)
	if err != nil {
		t.Fatalf("seeding organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, id)
	})
	return id
}

// A cloudflare_registrar domain is read through the Cloudflare
// integration's token with the Cloudflare Registrar adapter (not
// Namecheap's): expiry and auto-renew are stored, and another
// organization cannot verify it. Without the integration, Verify says
// to connect Cloudflare.
func TestVerifyCloudflareRegistrar(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	creds := &fakeCredentialSource{creds: map[integrations.Provider]map[string]string{
		integrations.ProviderCloudflare: {"apiToken": "t"},
	}}
	namecheap := &fakeRegistrar{err: errors.New("Namecheap must not be asked")}
	off := false
	expiresAt := time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)
	cf := &fakeRegistrar{info: domain.RegistrarInfo{ExpiresAt: expiresAt, AutoRenew: &off}}
	svc := newTestService(pool, creds, namecheap, nil, "").WithCloudflareRegistrar(cf)

	d, err := svc.Create(ctx, orgID, domain.CreateInput{
		Domain: "cfreg.example.com", Purpose: []string{"tracking"}, Registrar: domain.RegistrarCloudflareRegistrar,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.RegistrarAutoRenew != nil {
		t.Fatalf("auto-renew before Verify = %v, want unknown", *d.RegistrarAutoRenew)
	}
	verified, err := svc.Verify(ctx, orgID, d.ID)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if verified.ExpiresAt == nil || !verified.ExpiresAt.Equal(expiresAt) || verified.RegistrarAutoRenew == nil || *verified.RegistrarAutoRenew {
		t.Fatalf("verified = expires %v, auto-renew %v; want %v and off", verified.ExpiresAt, verified.RegistrarAutoRenew, expiresAt)
	}
	if got, err := svc.Get(ctx, orgID, d.ID); err != nil || got.RegistrarAutoRenew == nil || *got.RegistrarAutoRenew {
		t.Fatalf("stored auto-renew = %v, %v; want off", got.RegistrarAutoRenew, err)
	}

	other := seedOrg(t, ctx, pool)
	if _, err := svc.Verify(ctx, other, d.ID); err == nil {
		t.Fatal("another organization verified the domain")
	}

	unconnected := newTestService(pool, nil, nil, nil, "").WithCloudflareRegistrar(cf)
	if _, err := unconnected.Verify(ctx, orgID, d.ID); !strings.Contains(registrarField(err), "Cloudflare") {
		t.Fatalf("Verify without the Cloudflare integration = %v, want an error naming Cloudflare", err)
	}
}

// Cloudflare Registrar cannot renew through its API: Renew refuses before
// any adapter is called, and says what to do instead.
func TestRenewRefusesCloudflareRegistrar(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	namecheap, cf := &fakeRegistrar{}, &fakeRegistrar{}
	svc := newTestService(pool, &fakeCredentialSource{creds: map[integrations.Provider]map[string]string{
		integrations.ProviderCloudflare: {"apiToken": "t"},
		integrations.ProviderNamecheap:  {"apiUser": "u"},
	}}, namecheap, nil, "").WithCloudflareRegistrar(cf)

	d, err := svc.Create(ctx, orgID, domain.CreateInput{
		Domain: "cfrenew.example.com", Purpose: []string{"tracking"}, Registrar: domain.RegistrarCloudflareRegistrar,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = svc.Renew(ctx, orgID, d.ID, 1)
	if !strings.Contains(registrarField(err), "auto-renew") {
		t.Fatalf("Renew = %v, want a refusal pointing at auto-renew", err)
	}
	if namecheap.renewCalls+cf.renewCalls != 0 {
		t.Fatal("a renewal adapter was called")
	}
}

// registrarField is a validation error's "registrar" field ("" for any
// other error).
func registrarField(err error) string {
	var apiErr *apierror.Error
	if errors.As(err, &apiErr) {
		return apiErr.Fields["registrar"]
	}
	return ""
}
