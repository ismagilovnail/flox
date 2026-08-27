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
// specific permission. First use: §52/Phase 28 auth.
func Unauthorized(message string) *Error {
	return &Error{Status: http.StatusUnauthorized, Code: "unauthorized", Message: message}
}

// Forbidden means an authenticated session exists but its role lacks the
// permission a route requires (§52's server-side RBAC enforcement).
func Forbidden(message string) *Error {
	return &Error{Status: http.StatusForbidden, Code: "forbidden", Message: message}
}

// TooManyRequests means apps/internal/ratelimit's own Redis-backed
// counter rejected the request (§54/Phase 30) — never returned for any
// other reason.
func TooManyRequests(message string) *Error {
	return &Error{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: message}
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

	logger.Error("unhandled error", "error", err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(&Error{Code: "internal", Message: "internal server error"})
}
