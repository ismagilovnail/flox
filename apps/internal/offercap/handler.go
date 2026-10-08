package offercap

import (
	"encoding/json"
	"net/http"

	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

type capResponse struct {
	OfferID string `json:"offerId"`
	Cap     int    `json:"cap"`
	Count   uint64 `json:"count"`
	Reached bool   `json:"reached"`
}

// Handler answers GET /offer-caps: today's conversion count of each of the
// caller's active capped offers, from the snapshot routing uses — so the
// offers list shows "cap reached" exactly when routing stops sending
// traffic. available is false when there is no snapshot to read
// (ClickHouse down at startup); the list then shows no counts rather than
// zeros. The caller applies tenant.Middleware first.
func Handler(m *Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, _ := tenant.OrgID(r.Context())
		caps := []capResponse{}
		for _, s := range m.StatusesFor(orgID) {
			caps = append(caps, capResponse{OfferID: s.OfferID, Cap: s.Cap, Count: s.Count, Reached: s.Reached()})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"caps": caps, "available": m != nil})
	}
}
