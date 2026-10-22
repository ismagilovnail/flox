package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/errtrack"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

const leakySecret = "s3cret-THAT-MUST-NOT-LEAK"

type failingNetworks struct{ err error }

func (f failingNetworks) ByID(context.Context, string) (conversion.Network, error) {
	return conversion.Network{}, f.err
}

type panickingNetworks struct{}

func (panickingNetworks) ByID(context.Context, string) (conversion.Network, error) {
	panic("network lookup exploded")
}

type failingMapper struct{}

func (failingMapper) MapStatus(context.Context, string, string, string) (event.Type, error) {
	return "", errors.New("mapping store down")
}

type capturedReports struct {
	mu   sync.Mutex
	text []string
}

func (c *capturedReports) Report(msg string, err error, attrs map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.text = append(c.text, fmt.Sprint(msg, err, attrs))
}

func (c *capturedReports) Flush(time.Duration) {}

// Audit SEC-3: a network's incoming postback carries its secret in the
// query. Every branch of the postback handler that logs — a failed
// network lookup, a failed conversion write, a panic caught by errtrack —
// leaves the secret out of the log and out of the error report. (The
// tracker writes no per-request log line at all; the edge masks the
// secret in its own logs, infra/Caddyfile.tracker.)
func TestPostbackSecretNeverLogged(t *testing.T) {
	networkID := idgen.New()
	network := conversion.Network{ID: networkID, OrganizationID: idgen.New(), PostbackSecretHash: hashFor(leakySecret)}

	for name, h := range map[string]*PostbackHandler{
		"network lookup fails": {networks: failingNetworks{errors.New("postgres down")}},
		"recording fails": {networks: staticNetworks{network},
			service: conversion.NewService(failingMapper{}, nil, nil, nil, nil)},
		"handler panics": {networks: panickingNetworks{}},
	} {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			reports := &capturedReports{}
			h.logger = logger
			r := chi.NewRouter()
			r.Use(errtrack.Middleware(reports, logger))
			h.Register(r)

			for _, req := range []*http.Request{
				httptest.NewRequest(http.MethodGet, "/postback/"+networkID+"?secret="+leakySecret+"&click_id=c1&status=sale", nil),
				httptest.NewRequest(http.MethodPost, "/postback/"+networkID, strings.NewReader("secret="+leakySecret+"&click_id=c1&status=sale")),
			} {
				if req.Method == http.MethodPost {
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				}
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, req)
				if rec.Code < 500 {
					t.Fatalf("%s: status %d, want a server error (the logging branch)", req.Method, rec.Code)
				}
				if strings.Contains(rec.Body.String(), leakySecret) {
					t.Errorf("%s: the answer carries the secret: %s", req.Method, rec.Body.String())
				}
			}
			if logs.Len() == 0 {
				t.Fatal("nothing was logged: the branch under test did not run")
			}
			if strings.Contains(logs.String(), leakySecret) {
				t.Errorf("the secret is in the log:\n%s", logs.String())
			}
			for _, rep := range reports.text {
				if strings.Contains(rep, leakySecret) {
					t.Errorf("the secret is in an error report: %s", rep)
				}
			}
		})
	}
}
