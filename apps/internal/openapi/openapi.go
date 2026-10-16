// Package openapi serves the public API's reference (audit 2026-10-15,
// API-1): openapi.yaml, embedded in the binary, at GET /v1/openapi.json and
// /v1/openapi.yaml. No session or key is needed to read it.
//
// The spec is written by hand and held to the code by apps/api's tests:
// every operation in it exists in the router, every route of the router is
// in it or listed as left out on purpose, and every operation is called
// with an API key and its answer checked against the schema
// (apps/api/openapi_test.go). docs/public-api.md has the workflow.
package openapi

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.yaml.in/yaml/v3"
)

//go:embed openapi.yaml
var specYAML []byte

// Spec is the parsed document, the way encoding/json reads it.
func Spec() (map[string]any, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(specYAML, &doc); err != nil {
		return nil, fmt.Errorf("openapi: parsing openapi.yaml: %w", err)
	}
	return doc, nil
}

// Handler answers the two routes. serverURL is the API's public address
// with its version prefix (https://api.example.com/v1); empty keeps the
// relative "/v1", which a client resolves against where it read the spec.
type Handler struct {
	json, yaml []byte
}

// NewHandler renders both documents once, with servers set to serverURL.
func NewHandler(serverURL string) (*Handler, error) {
	doc, err := Spec()
	if err != nil {
		return nil, err
	}
	if serverURL == "" {
		serverURL = "/v1"
	}
	doc["servers"] = []any{map[string]any{"url": strings.TrimSuffix(serverURL, "/")}}
	j, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("openapi: rendering json: %w", err)
	}
	y, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("openapi: rendering yaml: %w", err)
	}
	return &Handler{json: j, yaml: y}, nil
}

// Register mounts GET /openapi.json and /openapi.yaml.
func (h *Handler) Register(r chi.Router) {
	r.Get("/openapi.json", h.serve(h.json, "application/json"))
	r.Get("/openapi.yaml", h.serve(h.yaml, "application/yaml"))
}

func (h *Handler) serve(body []byte, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(body)
	}
}
