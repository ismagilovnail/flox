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
// (§30's full "real module, not a text field" vision) are NOT implemented
// here — no registrar/DNS-provider adapter exists anywhere in this repo
// yet, unlike e.g. apps/internal/adaccount/facebookads's structurally-real-
// but-uncredentialed pattern. Status/SSL are plain operator-set fields, the
// same honest "you tell the system what's true" shape campaign.Status
// already has, not a fake "Verify"/"Issue SSL" button pretending to check
// anything. ExpiresAt/VerifiedAt stay server-computed (nil until a real
// registrar/CA integration exists to set them) rather than accepting
// client input for facts nothing here actually verifies.
package domain

import "time"

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
	CreatedAt      time.Time   `json:"createdAt"`
	UpdatedAt      time.Time   `json:"updatedAt"`
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
