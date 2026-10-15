package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/domain"
)

// Registrar implements domain.RegistrarProvider against Cloudflare
// Registrar (docs/domains-tracking-links.md, "Cloudflare Registrar"),
// with the same API token as the DNS adapter. It reads a registration —
// expiry, auto-renew — and cannot renew: Cloudflare's API offers no
// renewal ("Renewals are not yet available through the API"); Cloudflare
// renews a domain itself when its auto-renew is on.
//
// Like the DNS adapter, verified against a fake server matching the
// documented shapes, not against the real API (no account here). The
// Registrar API is in beta at Cloudflare: its fields may change.
type Registrar struct {
	Adapter
}

func NewRegistrar() *Registrar {
	return &Registrar{Adapter: *New()}
}

var _ domain.RegistrarProvider = (*Registrar)(nil)

// ErrRenewUnsupported is Renew's answer: Cloudflare cannot be asked to.
var ErrRenewUnsupported = errors.New("cloudflare: Cloudflare Registrar does not offer renewal through its API — turn on auto-renew for the domain in the Cloudflare dashboard")

// errNotInAccount: the account answered, but holds no such registration.
var errNotInAccount = errors.New("not in this account")

type accountsResponse struct {
	envelope
	Result []struct {
		ID string `json:"id"`
	} `json:"result"`
}

// registration is the part of a registration FLOX reads: the beta
// GET .../registrar/registrations/{domain} and, failing that, the older
// GET .../registrar/domains/{domain} (which has no auto_renew).
type registration struct {
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at"`
	AutoRenew *bool      `json:"auto_renew"`
}

type registrationResponse struct {
	envelope
	Result registration `json:"result"`
}

// DomainInfo finds the registration of domainName's apex in the accounts
// the token can see and returns its expiry and auto-renew. A token
// without Registrar read access, or no account holding the domain, is an
// error saying so.
func (r *Registrar) DomainInfo(ctx context.Context, fields map[string]string, domainName string) (domain.RegistrarInfo, error) {
	apex := apexDomain(domainName)
	accounts, err := r.accounts(ctx, fields)
	if err != nil {
		return domain.RegistrarInfo{}, err
	}
	if len(accounts) == 0 {
		return domain.RegistrarInfo{}, errors.New("cloudflare: the API token sees no account (it needs Account › Registrar: Read)")
	}
	for _, account := range accounts {
		reg, err := r.registration(ctx, fields, account, apex)
		if errors.Is(err, errNotInAccount) {
			continue
		}
		if err != nil {
			return domain.RegistrarInfo{}, err
		}
		if reg.ExpiresAt == nil {
			return domain.RegistrarInfo{}, fmt.Errorf("cloudflare: %s has no expiry yet (registration status %q)", apex, reg.Status)
		}
		return domain.RegistrarInfo{ExpiresAt: *reg.ExpiresAt, AutoRenew: reg.AutoRenew}, nil
	}
	return domain.RegistrarInfo{}, fmt.Errorf("cloudflare: %s is not registered with Cloudflare Registrar in any account this API token can see", apex)
}

// Renew always fails: see ErrRenewUnsupported.
func (r *Registrar) Renew(context.Context, map[string]string, string, int) (domain.RegistrarInfo, error) {
	return domain.RegistrarInfo{}, ErrRenewUnsupported
}

func (r *Registrar) accounts(ctx context.Context, fields map[string]string) ([]string, error) {
	resp, err := r.do(ctx, fields, http.MethodGet, "/accounts?per_page=50", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errors.New("cloudflare: the API token cannot list accounts (it needs Account › Registrar: Read)")
	}
	var parsed accountsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("cloudflare: parsing accounts response: %w", err)
	}
	if err := parsed.asError("listing accounts"); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(parsed.Result))
	for _, a := range parsed.Result {
		ids = append(ids, a.ID)
	}
	return ids, nil
}

// registration reads apex in account: the beta registrations endpoint,
// then the older domains one when the first does not know it (404).
func (r *Registrar) registration(ctx context.Context, fields map[string]string, account, apex string) (registration, error) {
	base := "/accounts/" + url.PathEscape(account) + "/registrar/"
	reg, err := r.readRegistration(ctx, fields, base+"registrations/"+url.PathEscape(apex))
	if !errors.Is(err, errNotInAccount) {
		return reg, err
	}
	return r.readRegistration(ctx, fields, base+"domains/"+url.PathEscape(apex))
}

func (r *Registrar) readRegistration(ctx context.Context, fields map[string]string, path string) (registration, error) {
	resp, err := r.do(ctx, fields, http.MethodGet, path, nil)
	if err != nil {
		return registration{}, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound:
		return registration{}, errNotInAccount
	case http.StatusUnauthorized, http.StatusForbidden:
		return registration{}, errors.New("cloudflare: the API token cannot read registrations (it needs Account › Registrar: Read)")
	}
	var parsed registrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return registration{}, fmt.Errorf("cloudflare: parsing registration response: %w", err)
	}
	if err := parsed.asError("reading the registration"); err != nil {
		return registration{}, err
	}
	return parsed.Result, nil
}
