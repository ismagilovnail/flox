package googleads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
)

// DefaultDataManagerBaseURL is Google's Data Manager API, the way offline
// conversions reach Google Ads since Google closed UploadClickConversions
// to developer tokens that had not used it before 15 June 2026 (audit
// UNI-2 part 2, docs/offline-conversions.md). Written against the
// documented events:ingest shape and tested against a fake server only.
const DefaultDataManagerBaseURL = "https://datamanager.googleapis.com/v1"

// MaxEventsPerRequest is events:ingest's documented limit.
const MaxEventsPerRequest = 2000

// DataManager sends conversions to Google Ads through events:ingest. The
// refresh token it is given must have been minted with the
// https://www.googleapis.com/auth/datamanager scope — the Ads API scope
// alone is refused.
type DataManager struct {
	OAuthBaseURL string
	BaseURL      string
	HTTPClient   *http.Client
}

func NewDataManager() *DataManager {
	return &DataManager{OAuthBaseURL: DefaultOAuthBaseURL, BaseURL: DefaultDataManagerBaseURL, HTTPClient: defaultClient}
}

// Destination is where events go: the Google Ads account that owns the
// conversion action, the manager account the credentials reach it
// through (empty for none), and the conversion action's id.
type Destination struct {
	AccountID      string
	LoginAccountID string
	ActionID       string
}

// Event is one conversion. Exactly one of the click ids is set (the one
// the click came with); Value nil sends none. TransactionID identifies
// the conversion: sending it again with another value restates it.
type Event struct {
	GCLID, GBRAID, WBRAID string
	At                    time.Time
	TransactionID         string
	Value                 *float64
	Currency              string
}

// IngestError is a refusal by the API: Status is the HTTP status.
// Permanent is true when sending the same request again cannot succeed
// (400: a bad event, the whole request is refused) — as opposed to a
// rate limit, a server error, or missing access that the operator can fix.
type IngestError struct {
	Status    int
	Message   string
	Permanent bool
}

func (e *IngestError) Error() string {
	return fmt.Sprintf("googleads: data manager refused the events (%d): %s", e.Status, e.Message)
}

// IngestResult is a request Google took: its id, and the warnings it
// gave per field (the events were still taken).
type IngestResult struct {
	RequestID string
	Warnings  []string
}

type dmAccount struct {
	AccountType string `json:"accountType"`
	AccountID   string `json:"accountId"`
}

type dmDestination struct {
	OperatingAccount     dmAccount  `json:"operatingAccount"`
	LoginAccount         *dmAccount `json:"loginAccount,omitempty"`
	ProductDestinationID string     `json:"productDestinationId"`
}

type dmAdIdentifiers struct {
	GCLID  string `json:"gclid,omitempty"`
	GBRAID string `json:"gbraid,omitempty"`
	WBRAID string `json:"wbraid,omitempty"`
}

type dmEvent struct {
	AdIdentifiers   dmAdIdentifiers `json:"adIdentifiers"`
	EventTimestamp  string          `json:"eventTimestamp"`
	TransactionID   string          `json:"transactionId"`
	ConversionValue *float64        `json:"conversionValue,omitempty"`
	Currency        string          `json:"currency,omitempty"`
	EventSource     string          `json:"eventSource"`
}

type dmRequest struct {
	Destinations []dmDestination `json:"destinations"`
	Events       []dmEvent       `json:"events"`
	ValidateOnly bool            `json:"validateOnly,omitempty"`
}

type dmResponse struct {
	RequestID     string `json:"requestId"`
	FieldWarnings []struct {
		Field       string `json:"field"`
		Description string `json:"description"`
	} `json:"fieldWarnings"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func digits(accountID string) string { return strings.ReplaceAll(accountID, "-", "") }

// Ingest sends events to dest in one request (at most
// MaxEventsPerRequest). The API refuses a whole request when one event is
// invalid; the caller sends events one by one to find which.
func (d *DataManager) Ingest(ctx context.Context, creds adaccount.Credentials, dest Destination, events []Event, validateOnly bool) (IngestResult, error) {
	if len(events) == 0 || len(events) > MaxEventsPerRequest {
		return IngestResult{}, fmt.Errorf("googleads: %d events, want 1..%d", len(events), MaxEventsPerRequest)
	}
	client := d.HTTPClient
	if client == nil {
		client = defaultClient
	}
	oauthBaseURL, baseURL := d.OAuthBaseURL, d.BaseURL
	if oauthBaseURL == "" {
		oauthBaseURL = DefaultOAuthBaseURL
	}
	if baseURL == "" {
		baseURL = DefaultDataManagerBaseURL
	}

	accessToken, err := refreshAccessToken(ctx, client, oauthBaseURL, creds)
	if err != nil {
		// A refused refresh token is the operator's to fix; not a bad event.
		return IngestResult{}, &IngestError{Status: http.StatusUnauthorized, Message: err.Error()}
	}

	destination := dmDestination{
		OperatingAccount:     dmAccount{AccountType: "GOOGLE_ADS", AccountID: digits(dest.AccountID)},
		ProductDestinationID: dest.ActionID,
	}
	if dest.LoginAccountID != "" {
		destination.LoginAccount = &dmAccount{AccountType: "GOOGLE_ADS", AccountID: digits(dest.LoginAccountID)}
	}
	body := dmRequest{Destinations: []dmDestination{destination}, ValidateOnly: validateOnly}
	for _, e := range events {
		body.Events = append(body.Events, dmEvent{
			AdIdentifiers:   dmAdIdentifiers{GCLID: e.GCLID, GBRAID: e.GBRAID, WBRAID: e.WBRAID},
			EventTimestamp:  e.At.UTC().Format(time.RFC3339),
			TransactionID:   e.TransactionID,
			ConversionValue: e.Value,
			Currency:        e.Currency,
			EventSource:     "WEB",
		})
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return IngestResult{}, fmt.Errorf("googleads: encoding events: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/events:ingest", bytes.NewReader(raw))
	if err != nil {
		return IngestResult{}, fmt.Errorf("googleads: building ingest request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return IngestResult{}, fmt.Errorf("googleads: calling events:ingest: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return IngestResult{}, fmt.Errorf("googleads: reading ingest response: %w", err)
	}
	var parsed dmResponse
	decodeErr := json.Unmarshal(data, &parsed)

	if resp.StatusCode/100 != 2 {
		msg := strings.TrimSpace(string(data))
		if decodeErr == nil && parsed.Error != nil {
			msg = parsed.Error.Message
			if parsed.Error.Status != "" {
				msg = parsed.Error.Status + ": " + msg
			}
		}
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return IngestResult{}, &IngestError{Status: resp.StatusCode, Message: msg, Permanent: resp.StatusCode == http.StatusBadRequest}
	}
	if decodeErr != nil {
		return IngestResult{}, fmt.Errorf("googleads: decoding ingest response (status %d): %w", resp.StatusCode, decodeErr)
	}
	out := IngestResult{RequestID: parsed.RequestID}
	for _, w := range parsed.FieldWarnings {
		out.Warnings = append(out.Warnings, strings.TrimSpace(w.Field+": "+w.Description))
	}
	return out, nil
}

// IsPermanent: err is a refusal that sending again cannot fix.
func IsPermanent(err error) bool {
	var ie *IngestError
	return errors.As(err, &ie) && ie.Permanent
}
