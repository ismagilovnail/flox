package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ismagilovnail/flox/apps/internal/httpclient"
)

// DefaultCloudflareBaseURL is the real Cloudflare API v4 host.
// cloudflareAdapter.BaseURL exists specifically so a test can point it
// at an httptest.Server instead. This project has no live Cloudflare
// account to exercise it against — verified structurally in the test
// file against a fake server, matching the real endpoint's documented
// JSON response shape.
const DefaultCloudflareBaseURL = "https://api.cloudflare.com/client/v4"

type cloudflareAdapter struct {
	BaseURL    string
	HTTPClient *http.Client
}

func newCloudflareAdapter() *cloudflareAdapter {
	return &cloudflareAdapter{BaseURL: DefaultCloudflareBaseURL, HTTPClient: httpclient.New(httpclient.ControlAPI)}
}

// verifyResponse mirrors the subset of Cloudflare's real JSON envelope
// GET /user/tokens/verify returns — the one endpoint whose entire
// purpose is checking whether an API token is valid, making it the
// natural credential check (as opposed to a side-effecting call).
type cloudflareVerifyResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (a *cloudflareAdapter) Verify(ctx context.Context, fields map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.BaseURL+"/user/tokens/verify", nil)
	if err != nil {
		return fmt.Errorf("integrations: building cloudflare request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+fields["apiToken"])

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("integrations: calling cloudflare: %w", err)
	}
	defer resp.Body.Close()

	var parsed cloudflareVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("integrations: parsing cloudflare response: %w", err)
	}
	if !parsed.Success {
		if len(parsed.Errors) > 0 {
			return fmt.Errorf("cloudflare: %s", parsed.Errors[0].Message)
		}
		return fmt.Errorf("cloudflare: token verification failed")
	}
	return nil
}
