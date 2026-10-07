package tlsask_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/tlsask"
)

func TestAskAllowsOnlyRegisteredDomains(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	orgID := idgen.New()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, orgID, "tls ask "+orgID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID) })
	host := "ask-" + strings.ToLower(idgen.New()) + ".example.com"
	if _, err := pool.Exec(ctx, `INSERT INTO domains (id, organization_id, domain) VALUES ($1, $2, $3)`, idgen.New(), orgID, host); err != nil {
		t.Fatal(err)
	}

	h := tlsask.Handler(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, tc := range []struct {
		query string
		want  int
	}{
		{"domain=" + host, http.StatusOK},
		{"domain=" + strings.ToUpper(host), http.StatusOK},
		{"domain=not-" + host, http.StatusNotFound},
		{"", http.StatusBadRequest},
	} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, tlsask.Path+"?"+tc.query, nil))
		if rec.Code != tc.want {
			t.Errorf("%q: status %d, want %d", tc.query, rec.Code, tc.want)
		}
	}
}
