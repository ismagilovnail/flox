package pagination_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/pagination"
)

func TestFromRequest(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  pagination.Page
		bad   bool
	}{
		{"", pagination.Page{Limit: 500}, false},
		{"limit=20&offset=40", pagination.Page{Limit: 20, Offset: 40}, false},
		{"limit=10000", pagination.Page{Limit: 500}, false},
		{"limit=0", pagination.Page{}, true},
		{"limit=-5", pagination.Page{}, true},
		{"limit=abc", pagination.Page{}, true},
		{"offset=-1", pagination.Page{}, true},
		{"offset=1.5", pagination.Page{}, true},
	} {
		got, err := pagination.FromRequest(httptest.NewRequest(http.MethodGet, "/offers?"+tc.query, nil))
		if tc.bad {
			var apiErr *apierror.Error
			if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnprocessableEntity {
				t.Errorf("%q: err = %v, want a 422", tc.query, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%q: got %+v, %v; want %+v", tc.query, got, err, tc.want)
		}
	}
}
