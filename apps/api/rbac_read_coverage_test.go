package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// openToEveryMember: tenant route groups every member reads, whatever the
// role. Anything else must carry its read permission.
var openToEveryMember = map[string]string{
	"/notifications": "every role reads its own notifications",
}

// TestEveryTenantRouteGroupIsReadGated is the read-side twin of
// TestEveryMutatingRouteIsPermissionGated. Until 2026-10-10 only team.read
// was enforced: a Viewer, whose role lacks offer.read and source.read,
// still read offers, networks and traffic sources through the API. Every
// tenant route group in this file now mounts readGate(<permission>.read);
// a new group without one fails here.
func TestEveryTenantRouteGroupIsReadGated(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	route := regexp.MustCompile(`srv\.Mux\(\)\.Route\("([^"]+)", func\(r chi\.Router\) \{`)
	var missing []string
	locs := route.FindAllSubmatchIndex(src, -1)
	for i, loc := range locs {
		path := string(src[loc[2]:loc[3]])
		end := len(src)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		block := string(src[loc[1]:end])
		block = block[:strings.Index(block, "\n\t}")+1]
		if !strings.Contains(block, "tenantMiddleware") {
			continue
		}
		if _, ok := openToEveryMember[path]; ok {
			continue
		}
		if !strings.Contains(block, "readGate(") {
			missing = append(missing, path)
		}
	}
	if len(locs) < 20 {
		t.Fatalf("found only %d route groups — the pattern no longer matches main.go", len(locs))
	}
	if len(missing) > 0 {
		t.Fatalf("tenant route groups without a read permission: %v\nadd r.Use(readGate(\"<x>.read\")) or list the group in openToEveryMember with a reason", missing)
	}
}
