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
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

// Client sends mail over SMTP using Go's standard library client — no
// third-party dependency for something this simple (CLAUDE.md: "no
// unnecessary frameworks"). Works unauthenticated against
// infra/docker-compose.dev.yml's mailhog in dev, and against a real
// authenticated relay (SES SMTP, Postmark, Sendgrid, a corporate relay,
// ...) in production by putting user:pass in SMTP_URL.
type Client struct {
	addr string // host:port Send dials
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

// sendTimeout bounds one message end to end: dial, TLS, AUTH, DATA. Signup
// sends inside the request, so a relay that accepts the connection and then
// stalls must not hold the request — smtp.SendMail has no context and no
// deadline at all.
const sendTimeout = 30 * time.Second

// Send implements auth.Mailer: what smtp.SendMail does (STARTTLS when the
// relay offers it, AUTH when credentials are set), over a connection
// bounded by ctx and sendTimeout.
func (c *Client) Send(ctx context.Context, to, subject, body string) error {
	if err := c.send(ctx, to, buildMessage(c.from, to, subject, body)); err != nil {
		return fmt.Errorf("mailer: sending to %s: %w", to, err)
	}
	return nil
}

func (c *Client) send(ctx context.Context, to string, msg []byte) error {
	// An address with a line break would add headers or SMTP commands.
	for _, v := range []string{c.from, to} {
		if strings.ContainsAny(v, "\r\n") {
			return errors.New("address contains a line break")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return err
	}
	host, _, _ := net.SplitHostPort(c.addr)
	sc, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer sc.Close()
	if ok, _ := sc.Extension("STARTTLS"); ok {
		if err := sc.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return err
		}
	}
	if c.auth != nil {
		if ok, _ := sc.Extension("AUTH"); !ok {
			return errors.New("relay does not offer AUTH but SMTP_URL has credentials")
		}
		if err := sc.Auth(c.auth); err != nil {
			return err
		}
	}
	// The envelope takes the bare address; MAIL_FROM may carry a display
	// name ("FLOX <no-reply@…>"), which belongs in the From header only.
	from := c.from
	if a, err := mail.ParseAddress(c.from); err == nil {
		from = a.Address
	}
	if err := sc.Mail(from); err != nil {
		return err
	}
	if err := sc.Rcpt(to); err != nil {
		return err
	}
	w, err := sc.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return sc.Quit()
}

// buildMessage is a minimal RFC 5322 message: headers, a blank line, then
// the plain-text body. No HTML/multipart — every email this codebase
// sends is one short paragraph plus a link.
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
