package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ismagilovnail/flox/apps/internal/urlsafety"
)

// slackAdapter posts to a real, operator-supplied Slack incoming-webhook
// URL — unlike Namecheap/Cloudflare, this needs no registered OAuth app
// at all (an incoming webhook is created once, by hand, in the target
// Slack workspace), so it's real, live-testable infrastructure the
// moment an operator has a URL to give it, not just structurally
// verified against a fake transport.
//
// The webhook URL is operator-supplied and this package makes a real
// server-side POST to it — the same SSRF shape apps/internal/urlsafety
// (§54) already exists for (network.Service's own postback_url field).
// ValidateURL rejects an obviously-bad URL before ever dialing it;
// HTTPClient's DialContext additionally closes the DNS-rebinding gap a
// save-time check alone can't.
type slackAdapter struct {
	HTTPClient *http.Client
}

func newSlackAdapter() *slackAdapter {
	return &slackAdapter{
		HTTPClient: &http.Client{Transport: &http.Transport{DialContext: urlsafety.SafeDialContext}},
	}
}

func (a *slackAdapter) Verify(ctx context.Context, fields map[string]string) error {
	webhookURL := fields["webhookUrl"]
	if err := urlsafety.ValidateURL(webhookURL); err != nil {
		return fmt.Errorf("slack: %w", err)
	}
	return a.post(ctx, webhookURL)
}

// post is split out from Verify purely so tests can exercise the real
// request/response handling against an httptest.Server (which binds to
// loopback, and so would always fail ValidateURL's own real SSRF check)
// without weakening Verify's actual production behavior, which always
// validates first.
func (a *slackAdapter) post(ctx context.Context, webhookURL string) error {
	body, err := json.Marshal(map[string]string{"text": "FLOX connected to this Slack channel. Campaign and team alerts will post here."})
	if err != nil {
		return fmt.Errorf("integrations: encoding slack payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("integrations: building slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("integrations: calling slack: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack: webhook rejected the request: %s", bytes.TrimSpace(respBody))
	}
	return nil
}
