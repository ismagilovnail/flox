package pixelsend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/audit"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/macro"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
	"github.com/ismagilovnail/flox/apps/internal/urlsafety"
)

// A test event (docs/pixels.md "Test event"): an operator sends one made-up
// event to a pixel now, from the pixel's page, to see the platform take it.
// It goes through the same Sender as real deliveries, synchronously (an
// operator action in apps/api, not a request path — CLAUDE.md #9), and is
// written to the pixel's delivery log already finished, so the worker
// never picks it up.
//
// Facebook and TikTok with a test event code receive it in their test tool
// only. Anywhere else — no code, Snapchat (which has none), generic — it is
// a real event the platform counts, and is sent only when the operator
// confirms that (ConfirmReal).

// TestEventPrefix starts a test event's click id and event id — each send
// gets new ones, so the platform does not deduplicate repeats.
const TestEventPrefix = "flox-test-"

// testValueUSD is what a test conversion is worth: Facebook refuses a
// Purchase without a value.
const testValueUSD = 1.0

// TestPixels is the slice of pixel.Repository a test send reads.
type TestPixels interface {
	GetByID(ctx context.Context, orgID, id string) (pixel.Pixel, error)
	Credentials(ctx context.Context, orgID, id string) (pixel.Credentials, error)
}

// TestLog writes a test send to the pixel's delivery log, finished.
type TestLog interface {
	RecordTest(ctx context.Context, orgID string, ev pixel.Event, pixelID string, out Outcome) (string, error)
}

// Tester sends test events.
type Tester struct {
	pixels  TestPixels
	senders map[pixel.Provider]Sender
	log     TestLog
	now     func() time.Time
}

func NewTester(pixels TestPixels, senders map[pixel.Provider]Sender, log TestLog) *Tester {
	return &Tester{pixels: pixels, senders: senders, log: log, now: time.Now}
}

// TestInput is a test send. EventType "" is the pixel's first event; IP
// and UserAgent are the operator's own (Facebook needs both for a website
// event).
type TestInput struct {
	EventType   string
	ConfirmReal bool
	IP          string
	UserAgent   string
}

// TestResult is how the platform answered.
type TestResult struct {
	DeliveryID string `json:"deliveryId"`
	EventType  string `json:"eventType"`
	ClickID    string `json:"clickId"`
	Delivered  bool   `json:"delivered"`
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
	// TestMode: the platform's test tool got it (a test event code is set),
	// not the ad account's statistics.
	TestMode bool      `json:"testMode"`
	SentAt   time.Time `json:"sentAt"`
}

// InTestMode reports whether p's events land in the platform's test tool
// rather than its statistics.
func InTestMode(p pixel.Pixel) bool {
	return (p.Provider == pixel.ProviderFacebook || p.Provider == pixel.ProviderTikTok) && p.TestEventCode != ""
}

// Send sends one test event to pixelID. A pixel that would count it as
// real needs in.ConfirmReal; the event type must be one the pixel reports.
// A platform's refusal is a result (Delivered false), not an error.
func (t *Tester) Send(ctx context.Context, orgID, pixelID string, in TestInput) (TestResult, error) {
	p, err := t.pixels.GetByID(ctx, orgID, pixelID)
	if err != nil {
		return TestResult{}, err
	}
	eventType := in.EventType
	if eventType == "" && len(p.Events) > 0 {
		eventType = p.Events[0]
	}
	if !slices.Contains(p.Events, eventType) {
		return TestResult{}, apierror.Validation("invalid test event", map[string]string{"eventType": "must be one of the events this pixel reports"})
	}
	testMode := InTestMode(p)
	if !testMode && !in.ConfirmReal {
		return TestResult{}, apierror.Validation("this test event would be real", map[string]string{
			"confirmReal": fmt.Sprintf("%s counts this event as real (no test event code): confirm to send it", p.Provider),
		})
	}
	creds, err := t.pixels.Credentials(ctx, orgID, pixelID)
	if err != nil {
		return TestResult{}, err
	}
	sender, ok := t.senders[creds.Provider]
	if !ok {
		return TestResult{}, fmt.Errorf("pixelsend: no sender for provider %q", creds.Provider)
	}

	now := t.now().UTC()
	id := TestEventPrefix + idgen.New()
	ev := pixel.Event{
		OrganizationID: orgID,
		SourceEventID:  id,
		Type:           event.Type(eventType),
		EventAt:        now,
		ClickID:        id,
		ClickAt:        now.Add(-time.Minute),
		IP:             in.IP,
		UserAgent:      in.UserAgent,
		Macros:         macro.Values{ClickID: id},
	}
	if ev.Type.IsCPA() {
		value := testValueUSD
		ev.Revenue, ev.Currency, ev.USDValue = &value, "USD", &value
	}

	sendCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	out := sender.Send(sendCtx, creds, ev)
	cancel()

	deliveryID, err := t.log.RecordTest(context.WithoutCancel(ctx), orgID, ev, pixelID, out)
	if err != nil {
		return TestResult{}, err
	}
	res := TestResult{
		DeliveryID: deliveryID, EventType: eventType, ClickID: id, Delivered: out.Err == nil,
		StatusCode: out.StatusCode, TestMode: testMode, SentAt: now,
	}
	if out.Err != nil {
		res.Message = out.Err.Error()
	}
	return res, nil
}

