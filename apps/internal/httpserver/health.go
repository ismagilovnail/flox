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
// Checks Postgres and ClickHouse. Not Redis: the API runs without it
// (rate limits degrade, nothing fails). ch is nil-able: a caller with no
// ClickHouse connection skips that check rather than reporting it
// failing.
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
