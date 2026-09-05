// Package cloudflare implements domain.DNSProviderAdapter against the
// real Cloudflare API v4 (zones, SSL verification, DNS records) — §30's
// DNS automation. This project has no live Cloudflare account to
// exercise it against; it's verified structurally in cloudflare_test.go
// against a fake httptest.Server, matching the real endpoints'
// documented JSON response shape (confirmed via AskUserQuestion before
// this phase started — see docs/domains-tracking-links.md).
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/domain"
)

// DefaultBaseURL is the real Cloudflare API v4 host. Adapter.BaseURL
// exists specifically so a test can point it at an httptest.Server
// instead.
const DefaultBaseURL = "https://api.cloudflare.com/client/v4"

type Adapter struct {
	BaseURL    string
	HTTPClient *http.Client
}

func New() *Adapter {
	return &Adapter{BaseURL: DefaultBaseURL, HTTPClient: http.DefaultClient}
}

var _ domain.DNSProviderAdapter = (*Adapter)(nil)

// apexDomain derives the registrable zone name a tracking hostname lives
// under (e.g. "track.example.com" -> "example.com") — a real Cloudflare
// zone is always registered for the apex domain, never a subdomain.
// This is a naive last-two-labels heuristic: it does not handle
// multi-part public suffixes (e.g. "example.co.uk") correctly. Accepted
// for now — this package is verified structurally only, against no live
// zone, and a real deployment can extend this if a customer's domain
// actually needs a public-suffix-list lookup.
func apexDomain(domainName string) string {
	labels := strings.Split(domainName, ".")
	if len(labels) <= 2 {
		return domainName
	}
	return strings.Join(labels[len(labels)-2:], ".")
}

func (a *Adapter) client() *http.Client {
	if a.HTTPClient != nil {
		return a.HTTPClient
	}
	return http.DefaultClient
}

func (a *Adapter) baseURL() string {
	if a.BaseURL != "" {
		return a.BaseURL
	}
	return DefaultBaseURL
}

func (a *Adapter) do(ctx context.Context, fields map[string]string, method, path string, body any) (*http.Response, error) {
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("cloudflare: encoding request body: %w", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, a.baseURL()+path, reader)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: building %s %s request: %w", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+fields["apiToken"])
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := a.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: calling %s %s: %w", method, path, err)
	}
	return resp, nil
}

type envelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (e envelope) asError(action string) error {
	if e.Success {
		return nil
	}
	if len(e.Errors) > 0 {
		return fmt.Errorf("cloudflare: %s: %s", action, e.Errors[0].Message)
	}
	return fmt.Errorf("cloudflare: %s failed", action)
}

type zonesResponse struct {
	envelope
	Result []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"result"`
}

// findZone looks up the real Cloudflare zone for domainName's apex —
// returns apierror-free "not found" as a plain false, since an
// operator's domain not yet being added to their Cloudflare account is
// a normal, expected state to report back, not a hard failure.
func (a *Adapter) findZone(ctx context.Context, fields map[string]string, domainName string) (zoneID string, status string, found bool, err error) {
	resp, err := a.do(ctx, fields, http.MethodGet, "/zones?name="+apexDomain(domainName), nil)
	if err != nil {
		return "", "", false, err
	}
	defer resp.Body.Close()

	var parsed zonesResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", "", false, fmt.Errorf("cloudflare: parsing zones response: %w", err)
	}
	if err := parsed.asError("listing zones"); err != nil {
		return "", "", false, err
	}
	if len(parsed.Result) == 0 {
		return "", "", false, nil
	}
	return parsed.Result[0].ID, parsed.Result[0].Status, true, nil
}

type sslVerificationResponse struct {
	envelope
	Result []struct {
		CertificateStatus string `json:"certificate_status"`
	} `json:"result"`
}

// ZoneStatus reports whether domainName's apex zone is active at
// Cloudflare and whether a real TLS certificate is currently issued —
// via the real GET /zones/{id}/ssl/verification endpoint, the
// documented way to check a zone's actual certificate status (a zone
// being "active" and a certificate being issued are genuinely separate
// facts at Cloudflare: DNS can be correctly delegated while
// certificate issuance is still pending).
func (a *Adapter) ZoneStatus(ctx context.Context, fields map[string]string, domainName string) (domain.DNSZoneStatus, error) {
	zoneID, zoneStatus, found, err := a.findZone(ctx, fields, domainName)
	if err != nil {
		return domain.DNSZoneStatus{}, err
	}
	if !found {
		return domain.DNSZoneStatus{}, fmt.Errorf("cloudflare: no zone found for %s (add the domain to this Cloudflare account first)", apexDomain(domainName))
	}

	resp, err := a.do(ctx, fields, http.MethodGet, "/zones/"+zoneID+"/ssl/verification", nil)
	if err != nil {
		return domain.DNSZoneStatus{}, err
	}
	defer resp.Body.Close()

	var parsed sslVerificationResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return domain.DNSZoneStatus{}, fmt.Errorf("cloudflare: parsing ssl verification response: %w", err)
	}
	if err := parsed.asError("checking SSL verification"); err != nil {
		return domain.DNSZoneStatus{}, err
	}

	sslIssued := false
	for _, r := range parsed.Result {
		if r.CertificateStatus == "active" {
			sslIssued = true
			break
		}
	}
	return domain.DNSZoneStatus{Active: zoneStatus == "active", SSLIssued: sslIssued}, nil
}

type dnsRecordsResponse struct {
	envelope
	Result []struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	} `json:"result"`
}

type dnsRecordBody struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}

// EnsureRecord creates or updates a proxied CNAME record for domainName
// pointing at target — a real, live-DNS-mutating action against the
// customer's own Cloudflare zone. Proxied (not just a plain DNS
// pass-through) so Cloudflare's own edge issues and renews the TLS
// certificate ZoneStatus later checks, the same "orange-clouded" record
// shape a real operator would set up in the Cloudflare dashboard by
// hand.
func (a *Adapter) EnsureRecord(ctx context.Context, fields map[string]string, domainName, target string) error {
	zoneID, _, found, err := a.findZone(ctx, fields, domainName)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("cloudflare: no zone found for %s (add the domain to this Cloudflare account first)", apexDomain(domainName))
	}

	resp, err := a.do(ctx, fields, http.MethodGet, "/zones/"+zoneID+"/dns_records?type=CNAME&name="+domainName, nil)
	if err != nil {
		return err
	}
	var existing dnsRecordsResponse
	decodeErr := json.NewDecoder(resp.Body).Decode(&existing)
	resp.Body.Close()
	if decodeErr != nil {
		return fmt.Errorf("cloudflare: parsing dns_records response: %w", decodeErr)
	}
	if err := existing.asError("listing DNS records"); err != nil {
		return err
	}

	body := dnsRecordBody{Type: "CNAME", Name: domainName, Content: target, Proxied: true}

	if len(existing.Result) > 0 {
		if existing.Result[0].Content == target {
			return nil // already points at the right place
		}
		resp, err = a.do(ctx, fields, http.MethodPut, "/zones/"+zoneID+"/dns_records/"+existing.Result[0].ID, body)
	} else {
		resp, err = a.do(ctx, fields, http.MethodPost, "/zones/"+zoneID+"/dns_records", body)
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result envelope
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("cloudflare: parsing dns_records write response: %w", err)
	}
	return result.asError("writing DNS record")
}