// RecordTest writes a test send to the delivery log as a finished row
// (success or dead, one attempt). The visitor fields — here the operator's
// own IP and user agent — are not kept.
func (s *PostgresStore) RecordTest(ctx context.Context, orgID string, ev pixel.Event, pixelID string, out Outcome) (string, error) {
	ev.IP, ev.UserAgent = "", ""
	payload, err := json.Marshal(ev)
	if err != nil {
		return "", fmt.Errorf("pixelsend: encoding test event: %w", err)
	}
	status, message := StatusSuccess, "test event"
	if out.Err != nil {
		status, message = StatusDead, "test event: "+out.Err.Error()
	}
	id := idgen.New()
	_, err = s.db.Exec(ctx, `
		INSERT INTO `+s.table+` (id, organization_id, pixel_id, source_event_id, event_type, click_id, payload,
		                       delivery_status, attempt_count, response_status_code, message)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, $9, $10)`,
		id, orgID, pixelID, ev.SourceEventID, string(ev.Type), ev.ClickID, payload, string(status), nullableInt(out.StatusCode), truncate(message, 500))
	if err != nil {
		return "", fmt.Errorf("pixelsend: recording test event: %w", err)
	}
	return id, nil
}

// NewSafeClient is the HTTP client for sends from apps/api: the same
// dial-time SSRF guard as the worker's (a generic pixel's URL is
// operator-supplied), bounded by attemptTimeout.
func NewSafeClient() *http.Client {
	return &http.Client{
		Timeout: attemptTimeout,
		Transport: &http.Transport{
			DialContext:         urlsafety.SafeDialContext,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}
}

// TestHandler serves POST /pixels/{id}/test-event (pixel write).
type TestHandler struct {
	tester       *Tester
	requireWrite func(http.Handler) http.Handler
	clientIP     func(*http.Request) net.IP
	logger       *slog.Logger
}

func NewTestHandler(tester *Tester, requireWrite func(http.Handler) http.Handler, clientIP func(*http.Request) net.IP, logger *slog.Logger) *TestHandler {
	return &TestHandler{tester: tester, requireWrite: requireWrite, clientIP: clientIP, logger: logger}
}

// Register mounts the route on the /pixels router (tenant middleware
// already applied).
func (h *TestHandler) Register(r chi.Router) {
	r.With(h.requireWrite).Post("/{id}/test-event", h.send)
}

type testRequest struct {
	EventType   string `json:"eventType"`
	ConfirmReal bool   `json:"confirmReal"`
}

func (h *TestHandler) send(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req testRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
			apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
			return
		}
	}
	in := TestInput{EventType: req.EventType, ConfirmReal: req.ConfirmReal, UserAgent: r.UserAgent()}
	if ip := h.clientIP(r); ip != nil {
		in.IP = ip.String()
	}
	id := chi.URLParam(r, "id")
	res, err := h.tester.Send(r.Context(), orgID, id, in)
	if err != nil {
		var apiErr *apierror.Error
		if !errors.As(err, &apiErr) {
			h.logger.Error("pixel test event", "pixel_id", id, "error", err)
		}
		apierror.Write(w, h.logger, err)
		return
	}
	audit.Annotate(r.Context(), audit.Entry{Action: "pixel.test_event_sent", EntityType: "pixel", EntityID: id})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

var _ TestLog = (*PostgresStore)(nil)
