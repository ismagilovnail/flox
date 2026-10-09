// Package domain implements the Domains API (§30's own-domain-parking
// slice): handler → service → repository, matching CLAUDE.md's Go
// architecture. Replaces apps/web's lib/mock/domains.ts + stores/domains.ts
// — the domains/tracking_links tables (migration 00007) have existed since
// early on and apps/tracker's own hot path (apps/internal/routingstore.
// ResolveTrackingLink) already reads them for real; this package is the
// missing CRUD layer that lets an operator actually populate them instead
// of only a seed script (loadtestseed) or a test being able to.
//
// Registrar connection, DNS record management, and automatic SSL issuance
// (§30's full "real module, not a text field" vision) are real for the
// two providers with a real credential to use: Namecheap (registrar) and
// Cloudflare (DNS) — both connected at the org level via apps/internal/
// integrations' Settings > Integrations credentials, decrypted here
// through the CredentialSource interface (never a second copy of them).
// GoDaddy/Route53/cloudflare_registrar/unmanaged have no adapter and stay
// plain operator-declared, same as every registrar/provider did before
// this phase. SSL/ExpiresAt/VerifiedAt are never client input — they
// change only through a real Verify/Renew/SyncDNS call
// (Repository.ApplyAutomationResult). Status is client input except
// 'active', which needs the domain's DNS ownership verified first
// (Service.VerifyOwnership, tech audit T6): a domain is tracked only
// while active.
package domain

import (
	"context"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/integrations"
)

// CredentialSource is the read side of apps/internal/integrations this
// package depends on for real Namecheap/Cloudflare credentials — a
// narrow interface (not the concrete *integrations.Service) so this
// package's own tests can substitute a fake without needing a real
// Postgres connection or encryption key for that half of Verify/Renew/
// SyncDNS.
type CredentialSource interface {
	CredentialsFor(ctx context.Context, orgID string, provider integrations.Provider) (fields map[string]string, found bool, err error)
}

// RegistrarProvider is what a real registrar adapter (this package's own
// namecheap subpackage) implements — one shape so Service.Verify/Renew
// never needs to special-case a specific registrar beyond picking which
// adapter to call.
type RegistrarProvider interface {
	// DomainInfo returns the domain's real current expiry date.
	DomainInfo(ctx context.Context, creds map[string]string, domainName string) (RegistrarInfo, error)
	// Renew is a real, billable action against the registrar account —
	// callers must treat a nil error as money having actually been
	// charged, not a dry run.
	Renew(ctx context.Context, creds map[string]string, domainName string, years int) (RegistrarInfo, error)
}

type RegistrarInfo struct {
	ExpiresAt time.Time
}

// DNSProviderAdapter is what a real DNS adapter (this package's own
// cloudflare subpackage) implements.
type DNSProviderAdapter interface {
	// ZoneStatus reports whether the domain's DNS zone is active at this
	// provider and whether a real TLS certificate is currently issued
	// for it.
	ZoneStatus(ctx context.Context, creds map[string]string, domainName string) (DNSZoneStatus, error)
	// EnsureRecord creates or updates a CNAME record pointing domainName
	// at target — a real, live-DNS-mutating action.
	EnsureRecord(ctx context.Context, creds map[string]string, domainName, target string) error
}

type DNSZoneStatus struct {
	Active    bool
	SSLIssued bool
}

type Status string

const (
	StatusActive  Status = "active"
	StatusPending Status = "pending"
	StatusError   Status = "error"
	StatusExpired Status = "expired"
)

func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusPending, StatusError, StatusExpired:
		return true
	default:
		return false
	}
}

type SSL string

const (
	SSLIssued  SSL = "issued"
	SSLPending SSL = "pending"
	SSLNone    SSL = "none"
	SSLError   SSL = "error"
)

type Purpose string

const (
	PurposeTracking Purpose = "tracking"
	PurposePWA      Purpose = "pwa"
	PurposeFallback Purpose = "fallback"
)

func (p Purpose) Valid() bool {
	switch p {
	case PurposeTracking, PurposePWA, PurposeFallback:
		return true
	default:
		return false
	}
}

type Registrar string

const (
	RegistrarNamecheap           Registrar = "namecheap"
	RegistrarGoDaddy             Registrar = "godaddy"
	RegistrarCloudflareRegistrar Registrar = "cloudflare_registrar"
	RegistrarUnmanaged           Registrar = "unmanaged"
)

func (r Registrar) Valid() bool {
	switch r {
	case RegistrarNamecheap, RegistrarGoDaddy, RegistrarCloudflareRegistrar, RegistrarUnmanaged:
		return true
	default:
		return false
	}
}

type DNSProvider string

const (
	DNSProviderCloudflare DNSProvider = "cloudflare"
	DNSProviderRoute53    DNSProvider = "route53"
	DNSProviderUnmanaged  DNSProvider = "unmanaged"
)

func (d DNSProvider) Valid() bool {
	switch d {
	case DNSProviderCloudflare, DNSProviderRoute53, DNSProviderUnmanaged:
		return true
	default:
		return false
	}
}

type Domain struct {
	ID             string      `json:"id"`
	OrganizationID string      `json:"organizationId"`
	Domain         string      `json:"domain"`
	Status         Status      `json:"status"`
	SSL            SSL         `json:"ssl"`
	Purpose        []string    `json:"purpose"`
	Registrar      Registrar   `json:"registrar"`
	DNSProvider    DNSProvider `json:"dnsProvider"`
	ExpiresAt      *time.Time  `json:"expiresAt"`
	VerifiedAt     *time.Time  `json:"verifiedAt"`
	// SSLExpiresAt is the expiry of the certificate the domain served on
	// the last successful TLS check; SSLError says why the last check
	// did not find a valid one. Both come from a real connection to the
	// domain (internal/sslcheck), never from client input.
	SSLExpiresAt *time.Time `json:"sslExpiresAt"`
	SSLCheckedAt *time.Time `json:"sslCheckedAt"`
	SSLError     string     `json:"sslError"`
	// OwnershipVerifiedAt is when the organization proved it controls the
	// domain's DNS (Service.VerifyOwnership); nil until then. Only a
	// verified domain can be active, and only an active one is tracked
	// (tech audit T6).
	OwnershipVerifiedAt *time.Time `json:"ownershipVerifiedAt"`
	// OwnershipRecord is the TXT record that proves it.
	OwnershipRecord OwnershipRecord `json:"ownershipRecord"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// OwnershipRecord is the DNS TXT record an organization adds to prove it
// controls a domain: Name holds Value.
type OwnershipRecord struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ownershipRecordName is where the TXT record lives: under the domain
// itself, so only whoever controls the domain's DNS can create it.
const ownershipRecordName = "_flox-verify."

func ownershipRecord(domainName, token string) OwnershipRecord {
	return OwnershipRecord{Name: ownershipRecordName + domainName, Value: "flox-verify=" + token}
}

type CreateInput struct {
	Domain      string
	Purpose     []string
	Registrar   Registrar
	DNSProvider DNSProvider
	Status      Status
}

// UpdateInput fields are pointers so PATCH can distinguish "not sent" from
// "sent empty" — matching trafficsource.UpdateInput. SSL/ExpiresAt/
// VerifiedAt are deliberately absent: nothing in this package computes or
// verifies them yet (see the package doc), so there is nothing for a PATCH
// to legitimately change them to.
type UpdateInput struct {
	Domain      *string
	Purpose     *[]string
	Registrar   *Registrar
	DNSProvider *DNSProvider
	Status      *Status
}
