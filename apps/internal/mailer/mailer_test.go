package mailer

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestNewParsesSMTPURL(t *testing.T) {
	c, err := New("smtp://localhost:1025", "FLOX <no-reply@floxlink.io>")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.addr != "localhost:1025" {
		t.Fatalf("addr = %q, want localhost:1025", c.addr)
	}
	if c.auth != nil {
		t.Fatalf("auth = %v, want nil for a credential-less SMTP_URL (matches mailhog)", c.auth)
	}
	if c.from != "FLOX <no-reply@floxlink.io>" {
		t.Fatalf("from = %q", c.from)
	}
}

func TestNewParsesCredentials(t *testing.T) {
	c, err := New("smtp://user:s3cret@smtp.example.com:587", "no-reply@example.com")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.addr != "smtp.example.com:587" {
		t.Fatalf("addr = %q, want smtp.example.com:587", c.addr)
	}
	if c.auth == nil {
		t.Fatal("auth = nil, want smtp.PlainAuth built from the URL's userinfo")
	}
}

func TestNewRejectsMissingHost(t *testing.T) {
	if _, err := New("not-a-url", "no-reply@example.com"); err == nil {
		t.Fatal("New with a hostless URL succeeded, want an error")
	}
}

func TestBuildMessageIncludesHeadersAndBody(t *testing.T) {
	msg := string(buildMessage("from@example.com", "to@example.com", "Subject line", "Body text."))

	for _, want := range []string{
		"From: from@example.com\r\n",
		"To: to@example.com\r\n",
		"Subject: Subject line\r\n",
		"\r\n\r\nBody text.",
	} {
		if !contains(msg, want) {
			t.Fatalf("message %q missing %q", msg, want)
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// fakeRelay is a scripted SMTP server: it answers every command with OK
// and records the session. stall makes it accept and then say nothing.
type fakeRelay struct {
	addr    string
	session chan string
}

func startRelay(t *testing.T, stall bool) *fakeRelay {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	r := &fakeRelay{addr: ln.Addr().String(), session: make(chan string, 1)}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if stall {
			time.Sleep(5 * time.Second)
			return
		}
		var log strings.Builder
		rd := bufio.NewReader(conn)
		reply := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		reply("220 fake ESMTP")
		inData := false
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				break
			}
			log.WriteString(line)
			switch {
			case inData && line == ".\r\n":
				inData = false
				reply("250 queued")
			case inData:
			case strings.HasPrefix(line, "EHLO"):
				reply("250-fake\r\n250 8BITMIME")
			case strings.HasPrefix(line, "DATA"):
				inData = true
				reply("354 go on")
			case strings.HasPrefix(line, "QUIT"):
				reply("221 bye")
				r.session <- log.String()
				return
			default:
				reply("250 ok")
			}
		}
		r.session <- log.String()
	}()
	return r
}

func TestSendDelivers(t *testing.T) {
	relay := startRelay(t, false)
	c, err := New("smtp://"+relay.addr, "FLOX <no-reply@floxlink.io>")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Send(context.Background(), "user@example.com", "Hi", "Body."); err != nil {
		t.Fatalf("Send: %v", err)
	}
	session := <-relay.session
	for _, want := range []string{
		"MAIL FROM:<no-reply@floxlink.io>", // the bare address in the envelope
		"RCPT TO:<user@example.com>",
		"From: FLOX <no-reply@floxlink.io>\r\n", // the display name in the header
		"Body.",
	} {
		if !strings.Contains(session, want) {
			t.Errorf("session missing %q:\n%s", want, session)
		}
	}
}

// A relay that accepts and then stalls must not hold the caller (signup
// sends inside the request): the context's deadline ends the send.
func TestSendGivesUpOnAStalledRelay(t *testing.T) {
	relay := startRelay(t, true)
	c, err := New("smtp://"+relay.addr, "no-reply@example.com")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := c.Send(ctx, "user@example.com", "Hi", "Body."); err == nil {
		t.Fatal("Send to a stalled relay succeeded")
	}
	if took := time.Since(started); took > 2*time.Second {
		t.Fatalf("Send took %s, want it bounded by the context's 300ms", took)
	}
}

func TestSendRefusesALineBreakInTheAddress(t *testing.T) {
	c, err := New("smtp://127.0.0.1:1", "no-reply@example.com")
	if err != nil {
		t.Fatal(err)
	}
	err = c.Send(context.Background(), "user@example.com\r\nBcc: victim@example.com", "Hi", "Body.")
	if err == nil || !strings.Contains(err.Error(), "line break") {
		t.Fatalf("err = %v, want the address refused before dialing", err)
	}
}
