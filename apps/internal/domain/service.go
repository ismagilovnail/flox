package domain

import (
	"context"
	"regexp"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
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
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, orgID string) ([]Domain, error) {
	return s.repo.List(ctx, orgID)
}

func (s *Service) Get(ctx context.Context, orgID, id string) (Domain, error) {
	return s.repo.GetByID(ctx, orgID, id)
}

func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (Domain, error) {
	in.Domain = strings.ToLower(strings.TrimSpace(in.Domain))
	if in.Status == "" {
		in.Status = StatusActive
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
	return s.repo.Create(ctx, idgen.New(), orgID, in)
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

	return s.repo.Update(ctx, orgID, id, in)
}

func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	return s.repo.Delete(ctx, orgID, id)
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
