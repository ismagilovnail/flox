package httpclient_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/adaccount/facebookads"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/googleads"
	"github.com/ismagilovnail/flox/apps/internal/adaccount/tiktokads"
	"github.com/ismagilovnail/flox/apps/internal/domain/cloudflare"
	"github.com/ismagilovnail/flox/apps/internal/domain/namecheap"
	"github.com/ismagilovnail/flox/apps/internal/httpclient"
)

// A server that accepts and never answers no longer holds the caller.
func TestNewGivesUpOnAStalledServer(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(func() { close(release); srv.Close() })

	started := time.Now()
	resp, err := httpclient.New(200 * time.Millisecond).Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("request to a stalled server succeeded")
	}
	if took := time.Since(started); took > 2*time.Second {
		t.Fatalf("took %s, want the 200ms timeout", took)
	}
}

func TestNewSafeRefusesPrivateAddressesAndProxies(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	t.Cleanup(srv.Close)

	c := httpclient.NewSafe(time.Second)
	if c.Transport.(*http.Transport).Proxy != nil {
		t.Error("a guarded client must not go through a proxy: the dial check would see the proxy, not the target")
	}
	if resp, err := c.Get(srv.URL); err == nil {
		resp.Body.Close()
		t.Fatal("guarded client reached a loopback server")
	}
}

// Every third-party adapter's default client has a timeout (tech audit:
// http.DefaultClient has none).
func TestAdaptersHaveATimeout(t *testing.T) {
	for name, c := range map[string]*http.Client{
		"facebookads": facebookads.New().HTTPClient,
		"tiktokads":   tiktokads.New().HTTPClient,
		"googleads":   googleads.New().HTTPClient,
		"cloudflare":  cloudflare.New().HTTPClient,
		"namecheap":   namecheap.New().HTTPClient,
	} {
		if c == nil || c == http.DefaultClient || c.Timeout <= 0 {
			t.Errorf("%s: client %v has no timeout", name, c)
		}
	}
}
