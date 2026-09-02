// Package mailer sends outbound email over SMTP — the real implementation
// docs/environment.md's SMTP_URL variable and infra/docker-compose.dev.yml's
// mailhog service were staged ahead of (see that file's own comment: "no
// SMTP client exists in this codebase yet... this is infra staged ahead of
// that"). First (and, as of this package landing, only) consumer is
// apps/internal/auth's signup email-verification flow.
//
// Behind an interface (CLAUDE.md non-negotiable #11: providers swappable,
// no vendor lock-in) — auth.Mailer is the narrow shape auth actually
// consumes; Client below satisfies it, but nothing in auth imports this
// package's types directly.
package mailer

import (
	"context"
	"fmt"
	"net/smtp"
	"net/url"
	"strings"
)

// Client sends mail over SMTP using Go's standard library client — no
// third-party dependency for something this simple (CLAUDE.md: "no
// unnecessary frameworks"). Works unauthenticated against
// infra/docker-compose.dev.yml's mailhog in dev, and against a real
// authenticated relay (SES SMTP, Postmark, Sendgrid, a corporate relay,
// ...) in production by putting user:pass in SMTP_URL.
type Client struct {
	addr string // host:port smtp.SendMail dials
	auth smtp.Auth
	from string
}

// New parses rawURL (SMTP_URL, e.g. "smtp://localhost:1025" in dev or
// "smtp://user:pass@smtp.example.com:587" in production) and returns a
// Client that sends as fromAddr (MAIL_FROM). No connection is opened here
// — smtp.SendMail dials fresh per message, same as every other outbound
// call in this codebase (postback delivery, ad-account sync) rather than
// holding a long-lived connection open.
func New(rawURL, fromAddr string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("mailer: parsing SMTP_URL: %w", err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("mailer: SMTP_URL %q has no host:port", rawURL)
	}

	var auth smtp.Auth
	if u.User != nil {
		pass, _ := u.User.Password()
		auth = smtp.PlainAuth("", u.User.Username(), pass, u.Hostname())
	}

	return &Client{addr: u.Host, auth: auth, from: fromAddr}, nil
}

// Send implements auth.Mailer. ctx is accepted for interface consistency
// with every other outbound-provider interface in this codebase (e.g.
// postbacklogs.OutgoingEnqueuer) — net/smtp has no context-aware dial, so
// it isn't threaded through smtp.SendMail itself; a slow/unreachable relay
// blocks for its own dial/network timeout, not ctx's deadline.
func (c *Client) Send(_ context.Context, to, subject, body string) error {
	msg := buildMessage(c.from, to, subject, body)
	if err := smtp.SendMail(c.addr, c.auth, c.from, []string{to}, msg); err != nil {
		return fmt.Errorf("mailer: sending to %s: %w", to, err)
	}
	return nil
}

// buildMessage is a minimal RFC 5322 message: headers, a blank line, then
// the plain-text body. No HTML/multipart — every email this codebase
// sends so far is one short paragraph plus a link, matching the plain
// invite-URL-in-a-JSON-response the invite flow already sends the same
// content as (before this package existed, that link never left the app;
// now it also goes out as this).
func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}
