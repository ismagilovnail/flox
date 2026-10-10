package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryMutatingRouteIsPermissionGated is the regression guard for the
// 2026-09-06 audit's first finding: the role -> permission policy seeded
// by migrations 00001/00020 was real in the database and enforced almost
// nowhere. 82 mutating routes across 17 packages — campaigns, offers,
// networks, traffic sources, domains, stream sets and the rest — accepted
// POST/PATCH/DELETE from any member of the org regardless of role, so a
// Viewer could rewrite live routing through the API while the frontend
// merely hid the buttons.
//
// The invariant is structural, so the test is too: it parses every
// handler's Register function and fails on any POST/PATCH/PUT/DELETE that
// isn't wrapped in r.With(h.requireWrite). A unit test of the middleware
// itself could not have caught the original bug — the middleware worked
// fine, it just wasn't mounted. Adding a new mutating route without a
// gate now fails the build's own test run rather than waiting for the
// next audit.
//
// deliberatelyOpen is the explicit escape hatch. Every entry is a route
// that genuinely must stay reachable without a permission check; adding
// to it should be a conscious decision made in review, which is the point
// of requiring the edit here rather than allowing a silent omission.
var deliberatelyOpen = map[string]string{
	// Pre-session or personal-account actions: there is no session to
	// carry a role yet, or the action is the caller's own account.
	"auth.signup":             "pre-session",
	"auth.login":              "pre-session",
	"auth.verifyEmail":        "pre-session",
	"auth.resendVerification": "pre-session",
	"auth.acceptInvite":       "pre-session",
	"auth.logout":             "ends the caller's own session",
	"auth.loginTOTP":          "pre-session second factor",
	"auth.enroll":             "personal 2FA, every role manages its own",
	"auth.confirm":            "personal 2FA, every role manages its own",
	"auth.disable":            "personal 2FA, re-proves the caller's password",
	"alert.markRead":          "the caller's own read state; every role reads notifications",
	// POST-shaped reads: a JSON query body, no write.
	"analytics.report":          "read-only query, POST only for its body",
	"analytics.timeseries":      "read-only query, POST only for its body",
	"analytics.breakdown":       "read-only query, POST only for its body",
	"analytics.funnel":          "read-only query, POST only for its body",
	"analytics.dashboard":       "read-only query, POST only for its body",
	"analytics.costTimezones":   "read-only query, POST only for its body",
	"analytics.dimensionValues": "read-only query, POST only for its body",
	"routingsimulate.simulate":  "in-memory simulation, persists nothing",
	// Deliberate product decision, documented in the Referral phase:
	// requesting a payout is open to every role; approving/paying is not.
	"referral.requestPayout": "open to all roles by design; approve/pay are gated",
}

func TestEveryMutatingRouteIsPermissionGated(t *testing.T) {
	const internalDir = "../internal"
	mutating := map[string]bool{"Post": true, "Patch": true, "Put": true, "Delete": true}

	var ungated []string
	checked := 0

	err := filepath.WalkDir(internalDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		pkg := file.Name.Name

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !strings.HasPrefix(fn.Name.Name, "Register") || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || !mutating[sel.Sel.Name] || len(call.Args) != 2 {
					return true
				}
				checked++

				// The receiver is `r` for a bare route and `r.With(...)`
				// for a gated one — anything wrapped in a With() call is
				// considered gated, and named below for the report.
				handler := exprName(call.Args[1])
				key := pkg + "." + handler
				if isWithCall(sel.X) {
					return true
				}
				if _, ok := deliberatelyOpen[key]; ok {
					return true
				}
				ungated = append(ungated, key+"  ("+path+")")
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", internalDir, err)
	}

	if checked == 0 {
		t.Fatal("parsed no mutating routes at all — the walk or the AST matching is broken, not the code under test")
	}
	if len(ungated) > 0 {
		t.Fatalf("%d mutating route(s) reachable without a permission check.\n"+
			"Wrap each in r.With(h.requireWrite), or add it to deliberatelyOpen with a reason:\n  %s",
			len(ungated), strings.Join(ungated, "\n  "))
	}
	t.Logf("%d mutating routes checked, %d deliberately open", checked, len(deliberatelyOpen))
}

// isWithCall reports whether expr is a `<something>.With(...)` call — the
// chi idiom for attaching middleware to a single route.
func isWithCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "With"
}

// exprName renders the handler argument (`h.create`) as just its method
// name, so the map keys above read as package.method.
func exprName(e ast.Expr) string {
	if sel, ok := e.(*ast.SelectorExpr); ok {
		return sel.Sel.Name
	}
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return "?"
}
