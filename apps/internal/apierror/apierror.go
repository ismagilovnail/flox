// Package apierror is the one error shape every domain package's service
// layer returns and every handler renders — so a client sees the same JSON
// error envelope from /campaigns as it will from /offers, /networks, etc.
// once those land, instead of each domain package inventing its own.
package apierror

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/ismagilovnail/flox/apps/internal/chconn"
)

type Error struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *Error) Error() string {
	return e.Message
}

func NotFound(message string) *Error {
	return &Error{Status: http.StatusNotFound, Code: "not_found", Message: message}
}

func Validation(message string, fields map[string]string) *Error {
	return &Error{Status: http.StatusUnprocessableEntity, Code: "validation", Message: message, Fields: fields}
}

func Conflict(message string) *Error {
	return &Error{Status: http.StatusConflict, Code: "conflict", Message: message}
}

// Unauthorized means the request has no valid session/credential at all —
// distinct from Forbidden, which means a real session exists but lacks a
// specific permission (§52).
func Unauthorized(message string) *Error {
	return &Error{Status: http.StatusUnauthorized, Code: "unauthorized", Message: message}
}

// Forbidden means an authenticated session exists but its role lacks the
// permission a route requires (§52's server-side RBAC enforcement).
func Forbidden(message string) *Error {
	return &Error{Status: http.StatusForbidden, Code: "forbidden", Message: message}
}

// Unavailable means a required-but-optional external dependency (object
// storage, etc.) is down — matching ClickHouse/Redis's existing "connect
// best-effort at startup, degrade the routes that actually need it"
// stance (apps/api/main.go). First use: apps/internal/contentgallery when
// apps/internal/storage failed to initialize.
func Unavailable(message string) *Error {
	return &Error{Status: http.StatusServiceUnavailable, Code: "unavailable", Message: message}
}

// TooManyRequests means apps/internal/ratelimit's own Redis-backed
// counter rejected the request (§54) — never returned for any
// other reason.
func TooManyRequests(message string) *Error {
	return &Error{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: message}
}

// PlanLimit means the organization's plan does not allow the request
// (internal/billing): a count limit is reached, or this month's clicks are
// well past the plan's. Only creating things is ever refused this way.
func PlanLimit(message string) *Error {
	return &Error{Status: http.StatusPaymentRequired, Code: "plan_limit", Message: message}
}

// Write renders err as this package's JSON error envelope. Any error not
// constructed via this package (a repository returning a raw pgx error, a
// bug) is logged with detail server-side and rendered to the client as an
// opaque 500 — never leaking internal error text over the wire.
func Write(w http.ResponseWriter, logger *slog.Logger, err error) {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(apiErr.Status)
		_ = json.NewEncoder(w).Encode(apiErr)
		return
	}

	if chconn.IsBusy(err) {
		// The organization runs its plan's number of reports at once
		// (chconn.TenantConn, audit CH-1): retry in a few seconds.
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(&Error{Code: "reports_busy",
			Message: "Too many reports are running at once for this organization. Try again in a few seconds."})
		return
	}

	if chconn.IsQueryTooHeavy(err) {
		// ClickHouse stopped the query at this binary's limits (tech
		// audit CH4): the request asks too much, the server is fine.
		logger.Warn("query stopped at its clickhouse limits", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(&Error{Code: "query_too_heavy",
			Message: "This report needs more time or memory than allowed. Narrow the date range or remove a breakdown."})
		return
	}

	logger.Error("unhandled error", "error", err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(&Error{Code: "internal", Message: "internal server error"})
}
