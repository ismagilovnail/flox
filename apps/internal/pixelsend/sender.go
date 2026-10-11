package pixelsend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ismagilovnail/flox/apps/internal/pixel"
)

// Sender reports one event to one ad platform — the §74 PixelProvider. A
// Sender never logs the event: it carries the visitor's IP and user agent.
type Sender interface {
	Send(ctx context.Context, c pixel.Credentials, ev pixel.Event) Outcome
}

// Outcome is how one send ended.
type Outcome struct {
	// StatusCode is the HTTP status, 0 when no response came back.
	StatusCode int
	// Err is nil on success. Its text is what the delivery log shows, so
	// it never contains the access token.
	Err error
	// Retry: the failure may pass (network error, timeout, 5xx, rate
	// limit). Otherwise the platform refused the event as sent — a bad
	// token, an unknown pixel — and repeating it cannot succeed.
	Retry bool
}

// HTTPClient is the slice of *http.Client the senders use.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// maxResponseBody bounds how much of a platform's answer is read.
const maxResponseBody = 64 << 10

// errNotConfigured is a pixel that cannot send as configured; it is never
// retried — editing the pixel does not re-send the event.
func errNotConfigured(what string) Outcome {
	return Outcome{Err: fmt.Errorf("pixel is not configured: %s", what)}
}

// do sends req and reads up to maxResponseBody of the answer. A transport
// error is reported without its URL: Facebook and Snapchat take the access
// token in the query string, and *url.Error would print it.
func do(client HTTPClient, req *http.Request) (int, []byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return 0, nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("reading response: %w", err)
	}
	return resp.StatusCode, body, nil
}

// postJSON POSTs body as JSON to endpoint with the given extra headers.
func postJSON(ctx context.Context, client HTTPClient, endpoint string, headers map[string]string, body any) (int, []byte, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, nil, fmt.Errorf("encoding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return do(client, req)
}

// retryableStatus: no answer, a server error, a timeout or a rate limit.
func retryableStatus(code int) bool {
	return code == 0 || code >= 500 || code == http.StatusTooManyRequests || code == http.StatusRequestTimeout
}

// httpOutcome is the outcome of a platform that answers errors with a
// non-2xx status; detail is the platform's own error text.
func httpOutcome(code int, err error, detail string) Outcome {
	if err != nil {
		return Outcome{StatusCode: code, Err: err, Retry: true}
	}
	if code >= 200 && code < 300 {
		return Outcome{StatusCode: code}
	}
	msg := fmt.Sprintf("platform responded %d", code)
	if detail != "" {
		msg += ": " + detail
	}
	return Outcome{StatusCode: code, Err: errors.New(msg), Retry: retryableStatus(code)}
}

// sha256Hex is how platforms take identifiers they match on but should not
// see in the clear (§54): lowercase hex SHA-256 of the trimmed value.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(s))))
	return hex.EncodeToString(sum[:])
}

// value is the event's monetary value: USD at the event date's rate when
// known, else the amount as reported in its own currency, else none.
func value(ev pixel.Event) (amount float64, currency string, ok bool) {
	switch {
	case ev.USDValue != nil:
		return *ev.USDValue, "USD", true
	case ev.Revenue != nil && ev.Currency != "":
		return *ev.Revenue, strings.ToUpper(ev.Currency), true
	}
	return 0, "", false
}

// truncate keeps a platform's error text short enough for the log.
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
