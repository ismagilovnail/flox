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
// Checks Postgres, the one dependency that exists as of Phase 18 —
// ClickHouse/Redis join this check whenever a later phase actually wires
// them in, not before (a check that doesn't check anything is exactly the
// "fake API that looks real" CLAUDE.md forbids).
func readyHandler(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]string{"postgres": "ok"}
		status := http.StatusOK

		if err := db.Ping(r.Context()); err != nil {
			checks["postgres"] = err.Error()
			status = http.StatusServiceUnavailable
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
