// Package namecheap implements domain.RegistrarProvider against the real
// Namecheap API (domains.getInfo/domains.renew) — §30's registrar
// automation. This project has no live Namecheap account to exercise it
// against; it's verified structurally in namecheap_test.go against a
// fake httptest.Server standing in for api.namecheap.com, matching the
// real endpoints' documented XML response shape (confirmed via
// AskUserQuestion before this phase started — see
// docs/domains-tracking-links.md).
package namecheap

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/domain"
)

// DefaultBaseURL is the real Namecheap production API host. Adapter.
// BaseURL exists specifically so a test can point it at an
// httptest.Server instead.
const DefaultBaseURL = "https://api.namecheap.com/xml.response"

// dateLayout is Namecheap's own documented date format for
// domains.getInfo/domains.renew's Expires/ExpireDate fields — M/D/YYYY,
// not ISO 8601.
const dateLayout = "1/2/2006"

type Adapter struct {
	BaseURL    string
	HTTPClient *http.Client
}

func New() *Adapter {
	return &Adapter{BaseURL: DefaultBaseURL, HTTPClient: http.DefaultClient}
}

var _ domain.RegistrarProvider = (*Adapter)(nil)

type ErrorResponse struct {
	XMLName xml.Name `xml:"ApiResponse"`
	Status  string   `xml:"Status,attr"`
	Errors  struct {
		Error []struct {
			Number int    `xml:"Number,attr"`
			Text   string `xml:",chardata"`
		} `xml:"Error"`
	} `xml:"Errors"`
}

func (e ErrorResponse) asError() error {
	if e.Status == "OK" {
		return nil
	}
	if len(e.Errors.Error) > 0 {
		return fmt.Errorf("namecheap: %s", e.Errors.Error[0].Text)
	}
	return fmt.Errorf("namecheap: request failed with status %q", e.Status)
}

type getInfoResponse struct {
	ErrorResponse
	CommandResponse struct {
		DomainGetInfoResult struct {
			DomainDetails struct {
				ExpiredDate string `xml:"ExpiredDate"`
			} `xml:"DomainDetails"`
		} `xml:"DomainGetInfoResult"`
	} `xml:"CommandResponse"`
}

func (a *Adapter) do(ctx context.Context, fields map[string]string, command string, extra url.Values) (*http.Response, error) {
	client := a.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	baseURL := a.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	q := url.Values{
		"ApiUser":  {fields["apiUser"]},
		"ApiKey":   {fields["apiKey"]},
		"UserName": {fields["apiUser"]},
		"ClientIp": {fields["clientIp"]},
		"Command":  {command},
	}
	for k, v := range extra {
		q[k] = v
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("namecheap: building %s request: %w", command, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("namecheap: calling %s: %w", command, err)
	}
	return resp, nil
}

// DomainInfo calls the real namecheap.domains.getInfo command — a
// read-only lookup of the domain's own current registration details,
// including its real expiry date.
func (a *Adapter) DomainInfo(ctx context.Context, fields map[string]string, domainName string) (domain.RegistrarInfo, error) {
	resp, err := a.do(ctx, fields, "namecheap.domains.getInfo", url.Values{"DomainName": {domainName}})
	if err != nil {
		return domain.RegistrarInfo{}, err
	}
	defer resp.Body.Close()

	var parsed getInfoResponse
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return domain.RegistrarInfo{}, fmt.Errorf("namecheap: parsing getInfo response: %w", err)
	}
	if err := parsed.asError(); err != nil {
		return domain.RegistrarInfo{}, err
	}

	expires, err := time.Parse(dateLayout, parsed.CommandResponse.DomainGetInfoResult.DomainDetails.ExpiredDate)
	if err != nil {
		return domain.RegistrarInfo{}, fmt.Errorf("namecheap: parsing ExpiredDate %q: %w", parsed.CommandResponse.DomainGetInfoResult.DomainDetails.ExpiredDate, err)
	}
	return domain.RegistrarInfo{ExpiresAt: expires}, nil
}

type renewResponse struct {
	ErrorResponse
	CommandResponse struct {
		DomainRenewResult struct {
			DomainDetails struct {
				ExpiredDate string `xml:"ExpiredDate"`
			} `xml:"DomainDetails"`
		} `xml:"DomainRenewResult"`
	} `xml:"CommandResponse"`
}

// Renew calls the real namecheap.domains.renew command — a genuine
// billable action against the connected Namecheap account. A nil error
// means Namecheap actually processed and charged the renewal.
func (a *Adapter) Renew(ctx context.Context, fields map[string]string, domainName string, years int) (domain.RegistrarInfo, error) {
	resp, err := a.do(ctx, fields, "namecheap.domains.renew", url.Values{
		"DomainName": {domainName},
		"Years":      {strconv.Itoa(years)},
	})
	if err != nil {
		return domain.RegistrarInfo{}, err
	}
	defer resp.Body.Close()

	var parsed renewResponse
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return domain.RegistrarInfo{}, fmt.Errorf("namecheap: parsing renew response: %w", err)
	}
	if err := parsed.asError(); err != nil {
		return domain.RegistrarInfo{}, err
	}

	expires, err := time.Parse(dateLayout, parsed.CommandResponse.DomainRenewResult.DomainDetails.ExpiredDate)
	if err != nil {
		return domain.RegistrarInfo{}, fmt.Errorf("namecheap: parsing renewed ExpiredDate %q: %w", parsed.CommandResponse.DomainRenewResult.DomainDetails.ExpiredDate, err)
	}
	return domain.RegistrarInfo{ExpiresAt: expires}, nil
}
