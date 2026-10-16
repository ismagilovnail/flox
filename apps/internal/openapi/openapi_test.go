package openapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/openapi"
)

func mustSpec(t *testing.T) map[string]any {
	t.Helper()
	doc, err := openapi.Spec()
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

var pathParam = regexp.MustCompile(`\{([^}]+)\}`)

// The document itself: every operation named, scoped and answered; every
// path parameter declared; every schema in the supported subset with its
// references resolving.
func TestSpecIsWellFormed(t *testing.T) {
	doc := mustSpec(t)
	if doc["openapi"] != "3.1.0" {
		t.Fatalf("openapi = %v", doc["openapi"])
	}
	v := openapi.NewValidator(doc)
	ops := openapi.Operations(doc)
	if len(ops) < 50 {
		t.Fatalf("only %d operations — paths no longer parse?", len(ops))
	}
	ids := map[string]string{}
	for _, op := range ops {
		at := op.Method + " " + op.Path
		if op.ID == "" {
			t.Errorf("%s: no operationId", at)
		} else if prev, dup := ids[op.ID]; dup {
			t.Errorf("%s: operationId %q also on %s", at, op.ID, prev)
		}
		ids[op.ID] = at
		if op.Scope != "read" && op.Scope != "write" {
			t.Errorf("%s: x-flox-scope = %q, want read or write", at, op.Scope)
		}
		if op.Method == http.MethodGet && op.Scope != "read" {
			t.Errorf("%s: a GET needs only the read scope", at)
		}
		if summary, _ := op.Spec["summary"].(string); summary == "" {
			t.Errorf("%s: no summary", at)
		}
		if tags, _ := op.Spec["tags"].([]any); len(tags) != 1 {
			t.Errorf("%s: want exactly one tag, got %v", at, tags)
		}
		responses, _ := op.Spec["responses"].(map[string]any)
		success := 0
		for code := range responses {
			if strings.HasPrefix(code, "2") {
				success++
			}
		}
		if success != 1 || responses["default"] == nil {
			t.Errorf("%s: want one 2xx response and a default, got %v", at, responses)
		}

		declared := map[string]bool{}
		item, _ := doc["paths"].(map[string]any)[op.Path].(map[string]any)
		for _, list := range []any{item["parameters"], op.Spec["parameters"]} {
			params, _ := list.([]any)
			for _, p := range params {
				pm, _ := p.(map[string]any)
				if ref, ok := pm["$ref"].(string); ok {
					var err error
					if pm, err = v.Resolve(ref); err != nil {
						t.Errorf("%s: %v", at, err)
						continue
					}
				}
				if pm["in"] == "path" {
					declared[pm["name"].(string)] = true
				}
			}
		}
		for _, m := range pathParam.FindAllStringSubmatch(op.Path, -1) {
			if !declared[m[1]] {
				t.Errorf("%s: path parameter {%s} not declared", at, m[1])
			}
		}

		if s := op.RequestSchema(); s != nil {
			for _, e := range v.CheckSchema(s) {
				t.Errorf("%s request: %s", at, e)
			}
		}
		for code := range responses {
			r, _ := responses[code].(map[string]any)
			if ref, ok := r["$ref"].(string); ok {
				if _, err := v.Resolve(ref); err != nil {
					t.Errorf("%s %s: %v", at, code, err)
				}
				continue
			}
			content, _ := r["content"].(map[string]any)
			media, _ := content["application/json"].(map[string]any)
			if s, ok := media["schema"].(map[string]any); ok {
				for _, e := range v.CheckSchema(s) {
					t.Errorf("%s %s: %s", at, code, e)
				}
			} else if code != "204" {
				t.Errorf("%s %s: no JSON schema", at, code)
			}
		}
	}
	schemas, _ := doc["components"].(map[string]any)["schemas"].(map[string]any)
	for name, s := range schemas {
		for _, e := range v.CheckSchema(s.(map[string]any)) {
			t.Errorf("components.schemas.%s: %s", name, e)
		}
	}
}

// The served documents carry this server's address and are the same spec
// in both encodings.
func TestHandlerServesBothEncodings(t *testing.T) {
	h, err := openapi.NewHandler("https://api.flox.test/v1/")
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	h.Register(r)

	get := func(path string) (*http.Response, string) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		body, _ := io.ReadAll(rec.Result().Body)
		return rec.Result(), string(body)
	}
	res, body := get("/openapi.json")
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("openapi.json = %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	var doc struct {
		Servers []struct{ URL string } `json:"servers"`
		Paths   map[string]any         `json:"paths"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Servers) != 1 || doc.Servers[0].URL != "https://api.flox.test/v1" {
		t.Fatalf("servers = %+v", doc.Servers)
	}
	if len(doc.Paths) == 0 {
		t.Fatal("no paths in the JSON document")
	}
	res, body = get("/openapi.yaml")
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "url: https://api.flox.test/v1") {
		t.Fatalf("openapi.yaml = %d, servers missing", res.StatusCode)
	}

	rel, err := openapi.NewHandler("")
	if err != nil {
		t.Fatal(err)
	}
	r = chi.NewRouter()
	rel.Register(r)
	if _, body = get("/openapi.json"); !strings.Contains(body, `"url": "/v1"`) {
		t.Fatal("without a public address the server should be the relative /v1")
	}
}

func TestValidator(t *testing.T) {
	doc := map[string]any{"components": map[string]any{"schemas": map[string]any{
		"Thing": map[string]any{
			"type": "object", "additionalProperties": false, "required": []any{"id", "when"},
			"properties": map[string]any{
				"id":    map[string]any{"type": "string", "minLength": 2},
				"when":  map[string]any{"type": "string", "format": "date-time"},
				"count": map[string]any{"type": []any{"integer", "null"}, "minimum": 0},
				"kind":  map[string]any{"type": "string", "enum": []any{"a", "b"}},
				"tags":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 2},
				"next":  map[string]any{"anyOf": []any{map[string]any{"$ref": "#/components/schemas/Thing"}, map[string]any{"type": "null"}}},
			},
		},
	}}}
	v := openapi.NewValidator(doc)
	thing, err := v.Resolve("#/components/schemas/Thing")
	if err != nil {
		t.Fatal(err)
	}
	decode := func(s string) any {
		var x any
		if err := json.Unmarshal([]byte(s), &x); err != nil {
			t.Fatal(err)
		}
		return x
	}
	for body, wantErrs := range map[string]int{
		`{"id":"ab","when":"2026-10-16T10:00:00Z"}`:                                                  0,
		`{"id":"ab","when":"2026-10-16T10:00:00Z","count":null,"kind":"a","tags":["x"]}`:             0,
		`{"id":"ab","when":"2026-10-16T10:00:00Z","next":{"id":"cd","when":"2026-10-16T10:00:00Z"}}`: 0,
		`{"id":"a","when":"yesterday"}`:                                                              2,
		`{"id":"ab"}`:                                                                                1,
		`{"id":"ab","when":"2026-10-16T10:00:00Z","extra":1}`:                                        1,
		`{"id":"ab","when":"2026-10-16T10:00:00Z","count":1.5}`:                                      1,
		`{"id":"ab","when":"2026-10-16T10:00:00Z","count":-1}`:                                       1,
		`{"id":"ab","when":"2026-10-16T10:00:00Z","kind":"c"}`:                                       1,
		`{"id":"ab","when":"2026-10-16T10:00:00Z","tags":["x","y","z"]}`:                             1,
		`{"id":"ab","when":"2026-10-16T10:00:00Z","next":{"id":"cd"}}`:                               1,
		`[]`: 1,
	} {
		if errs := v.Validate(thing, decode(body)); len(errs) != wantErrs {
			t.Errorf("%s: want %d errors, got %v", body, wantErrs, errs)
		}
	}
	if errs := v.CheckSchema(map[string]any{"type": "string", "oneOf": []any{}}); len(errs) != 1 {
		t.Errorf("an unsupported keyword must be reported, got %v", errs)
	}
	if errs := v.CheckSchema(map[string]any{"$ref": "#/components/schemas/Missing"}); len(errs) != 1 {
		t.Errorf("an unresolved reference must be reported, got %v", errs)
	}
}
