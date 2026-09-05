package integrations

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
)

// DefaultNamecheapBaseURL is the real Namecheap production API host.
// namecheapAdapter.BaseURL exists specifically so a test can point it at
// an httptest.Server instead. This project has no live Namecheap account
// to exercise it against — verified structurally in the test file
// against a fake server standing in for api.namecheap.com, matching the
// real endpoint's documented XML response shape.
const DefaultNamecheapBaseURL = "https://api.namecheap.com/xml.response"

type namecheapAdapter struct {
	BaseURL    string
	HTTPClient *http.Client
}

func newNamecheapAdapter() *namecheapAdapter {
	return &namecheapAdapter{BaseURL: DefaultNamecheapBaseURL, HTTPClient: http.DefaultClient}
}

// namecheapResponse mirrors the subset of Namecheap's real XML envelope
// every API command returns — Status is "OK" or "ERROR" on the root
// element, with Errors/Error populated only in the ERROR case.
type namecheapResponse struct {
	XMLName xml.Name `xml:"ApiResponse"`
	Status  string   `xml:"Status,attr"`
	Errors  struct {
		Error []struct {
			Number int    `xml:"Number,attr"`
			Text   string `xml:",chardata"`
		} `xml:"Error"`
	} `xml:"Errors"`
}

// Verify calls namecheap.users.getBalances — a real, lightweight,
// read-only command that requires a valid ApiUser/ApiKey pair and
// nothing else, making it a natural credential check rather than a
// side-effecting call. clientIp is required by Namecheap's own API for
// every command (the calling IP must be allowlisted in the account's
// API settings), not a FLOX-specific field.
func (a *namecheapAdapter) Verify(ctx context.Context, fields map[string]string) error {
	q := url.Values{
		"ApiUser":  {fields["apiUser"]},
		"ApiKey":   {fields["apiKey"]},
		"UserName": {fields["apiUser"]},
		"ClientIp": {fields["clientIp"]},
		"Command":  {"namecheap.users.getBalances"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.BaseURL+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("integrations: building namecheap request: %w", err)
	}
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("integrations: calling namecheap: %w", err)
	}
	defer resp.Body.Close()

	var parsed namecheapResponse
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("integrations: parsing namecheap response: %w", err)
	}
	if parsed.Status != "OK" {
		if len(parsed.Errors.Error) > 0 {
			return fmt.Errorf("namecheap: %s", parsed.Errors.Error[0].Text)
		}
		return fmt.Errorf("namecheap: request failed with status %q", parsed.Status)
	}
	return nil
}
