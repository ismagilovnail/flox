package domain

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/domain/tlsprobe"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/integrations"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
)

// domainPattern matches a bare hostname (labels separated by dots, a
// 2+-letter TLD) — the same shape apps/web's domainFormSchema already
// validated client-side; re-checked here since the server never trusts
// client-side validation alone. No scheme, no path, no port: a domain row
// is a Host-header value the tracker matches literally
// (apps/internal/routingstore.ResolveTrackingLink does `lower(d.domain) =
// lower($1)` against r.Host), not a URL.
var domainPattern = regexp.MustCompile(`^([a-z0-9-]+\.)+[a-z]{2,}$`)

type Service struct {
	repo             *Repository
	credentials      CredentialSource
	registrar        RegistrarProvider
	dnsProvider      DNSProviderAdapter
	trackerDNSTarget string
	prober           SSLProber
	txt              TXTResolver
}

// TXTResolver looks up a name's TXT records — the system resolver in
// production (NetTXTResolver), a fake in tests. Behind an interface like
// every other provider (CLAUDE.md #11).
type TXTResolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// NetTXTResolver is the system DNS resolver. A name with no records is an
// empty answer, not an error.
type NetTXTResolver struct{}

func (NetTXTResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	recs, err := net.DefaultResolver.LookupTXT(ctx, name)
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return nil, nil
	}
	return recs, err
}

// WithTXTResolver replaces the DNS resolver ownership checks use.
func (s *Service) WithTXTResolver(r TXTResolver) *Service {
	s.txt = r
	return s
}

// reservedTLDs can never be registered by anyone (RFC 2606, RFC 6761), so
// nobody can prove — or squat — them: a domain under one is verified as
// soon as it is added. Dev and test stacks live on them (track.localhost).
var reservedTLDs = map[string]bool{"localhost": true, "test": true, "example": true, "invalid": true}

func reservedName(domainName string) bool {
	i := strings.LastIndex(domainName, ".")
	return reservedTLDs[domainName[i+1:]]
}

// errNeedsOwnership refuses activating a domain whose DNS ownership is not
// verified.
func errNeedsOwnership() error {
	return apierror.Validation("invalid domain", map[string]string{
		"status": "a domain becomes active once you verify you control its DNS (add the TXT record, then Verify ownership)",
	})
}

// SSLProber checks the certificate a domain actually serves —
// tlsprobe.Prober in production.
type SSLProber interface {
	Probe(ctx context.Context, host string) tlsprobe.Result
}

// NewService wires the automation dependencies alongside the repository
// — registrar/dnsProvider are always this package's own namecheap/
// cloudflare adapters in production (apps/api/main.go), swappable here
// purely so this package's own tests can substitute a fake without a
// real Namecheap/Cloudflare account. trackerDNSTarget is config.
// TrackerDNSTarget — see its own doc comment for why an empty value is
// refused rather than defaulted.
func NewService(repo *Repository, credentials CredentialSource, registrar RegistrarProvider, dnsProvider DNSProviderAdapter, trackerDNSTarget string, prober SSLProber) *Service {
	return &Service{repo: repo, credentials: credentials, registrar: registrar, dnsProvider: dnsProvider, trackerDNSTarget: trackerDNSTarget, prober: prober, txt: NetTXTResolver{}}
}

// List returns one page of the organization's domains and their total.
func (s *Service) List(ctx context.Context, orgID string, page pagination.Page) ([]Domain, int, error) {
	items, err := s.repo.List(ctx, orgID, page)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.Count(ctx, orgID)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *Service) Get(ctx context.Context, orgID, id string) (Domain, error) {
	return s.repo.GetByID(ctx, orgID, id)
}

