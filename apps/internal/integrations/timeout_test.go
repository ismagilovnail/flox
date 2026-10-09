package integrations

import (
	"net/http"
	"testing"
)

// Every integration's client has a timeout (tech audit: http.DefaultClient
// has none; a stalled Slack webhook held up the alerts behind it).
func TestIntegrationClientsHaveATimeout(t *testing.T) {
	for name, c := range map[string]*http.Client{
		"slack":      newSlackAdapter().HTTPClient,
		"cloudflare": newCloudflareAdapter().HTTPClient,
		"namecheap":  newNamecheapAdapter().HTTPClient,
	} {
		if c == nil || c == http.DefaultClient || c.Timeout <= 0 {
			t.Errorf("%s: client has no timeout", name)
		}
	}
}
