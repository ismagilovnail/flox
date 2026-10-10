package chstore

import (
	"regexp"
	"strconv"
)

// serverError matches a ClickHouse server exception as clickhouse-go
// reports it over HTTP — a plain error whose text carries the code:
// `[HTTP 500] response body: "Code: 16. DB::Exception: ...`.
var serverError = regexp.MustCompile(`Code: (\d+)\. DB::Exception`)

// transientCodes are server exceptions that say "not now", not "never":
// overload, limits, timeouts, a replica or Keeper hiccup. Retried like a
// network error.
var transientCodes = map[int]bool{
	3:   true, // UNEXPECTED_END_OF_FILE
	159: true, // TIMEOUT_EXCEEDED
	160: true, // TOO_SLOW
	164: true, // READONLY
	202: true, // TOO_MANY_SIMULTANEOUS_QUERIES
	203: true, // NO_FREE_CONNECTION
	209: true, // SOCKET_TIMEOUT
	210: true, // NETWORK_ERROR
	236: true, // ABORTED
	241: true, // MEMORY_LIMIT_EXCEEDED
	242: true, // TABLE_IS_READ_ONLY
	252: true, // TOO_MANY_PARTS
	394: true, // QUERY_WAS_CANCELLED
	425: true, // SYSTEM_ERROR
	999: true, // KEEPER_EXCEPTION
}

// Rejected reports whether the ClickHouse server refused the data itself
// (a type it cannot take, a column it does not have) — retrying the same
// rows will fail the same way. Network errors, timeouts and transient
// server exceptions are not rejections (tech audit Q2).
func Rejected(err error) bool {
	if err == nil {
		return false
	}
	m := serverError.FindStringSubmatch(err.Error())
	if m == nil {
		return false
	}
	code, convErr := strconv.Atoi(m[1])
	return convErr == nil && !transientCodes[code]
}