func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (Domain, error) {
	in.Domain = strings.ToLower(strings.TrimSpace(in.Domain))
	// A new domain is pending until its organization proves it controls
	// the DNS (VerifyOwnership, tech audit T6) — except a reserved name,
	// which nobody can own.
	reserved := in.Domain != "" && reservedName(in.Domain)
	if in.Status == "" {
		in.Status = StatusPending
		if reserved {
			in.Status = StatusActive
		}
	}
	if in.Status == StatusActive && !reserved {
		return Domain{}, errNeedsOwnership()
	}
	if in.Registrar == "" {
		in.Registrar = RegistrarUnmanaged
	}
	if in.DNSProvider == "" {
		in.DNSProvider = DNSProviderUnmanaged
	}
	if fields := validate(in.Domain, in.Purpose, in.Registrar, in.DNSProvider, in.Status); len(fields) > 0 {
		return Domain{}, apierror.Validation("invalid domain", fields)
	}
	var verifiedAt *time.Time
	if reserved {
		now := time.Now()
		verifiedAt = &now
	}
	return s.repo.Create(ctx, idgen.New(), orgID, in, verifiedAt)
}

func (s *Service) Update(ctx context.Context, orgID, id string, in UpdateInput) (Domain, error) {
	fields := map[string]string{}

	if in.Domain != nil {
		trimmed := strings.ToLower(strings.TrimSpace(*in.Domain))
		in.Domain = &trimmed
		if !domainPattern.MatchString(trimmed) {
			fields["domain"] = "must be a valid domain, e.g. track.example.com"
		}
	}
	if in.Purpose != nil {
		if msg := validatePurpose(*in.Purpose); msg != "" {
			fields["purpose"] = msg
		}
	}
	if in.Registrar != nil && !in.Registrar.Valid() {
		fields["registrar"] = "must be one of namecheap, godaddy, cloudflare_registrar, unmanaged"
	}
	if in.DNSProvider != nil && !in.DNSProvider.Valid() {
		fields["dnsProvider"] = "must be one of cloudflare, route53, unmanaged"
	}
	if in.Status != nil && !in.Status.Valid() {
		fields["status"] = "must be one of active, pending, error, expired"
	}
	if len(fields) > 0 {
		return Domain{}, apierror.Validation("invalid domain", fields)
	}

	cur, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Domain{}, err
	}
	verified := cur.OwnershipVerifiedAt != nil
	// A new name is a new domain: proving the old one says nothing about
	// it. Without this a verified domain could be renamed into someone
	// else's and track on it at once.
	var reset *ownershipReset
	if in.Domain != nil && *in.Domain != cur.Domain {
		verified = reservedName(*in.Domain)
		reset = &ownershipReset{}
		if verified {
			now := time.Now()
			reset.verifiedAt = &now
		} else if in.Status == nil && cur.Status == StatusActive {
			pending := StatusPending
			in.Status = &pending
		}
	}
	if in.Status != nil && *in.Status == StatusActive && !verified {
		return Domain{}, errNeedsOwnership()
	}
	return s.repo.Update(ctx, orgID, id, in, reset)
}

func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	return s.repo.Delete(ctx, orgID, id)
}

// ownershipLookupTimeout bounds the DNS lookup of an ownership check.
const ownershipLookupTimeout = 10 * time.Second

// VerifyOwnership checks the domain's TXT record (OwnershipRecord) and,
// when it carries this domain's token, marks it verified and active (tech
// audit T6). Only whoever controls the domain's DNS can create that
// record, and the token is per domain row, so another organization's
// record for the same name proves nothing here. A domain already active
// in another organization stays there: 409.
func (s *Service) VerifyOwnership(ctx context.Context, orgID, id string) (Domain, error) {
	d, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Domain{}, err
	}
	lookupCtx, cancel := context.WithTimeout(ctx, ownershipLookupTimeout)
	defer cancel()
	records, err := s.txt.LookupTXT(lookupCtx, d.OwnershipRecord.Name)
	if err != nil {
		return Domain{}, apierror.Validation("could not look up "+d.OwnershipRecord.Name+": "+err.Error()+" — try again in a minute", nil)
	}
	for _, r := range records {
		if strings.TrimSpace(r) == d.OwnershipRecord.Value {
			return s.repo.MarkOwnershipVerified(ctx, orgID, id, time.Now())
		}
	}
	msg := "no TXT record " + d.OwnershipRecord.Name + " yet"
	if len(records) > 0 {
		msg = d.OwnershipRecord.Name + " has TXT records, but not " + d.OwnershipRecord.Value
	}
	return Domain{}, apierror.Validation(msg+" — DNS changes can take a few minutes to appear", map[string]string{"ownership": "not verified"})
}

