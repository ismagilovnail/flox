// Package apikey implements the Settings > API keys panel (§30) — real
// CRUD over the api_keys table (migration 00010).
// Same one-way-hash-at-rest precedent as sessions/invite tokens
// (apps/internal/auth/crypto.go's own doc comment): only key_hash is
// ever persisted, the full key exists only in the create response, shown
// once.
package apikey

import "time"

type Scope string

const (
	ScopeRead  Scope = "read"
	ScopeWrite Scope = "write"
	ScopeAdmin Scope = "admin"
)

func (s Scope) valid() bool {
	switch s {
	case ScopeRead, ScopeWrite, ScopeAdmin:
		return true
	}
	return false
}

type ApiKey struct {
	ID              string     `json:"id"`
	OrganizationID  string     `json:"organizationId"`
	Name            string     `json:"name"`
	Prefix          string     `json:"prefix"`
	Scope           Scope      `json:"scope"`
	CreatedByUserID string     `json:"createdByUserId"`
	CreatedAt       time.Time  `json:"createdAt"`
	LastUsedAt      *time.Time `json:"lastUsedAt"`
	RevokedAt       *time.Time `json:"-"`
	// ExpiresAt is set on a rotated key: it keeps authenticating until
	// then (the grace period), then stops. ReplacedByID is its successor.
	ExpiresAt    *time.Time `json:"expiresAt"`
	ReplacedByID *string    `json:"replacedById"`
}

// usable reports whether the key authenticates at now.
func (k ApiKey) usable(now time.Time) bool {
	return k.RevokedAt == nil && (k.ExpiresAt == nil || now.Before(*k.ExpiresAt))
}

type CreateInput struct {
	Name  string
	Scope Scope
}

// CreateResult carries the full plaintext key alongside the stored
// record — the one and only time it's ever available after this call
// returns, matching the panel's own "shown once" UX.
type CreateResult struct {
	ApiKey  ApiKey
	FullKey string
}
