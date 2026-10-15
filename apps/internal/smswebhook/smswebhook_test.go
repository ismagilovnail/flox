package smswebhook_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/smswebhook"
)

const secret = "sixteen-chars-secret"

// The gateway gets {to, text} as JSON with a timestamp and an HMAC it can
// recompute; a non-2xx answer is an error without the body.
func TestSendSMS(t *testing.T) {
	var got map[string]string
	var ts, sig string
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		ts, sig = r.Header.Get("X-Flox-Timestamp"), r.Header.Get("X-Flox-Signature")
		if got["to"] == "+15550000000" {
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte("code 123456 failed"))
		}
	}))
	defer srv.Close()
	s, err := smswebhook.New(srv.URL, secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SendSMS(context.Background(), "+79001234567", "FLOX code: 123456"); err != nil {
		t.Fatalf("SendSMS: %v", err)
	}
	if got["to"] != "+79001234567" || got["text"] != "FLOX code: 123456" || ts == "" || sig != smswebhook.Sign([]byte(secret), ts, raw) {
		t.Fatalf("request = %v ts %q sig %q", got, ts, sig)
	}
	err = s.SendSMS(context.Background(), "+15550000000", "x")
	if err == nil || !strings.Contains(err.Error(), "502") || strings.Contains(err.Error(), "123456") {
		t.Fatalf("gateway error = %v, want the status and not the body", err)
	}
}

func TestNewRefusesBadConfig(t *testing.T) {
	if _, err := smswebhook.New("ftp://gw", secret); err == nil {
		t.Fatal("ftp URL accepted")
	}
	if _, err := smswebhook.New("https://gw.example/sms", "short"); err == nil {
		t.Fatal("short secret accepted")
	}
}
