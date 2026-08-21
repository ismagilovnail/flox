package httpserver

import (
	"encoding/json"
	"net/http"
)

// healthHandler is liveness: "is the process up and able to handle HTTP at
// all." It never checks dependencies — that's readiness's job.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readyHandler is readiness: "can this instance actually serve traffic."
// Checks Postgres (since Phase 18) and ClickHouse (since Phase 25, once
// apps/api actually queries it — internal/analytics). Redis joins this
// check whenever a later phase wires apps/api to it, not before (a check
// that doesn't check anything is exactly the "fake API that looks real"
// CLAUDE.md forbids). ch is nil-able: not every caller of httpserver.New
// has a ClickHouse connection to offer, and a nil check is skipped rather
// than reported as failing.
func readyHandler(db Pinger, ch Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]string{"postgres": "ok"}
		status := http.StatusOK

		if err := db.Ping(r.Context()); err != nil {
			checks["postgres"] = err.Error()
			status = http.StatusServiceUnavailable
		}
		if ch != nil {
			if err := ch.Ping(r.Context()); err != nil {
				checks["clickhouse"] = err.Error()
				status = http.StatusServiceUnavailable
			} else {
				checks["clickhouse"] = "ok"
			}
		}

		body := map[string]any{"checks": checks}
		if status == http.StatusOK {
			body["status"] = "ok"
		} else {
			body["status"] = "unavailable"
		}
		writeJSON(w, status, body)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
