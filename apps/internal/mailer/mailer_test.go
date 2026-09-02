package mailer

import "testing"

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
