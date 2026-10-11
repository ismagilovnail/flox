package pixel

import (
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/macro"
)

// Event is one FLOX event to report to the pixels of the stream set its
// click went through — queued once per matching pixel (internal/pixelsend)
// and stored as that delivery's payload, so the send works from what was
// known when the event happened.
type Event struct {
	OrganizationID string `json:"organizationId"`
	StreamSetID    string `json:"streamSetId"`
	// SourceEventID is the event's own id: the postbacks row of a
	// conversion, the tracker's id of a PWA_INSTALL. Sent as the platform's
	// event_id, which the platform deduplicates on.
	SourceEventID string     `json:"sourceEventId"`
	Type          event.Type `json:"type"`
	EventAt       time.Time  `json:"eventAt"`

	ClickID string    `json:"clickId"`
	ClickAt time.Time `json:"clickAt"`

	// Revenue/Currency as the network reported them; USDValue at the rate
	// of the event's date (§50-FX), nil without one. A platform gets a value
	// only when one is known — never a made-up 0.
	Revenue  *float64 `json:"revenue,omitempty"`
	Currency string   `json:"currency,omitempty"`
	USDValue *float64 `json:"usdValue,omitempty"`

	// Matching data: the visitor's IP and user agent and the platform's own
	// click id, as the click carried them. Sent only to the platform, never
	// logged (§54).
	IP        string `json:"ip,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
	FBClickID string `json:"fbClickId,omitempty"`
	TTClickID string `json:"ttClickId,omitempty"`

	// Macros fill the generic provider's URL template.
	Macros macro.Values `json:"macros"`
}
