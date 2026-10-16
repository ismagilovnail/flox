package openapi

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// Operation is one method on one path of the spec.
type Operation struct {
	Method string // GET, POST, …
	Path   string // relative to /v1, e.g. /campaigns/{id}
	ID     string // operationId
	Scope  string // x-flox-scope: the API key scope it needs
	Spec   map[string]any
}

var methods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

// Operations lists every operation, sorted by path then method.
func Operations(doc map[string]any) []Operation {
	paths, _ := doc["paths"].(map[string]any)
	var out []Operation
	for path, item := range paths {
		pi, _ := item.(map[string]any)
		for _, m := range methods {
			op, ok := pi[strings.ToLower(m)].(map[string]any)
			if !ok {
				continue
			}
			id, _ := op["operationId"].(string)
			scope, _ := op["x-flox-scope"].(string)
			out = append(out, Operation{Method: m, Path: path, ID: id, Scope: scope, Spec: op})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}

// RequestSchema is the operation's JSON request body schema, nil without one.
func (o Operation) RequestSchema() map[string]any {
	rb, _ := o.Spec["requestBody"].(map[string]any)
	return jsonSchema(rb)
}

// ResponseSchema is the JSON schema documented for status — exactly that
// code, never `default`. ok is false when the code is not documented;
// schema is nil for a documented response without a body (204).
func (o Operation) ResponseSchema(status int) (schema map[string]any, ok bool) {
	responses, _ := o.Spec["responses"].(map[string]any)
	r, ok := responses[fmt.Sprint(status)].(map[string]any)
	if !ok {
		return nil, false
	}
	return jsonSchema(r), true
}

func jsonSchema(holder map[string]any) map[string]any {
	content, _ := holder["content"].(map[string]any)
	media, _ := content["application/json"].(map[string]any)
	schema, _ := media["schema"].(map[string]any)
	return schema
}