// Verify checks the domain against whichever real provider(s) it
// actually names — Namecheap for its real expiry date, Cloudflare for
// its real DNS-zone/TLS status — and persists whatever it finds.
// Read-only against both providers: no charge, no DNS write. The two
// checks are independent: a domain can have registrar=namecheap
// disconnected but dnsProvider=cloudflare connected (or vice versa), and
// Verify still runs whichever half it actually can rather than failing
// the whole call because the OTHER half isn't set up yet — the
// not-connected error for one half is only surfaced if it's the only
// applicable check, or if it isn't just skipped and both applicable
// checks come back unconnected. A real API failure on a check that IS
// connected, by contrast, always stops and surfaces immediately — that's
// a genuine error, not a "not configured yet" state to skip past.
func (s *Service) Verify(ctx context.Context, orgID, id string) (Domain, error) {
	d, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Domain{}, err
	}

	var res AutomationResult
	checked := false
	var notConnected error

	if d.Registrar == RegistrarNamecheap {
		creds, found, err := s.credentials.CredentialsFor(ctx, orgID, integrations.ProviderNamecheap)
		if err != nil {
			return Domain{}, err
		}
		if !found {
			notConnected = apierror.Validation("cannot verify", map[string]string{"registrar": "connect Namecheap in Settings > Integrations first"})
		} else {
			info, err := s.registrar.DomainInfo(ctx, creds, d.Domain)
			if err != nil {
				return Domain{}, apierror.Validation("could not verify with Namecheap: "+err.Error(), nil)
			}
			expiresAt := info.ExpiresAt
			res.ExpiresAt = &expiresAt
			checked = true
		}
	}

	if d.DNSProvider == DNSProviderCloudflare {
		creds, found, err := s.credentials.CredentialsFor(ctx, orgID, integrations.ProviderCloudflare)
		if err != nil {
			return Domain{}, err
		}
		if !found {
			notConnected = apierror.Validation("cannot verify", map[string]string{"dnsProvider": "connect Cloudflare in Settings > Integrations first"})
		} else {
			zone, err := s.dnsProvider.ZoneStatus(ctx, creds, d.Domain)
			if err != nil {
				return Domain{}, apierror.Validation("could not verify with Cloudflare: "+err.Error(), nil)
			}
			ssl := SSLNone
			switch {
			case zone.SSLIssued:
				ssl = SSLIssued
			case zone.Active:
				ssl = SSLPending
			}
			res.SSL = &ssl
			checked = true
		}
	}

	if !checked {
		if notConnected != nil {
			return Domain{}, notConnected
		}
		return Domain{}, apierror.Validation("cannot verify", map[string]string{
			"registrar": "must be namecheap and/or dnsProvider must be cloudflare, with the matching integration connected in Settings",
		})
	}

	now := time.Now()
	res.VerifiedAt = &now
	return s.repo.ApplyAutomationResult(ctx, orgID, id, res)
}

// Renew calls the real Namecheap domains.renew command — a genuine,
// billable action against the org's connected Namecheap account. A nil
// error means Namecheap actually charged and extended the registration;
// this is never a dry run.
func (s *Service) Renew(ctx context.Context, orgID, id string, years int) (Domain, error) {
	if years < 1 || years > 10 {
		return Domain{}, apierror.Validation("invalid renewal", map[string]string{"years": "must be between 1 and 10"})
	}
	d, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Domain{}, err
	}
	if d.Registrar != RegistrarNamecheap {
		return Domain{}, apierror.Validation("cannot renew", map[string]string{"registrar": "must be namecheap"})
	}
	creds, found, err := s.credentials.CredentialsFor(ctx, orgID, integrations.ProviderNamecheap)
	if err != nil {
		return Domain{}, err
	}
	if !found {
		return Domain{}, apierror.Validation("cannot renew", map[string]string{"registrar": "connect Namecheap in Settings > Integrations first"})
	}

	info, err := s.registrar.Renew(ctx, creds, d.Domain, years)
	if err != nil {
		return Domain{}, apierror.Validation("renewal failed: "+err.Error(), nil)
	}

	now := time.Now()
	expiresAt := info.ExpiresAt
	return s.repo.ApplyAutomationResult(ctx, orgID, id, AutomationResult{ExpiresAt: &expiresAt, VerifiedAt: &now})
}

