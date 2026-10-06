package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

type staticNetworks struct{ n conversion.Network }

func (s staticNetworks) ByID(context.Context, string) (conversion.Network, error) { return s.n, nil }

// Audit 2026-10-06, S3: a malformed amount is a 400 (never retried), and it
// is refused before the conversion service is reached — service is nil
// here, so any request that got past the check would panic the test.
func TestPostbackRejectsInvalidRevenue(t *testing.T) {
	networkID := idgen.New()
	h := &PostbackHandler{
		networks: staticNetworks{conversion.Network{ID: networkID, OrganizationID: idgen.New(), PostbackSecretHash: hashFor("s3cret")}},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	r := chi.NewRouter()
	h.Register(r)

	for _, revenue := range []string{"NaN", "Inf", "-Inf", "-10", "1e10", "1e400", "ten"} {
		req := httptest.NewRequest(http.MethodGet,
			"/postback/"+networkID+"?secret=s3cret&click_id=c1&status=sale&revenue="+revenue, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("revenue=%s: status = %d, want 400", revenue, rec.Code)
		}
		var body postbackResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("revenue=%s: decoding body: %v", revenue, err)
		}
		if body.Result != "error" || !strings.HasPrefix(body.Message, "invalid revenue") {
			t.Fatalf("revenue=%s: body = %+v, want an invalid revenue error", revenue, body)
		}
	}
}
