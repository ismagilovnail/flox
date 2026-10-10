// Package pagination is the one limit/offset contract of the API's entity
// lists (offers, networks, landings, ... — tech audit §5 "lists
// without LIMIT"): ?limit=&offset=, at most MaxLimit rows a response, and
// the list's total beside the rows, so a client knows when it has them all.
// apps/web reads every page (src/lib/api/pagination.ts) and keeps its
// tables client-side; an integration can stop at the page it needs.
//
// A paginated query must order by a unique key (an entity's name is not;
// its id is the tie-break), or rows shift between pages.
package pagination

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

const (
	// MaxLimit bounds one response; DefaultLimit is what a request without
	// ?limit= gets — the same, so a client that never pages sees every row
	// of a list up to MaxLimit.
	MaxLimit     = 500
	DefaultLimit = MaxLimit
)

// Page is one requested slice of a list.
type Page struct {
	Limit, Offset int
}

// FromRequest reads ?limit= and ?offset=. A limit over MaxLimit is clamped
// to it; a value that is not a number, a limit below 1 or a negative offset
// is a 422.
func FromRequest(r *http.Request) (Page, error) {
	p := Page{Limit: DefaultLimit}
	q := r.URL.Query()
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Page{}, apierror.Validation("invalid limit", map[string]string{"limit": "must be a whole number from 1"})
		}
		p.Limit = min(n, MaxLimit)
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return Page{}, apierror.Validation("invalid offset", map[string]string{"offset": "must be a whole number from 0"})
		}
		p.Offset = n
	}
	return p, nil
}

// Querier is the slice of pgx a count needs (a pool or a transaction).
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Count is a list's total: the organization's rows in table, a table name
// constant — never input.
func Count(ctx context.Context, db Querier, table, orgID string) (int, error) {
	var n int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE organization_id = $1`, orgID).Scan(&n); err != nil {
		return 0, fmt.Errorf("pagination: counting %s: %w", table, err)
	}
	return n, nil
}
