// Package httpclient builds the clients FLOX calls third-party APIs with:
// ad networks (cost sync), registrars and DNS, Slack. Every one has a
// timeout on the whole call — connect, TLS, headers, body.
//
// http.DefaultClient has none. Before the 2026-10-06 tech audit the cost
// sync used it: one ad network leaving a connection open blocked the sync
// of every organization behind it until the worker restarted, and the same
// held for the registrar/DNS calls and the Slack alert webhook.
//
// A context deadline still applies; the shorter of the two wins. Outgoing
// postbacks have their own client and a 10 s deadline per attempt
// (internal/postback, worker/main.go).
package httpclient

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/urlsafety"
)

// Timeouts per kind of API.
const (
	// AdAPI: cost reports (Facebook, TikTok, Google Ads) can take a while
	// for a large account.
	AdAPI = 60 * time.Second
	// ControlAPI: registrars, DNS — small requests.
	ControlAPI = 30 * time.Second
	// Webhook: a notification; a slow receiver must not hold up the
	// alerts behind it.
	Webhook = 10 * time.Second
)

// New is a client bounded by timeout.
func New(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: transport(nil)}
}

// NewSafe is New dialing through urlsafety.SafeDialContext: for URLs an
// operator typed in (a Slack webhook), which must not reach private
// addresses even after a DNS rebind.
func NewSafe(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: transport(urlsafety.SafeDialContext)}
}

func transport(dial func(ctx context.Context, network, addr string) (net.Conn, error)) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	if dial != nil {
		t.DialContext = dial
		// Through a proxy the dial would check the proxy's address, not
		// the target's: no HTTP_PROXY for a guarded client.
		t.Proxy = nil
	}
	return t
}
