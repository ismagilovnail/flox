// Package smswebhook sends SMS through the operator's own gateway: FLOX
// POSTs {to, text} as JSON to SMS_WEBHOOK_URL and the gateway hands it to
// whichever SMS provider it uses (docs/auth.md "Second factor: email and
// SMS"). Each request is signed so the gateway can tell it comes from
// FLOX: X-Flox-Timestamp (Unix seconds) and X-Flox-Signature =
// hex(HMAC-SHA256(SMS_WEBHOOK_SECRET, timestamp + "." + body)). The
// gateway should refuse a timestamp more than a few minutes old.
package smswebhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// MinSecretLength guards against a guessable signing secret.
const MinSecretLength = 16

// Sender posts SMS to the gateway.
type Sender struct {
	url    string
	secret []byte
	client *http.Client
	now    func() time.Time
}

// New is a Sender for url signed with secret; both are required.
func New(rawURL, secret string) (*Sender, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("smswebhook: SMS_WEBHOOK_URL must be an http(s) URL")
	}
	if len(secret) < MinSecretLength {
		return nil, fmt.Errorf("smswebhook: SMS_WEBHOOK_SECRET must be at least %d characters", MinSecretLength)
	}
	return &Sender{url: rawURL, secret: []byte(secret), client: &http.Client{Timeout: 10 * time.Second}, now: time.Now}, nil
}

// WithClient replaces the HTTP client (tests).
func (s *Sender) WithClient(c *http.Client) *Sender {
	s.client = c
	return s
}

// Sign is the signature of body sent at timestamp — what the gateway
// recomputes to check a request.
func Sign(secret []byte, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// SendSMS posts one message; any non-2xx answer is an error (carrying the
// status, never the body — it could echo the code).
func (s *Sender) SendSMS(ctx context.Context, to, text string) error {
	body, err := json.Marshal(map[string]string{"to": to, "text": text})
	if err != nil {
		return err
	}
	ts := strconv.FormatInt(s.now().Unix(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("smswebhook: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Flox-Timestamp", ts)
	req.Header.Set("X-Flox-Signature", Sign(s.secret, ts, body))
	resp, err := s.client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return fmt.Errorf("smswebhook: sending: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("smswebhook: gateway answered %d", resp.StatusCode)
	}
	return nil
}