// SyncDNS creates or updates a real Cloudflare DNS record pointing this
// domain at this deployment's own tracker — a genuine, live-DNS-
// mutating action against the org's connected Cloudflare account.
// Refuses outright (rather than proceeding with a fabricated value) if
// TRACKER_DNS_TARGET isn't configured for this deployment.
func (s *Service) SyncDNS(ctx context.Context, orgID, id string) (Domain, error) {
	if s.trackerDNSTarget == "" {
		return Domain{}, apierror.Validation("cannot sync DNS", map[string]string{"target": "this deployment has no TRACKER_DNS_TARGET configured"})
	}
	d, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Domain{}, err
	}
	if d.DNSProvider != DNSProviderCloudflare {
		return Domain{}, apierror.Validation("cannot sync DNS", map[string]string{"dnsProvider": "must be cloudflare"})
	}
	creds, found, err := s.credentials.CredentialsFor(ctx, orgID, integrations.ProviderCloudflare)
	if err != nil {
		return Domain{}, err
	}
	if !found {
		return Domain{}, apierror.Validation("cannot sync DNS", map[string]string{"dnsProvider": "connect Cloudflare in Settings > Integrations first"})
	}

	if err := s.dnsProvider.EnsureRecord(ctx, creds, d.Domain, s.trackerDNSTarget); err != nil {
		return Domain{}, apierror.Validation("DNS sync failed: "+err.Error(), nil)
	}

	now := time.Now()
	return s.repo.ApplyAutomationResult(ctx, orgID, id, AutomationResult{VerifiedAt: &now})
}

func validate(domainName string, purpose []string, registrar Registrar, dnsProvider DNSProvider, status Status) map[string]string {
	fields := map[string]string{}
	if !domainPattern.MatchString(domainName) {
		fields["domain"] = "must be a valid domain, e.g. track.example.com"
	}
	if msg := validatePurpose(purpose); msg != "" {
		fields["purpose"] = msg
	}
	if !registrar.Valid() {
		fields["registrar"] = "must be one of namecheap, godaddy, cloudflare_registrar, unmanaged"
	}
	if !dnsProvider.Valid() {
		fields["dnsProvider"] = "must be one of cloudflare, route53, unmanaged"
	}
	if !status.Valid() {
		fields["status"] = "must be one of active, pending, error, expired"
	}
	return fields
}

func validatePurpose(purpose []string) string {
	if len(purpose) == 0 {
		return "select at least one purpose"
	}
	for _, p := range purpose {
		if !Purpose(p).Valid() {
			return "must be one of tracking, pwa, fallback"
		}
	}
	return ""
}

// CheckSSL connects to the domain over TLS now and records what
// certificate it serves — the same check the worker runs on a schedule
// (internal/sslcheck), for an operator who just pointed DNS and does not
// want to wait for it.
func (s *Service) CheckSSL(ctx context.Context, orgID, id string) (Domain, error) {
	d, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return Domain{}, err
	}
	return s.repo.ApplySSLCheck(ctx, orgID, id, ProbeSSL(ctx, s.prober, d.Domain))
}

// ProbeSSL runs one probe and maps it onto the domain's SSL fields.
func ProbeSSL(ctx context.Context, prober SSLProber, host string) SSLCheckResult {
	res := prober.Probe(ctx, host)
	return SSLCheckResult{SSL: SSL(res.Outcome), ExpiresAt: res.ExpiresAt, Err: res.Err, CheckedAt: time.Now()}
}
