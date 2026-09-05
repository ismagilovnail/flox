package integrations

import (
	"context"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// Verifier is what each provider's adapter implements — a single real
// call that proves a set of credential fields actually works against
// that platform, used both when connecting (must succeed before
// anything is ever stored) and when re-verifying an already-stored
// credential (POST .../verify).
type Verifier interface {
	Verify(ctx context.Context, fields map[string]string) error
}

// AdPlatformSource is the read side of apps/internal/adaccount this
// package depends on for its read-only ad-platform cards — a narrow
// interface (not the concrete *adaccount.Repository) so this package's
// own tests can substitute a fake without needing a real Postgres
// connection for that half of Summary.
type AdPlatformSource interface {
	AdPlatformSummary(ctx context.Context, orgID string) ([]adaccount.PlatformCount, error)
}

type Service struct {
	repo        *Repository
	key         []byte
	adapters    map[Provider]Verifier
	adPlatforms AdPlatformSource
}

func NewService(repo *Repository, key []byte, adPlatforms AdPlatformSource) *Service {
	return &Service{
		repo: repo,
		key:  key,
		adapters: map[Provider]Verifier{
			ProviderNamecheap:  newNamecheapAdapter(),
			ProviderCloudflare: newCloudflareAdapter(),
			ProviderSlack:      newSlackAdapter(),
		},
		adPlatforms: adPlatforms,
	}
}

// SetAdapter overrides the adapter used for one provider — test-only
// (this project has no live Namecheap/Cloudflare account to exercise the
// real adapters against), letting integrations_test substitute a fake
// Verifier without a package-internal test file.
func (s *Service) SetAdapter(provider Provider, v Verifier) {
	s.adapters[provider] = v
}

func toIntegration(r storedRow) Integration {
	status := StatusConnected
	if r.LastError != nil {
		status = StatusError
	}
	verifiedAt := r.LastVerifiedAt
	return Integration{Provider: r.Provider, Status: status, LastVerifiedAt: &verifiedAt, LastError: r.LastError}
}

// Summary returns every provider's status in one call — the three
// credential-based integrations always appear (StatusNotConnected when
// no row exists yet, matching the frontend's own "always show all
// providers" panel shape) plus the real ad-platform aggregate.
func (s *Service) Summary(ctx context.Context, orgID string) (Summary, error) {
	rows, err := s.repo.List(ctx, orgID)
	if err != nil {
		return Summary{}, err
	}
	byProvider := make(map[Provider]storedRow, len(rows))
	for _, row := range rows {
		byProvider[row.Provider] = row
	}

	all := []Provider{ProviderNamecheap, ProviderCloudflare, ProviderSlack}
	integrations := make([]Integration, 0, len(all))
	for _, p := range all {
		if row, ok := byProvider[p]; ok {
			integrations = append(integrations, toIntegration(row))
			continue
		}
		integrations = append(integrations, Integration{Provider: p, Status: StatusNotConnected})
	}

	platforms, err := s.adPlatforms.AdPlatformSummary(ctx, orgID)
	if err != nil {
		return Summary{}, err
	}
	adPlatforms := make([]AdPlatformStatus, len(platforms))
	for i, p := range platforms {
		adPlatforms[i] = AdPlatformStatus{Provider: p.CostIntegration, ConnectedCount: p.ConnectedCount, TotalCount: p.TotalCount}
	}

	return Summary{Integrations: integrations, AdPlatforms: adPlatforms}, nil
}

// Connect validates every field this provider requires is present, then
// requires the adapter's own real Verify call to succeed BEFORE anything
// is ever written — unlike RecordVerification (re-checking an already-
// stored credential), a brand-new connect attempt that fails leaves no
// row at all, matching the mock's own "Connect" action having no partial/
// broken intermediate state.
func (s *Service) Connect(ctx context.Context, orgID string, in ConnectInput) (Integration, error) {
	if !in.Provider.Valid() {
		return Integration{}, apierror.Validation("invalid integration provider", nil)
	}

	fieldErrs := map[string]string{}
	cleaned := make(map[string]string, len(in.Fields))
	for _, name := range in.Provider.RequiredFields() {
		v := strings.TrimSpace(in.Fields[name])
		if v == "" || len(v) > 500 {
			fieldErrs[name] = "required, must be at most 500 characters"
			continue
		}
		cleaned[name] = v
	}
	if len(fieldErrs) > 0 {
		return Integration{}, apierror.Validation("invalid integration credentials", fieldErrs)
	}

	adapter := s.adapters[in.Provider]
	if err := adapter.Verify(ctx, cleaned); err != nil {
		return Integration{}, apierror.Validation("could not verify credentials: "+err.Error(), nil)
	}

	payload, err := encodePayload(cleaned)
	if err != nil {
		return Integration{}, err
	}
	encrypted, err := encrypt(s.key, payload)
	if err != nil {
		return Integration{}, err
	}
	if err := s.repo.Upsert(ctx, idgen.New(), orgID, in.Provider, encrypted); err != nil {
		return Integration{}, err
	}

	return Integration{Provider: in.Provider, Status: StatusConnected}, nil
}

// Verify re-runs the real check against an already-stored credential —
// the "Verify connection" action, distinct from Connect: it never
// deletes the row on failure, it records the failure as an "error"
// status so the operator can see why without having to reconnect from
// scratch (they may not even have the credential to hand right now).
func (s *Service) Verify(ctx context.Context, orgID string, provider Provider) (Integration, error) {
	if !provider.Valid() {
		return Integration{}, apierror.Validation("invalid integration provider", nil)
	}
	encrypted, found, err := s.repo.PayloadByProvider(ctx, orgID, provider)
	if err != nil {
		return Integration{}, err
	}
	if !found {
		return Integration{}, apierror.NotFound("integration not connected")
	}
	payload, err := decrypt(s.key, encrypted)
	if err != nil {
		return Integration{}, err
	}
	fields, err := decodePayload(payload)
	if err != nil {
		return Integration{}, err
	}

	verifyErr := s.adapters[provider].Verify(ctx, fields)
	if err := s.repo.RecordVerification(ctx, orgID, provider, verifyErr); err != nil {
		return Integration{}, err
	}

	status := StatusConnected
	var lastErr *string
	if verifyErr != nil {
		status = StatusError
		msg := verifyErr.Error()
		lastErr = &msg
	}
	return Integration{Provider: provider, Status: status, LastError: lastErr}, nil
}

func (s *Service) Disconnect(ctx context.Context, orgID string, provider Provider) error {
	if !provider.Valid() {
		return apierror.Validation("invalid integration provider", nil)
	}
	return s.repo.Delete(ctx, orgID, provider)
}
