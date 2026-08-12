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
// No dependency checks yet because there are no dependencies to check —
// Postgres/ClickHouse/Redis connections don't exist until Phase 17+ wires
// them up, at which point this handler starts pinging each one and can
// return 503 on a failed check instead of always 200.
func readyHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "checks": map[string]string{}})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
