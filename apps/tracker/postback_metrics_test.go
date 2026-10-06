package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
)

// Audit 2026-10-06, §14: incoming postbacks are counted by result, including
// the ones refused before the conversion engine runs.
func TestPostbackIncomingIsCountedByResult(t *testing.T) {
	networkID := idgen.New()
	h := &PostbackHandler{
		networks: staticNetworks{conversion.Network{ID: networkID, OrganizationID: idgen.New(), PostbackSecretHash: hashFor("s3cret")}},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	r := chi.NewRouter()
	h.Register(r)

	for _, tc := range []struct{ path, result string }{
		{"/postback/" + networkID + "?secret=s3cret&click_id=c&status=sale&revenue=NaN", "bad_request"},
		{"/postback/" + networkID + "?secret=wrong&click_id=c&status=sale", "unauthorized"},
		{"/postback/not-an-id?secret=s3cret", "unknown_network"},
	} {
		before := testutil.ToFloat64(metrics.PostbackIncomingTotal.WithLabelValues(tc.result))
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tc.path, nil))
		if got := testutil.ToFloat64(metrics.PostbackIncomingTotal.WithLabelValues(tc.result)) - before; got != 1 {
			t.Errorf("%s: postback_incoming_total{result=%q} grew by %v, want 1", tc.path, tc.result, got)
		}
	}
}
