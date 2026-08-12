// Package idgen generates and validates the ULIDs used as every table's
// primary key (§35 — "one standard, consistently, everywhere"). Output
// matches the Postgres `ulid` domain's format CHECK exactly (Crockford
// base32, 26 chars; the timestamp component keeps the first character in
// 0-7 for any millisecond timestamp before the year 10889).
package idgen

import (
	"regexp"

	"github.com/oklog/ulid/v2"
)

var pattern = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)

// New is safe for concurrent use — ulid.Make() guards its monotonic
// entropy source internally.
func New() string {
	return ulid.Make().String()
}

func IsValid(s string) bool {
	return pattern.MatchString(s)
}
