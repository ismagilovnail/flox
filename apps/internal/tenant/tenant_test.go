package tenant_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestRequireSameOrigin(t *testing.T) {
	mw := tenant.RequireSameOrigin("https://app.floxlink.io", discardLogger())
	handler := mw(okHandler())

	cases := []struct {
		name       string
		method     string
		origin     string
		wantStatus int
	}{
		{"matching origin allowed", http.MethodPost, "https://app.floxlink.io", http.StatusOK},
		{"mismatched origin rejected", http.MethodPost, "https://evil.example", http.StatusForbidden},
		{"missing origin allowed (non-browser client)", http.MethodPost, "", http.StatusOK},
		{"GET never checked, even cross-origin", http.MethodGet, "https://evil.example", http.StatusOK},
		{"PATCH mismatched origin rejected", http.MethodPatch, "https://evil.example", http.StatusForbidden},
		{"DELETE mismatched origin rejected", http.MethodDelete, "https://evil.example", http.StatusForbidden},
		{"PUT mismatched origin rejected", http.MethodPut, "https://evil.example", http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/campaigns", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}
