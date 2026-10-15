package auth

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// The web's sign-up and invite forms refuse a short password with
// MIN_PASSWORD_LENGTH (apps/web/src/schemas/fields.ts); it must stay this
// service's minPasswordLength, or the form lets through what the API refuses.
func TestPasswordMinimumMatchesTheFrontend(t *testing.T) {
	src, err := os.ReadFile("../../web/src/schemas/fields.ts")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`MIN_PASSWORD_LENGTH = (\d+);`).FindSubmatch(src)
	if m == nil {
		t.Fatal("MIN_PASSWORD_LENGTH not found in schemas/fields.ts")
	}
	if n, _ := strconv.Atoi(string(m[1])); n != minPasswordLength {
		t.Fatalf("frontend MIN_PASSWORD_LENGTH = %d, backend minPasswordLength = %d", n, minPasswordLength)
	}
}
