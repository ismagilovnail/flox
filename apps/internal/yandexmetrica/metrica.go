// Package yandexmetrica is the slice of Yandex Metrica's Management API
// FLOX uses: offline conversions by yclid (audit UNI-2 part 3,
// docs/offline-conversions.md). Metrica attaches each one to the visit the
// yclid came with and passes it on to Yandex Direct.
//
// Written against the documented requests (management/offline-conv,
// openapi offline_conversions upload/findById) and tested against a fake
// server only: there is no Metrica counter here.
package yandexmetrica

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/httpclient"
)

const DefaultBaseURL = "https://api-metrika.yandex.net"

// MaxConversionsPerUpload bounds one file. Metrica documents only a 1 GB
// file limit; smaller files keep a refused one cheap to split.
const MaxConversionsPerUpload = 5000

// VisitWindow is how long after a visit Metrica still attaches offline
// data to it ("период дополнения визитов составляет 21 день").
const VisitWindow = 21 * 24 * time.Hour

// Upload statuses. PROCESSED and LINKAGE_FAILURE are final; LINKAGE_FAILURE:
// no row of the file matched a visitor.
const (
	StatusProcessed      = "PROCESSED"
	StatusLinkageFailure = "LINKAGE_FAILURE"
)

// Final reports whether an upload's status will not change any more.
func Final(status string) bool { return status == StatusProcessed || status == StatusLinkageFailure }

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func New() *Client {
	return &Client{BaseURL: DefaultBaseURL, HTTPClient: httpclient.New(httpclient.AdAPI)}
}

// Conversion is one row of the file. Price nil sends none (Metrica then
// counts the goal without revenue); Currency goes with a price only.
type Conversion struct {
	Yclid    string
	Goal     string
	At       time.Time
	Price    *float64
	Currency string
}

// Uploading is Metrica's record of a file: its id, status, how many rows
// the file had and how many passed validation.
type Uploading struct {
	ID             int64  `json:"id"`
	Status         string `json:"status"`
	SourceQuantity int    `json:"source_quantity"`
	LineQuantity   int    `json:"line_quantity"`
}

// Error is a refusal by the API. Permanent: sending the same request
// again cannot succeed (400: the file or a parameter is invalid) — as
// opposed to a refused token, missing access, a quota or a server error,
// which the operator or time can fix.
type Error struct {
	Status    int
	Message   string
	Permanent bool
}

func (e *Error) Error() string {
	return fmt.Sprintf("yandexmetrica: refused (%d): %s", e.Status, e.Message)
}

// IsPermanent: err is a refusal that sending again cannot fix.
func IsPermanent(err error) bool {
	var me *Error
	return errors.As(err, &me) && me.Permanent
}

// IsAuth: err is Metrica refusing the token or its access to the counter.
func IsAuth(err error) bool {
	var me *Error
	return errors.As(err, &me) && (me.Status == http.StatusUnauthorized || me.Status == http.StatusForbidden)
}

// Counter checks that token reaches the counter — called when the
// settings are saved, so a wrong number or token shows at once.
func (c *Client) Counter(ctx context.Context, token, counterID string) error {
	_, err := c.do(ctx, token, http.MethodGet, "/management/v1/counter/"+url.PathEscape(counterID), "", nil)
	return err
}

// Upload sends conversions to the counter in one CSV file (yclid
// identifiers). Metrica processes it within about three hours; Check
// tells how it went.
func (c *Client) Upload(ctx context.Context, token, counterID string, conversions []Conversion) (Uploading, error) {
	if len(conversions) == 0 || len(conversions) > MaxConversionsPerUpload {
		return Uploading{}, fmt.Errorf("yandexmetrica: %d conversions, want 1..%d", len(conversions), MaxConversionsPerUpload)
	}
	file, err := encodeCSV(conversions)
	if err != nil {
		return Uploading{}, err
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "conversions.csv")
	if err != nil {
		return Uploading{}, fmt.Errorf("yandexmetrica: building upload: %w", err)
	}
	if _, err := part.Write(file); err != nil {
		return Uploading{}, fmt.Errorf("yandexmetrica: building upload: %w", err)
	}
	if err := mw.Close(); err != nil {
		return Uploading{}, fmt.Errorf("yandexmetrica: building upload: %w", err)
	}
	data, err := c.do(ctx, token, http.MethodPost,
		"/management/v1/counter/"+url.PathEscape(counterID)+"/offline_conversions/upload", mw.FormDataContentType(), &body)
	if err != nil {
		return Uploading{}, err
	}
	return decodeUploading(data)
}

// Check reads an upload's status.
func (c *Client) Check(ctx context.Context, token, counterID string, uploadingID int64) (Uploading, error) {
	data, err := c.do(ctx, token, http.MethodGet,
		"/management/v1/counter/"+url.PathEscape(counterID)+"/offline_conversions/uploading/"+strconv.FormatInt(uploadingID, 10), "", nil)
	if err != nil {
		return Uploading{}, err
	}
	return decodeUploading(data)
}

// encodeCSV is the documented file: a header row, then Yclid, Target,
// DateTime (Unix seconds), Price (a dot as the decimal separator) and
// Currency (ISO 4217).
func encodeCSV(conversions []Conversion) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"Yclid", "Target", "DateTime", "Price", "Currency"})
	for _, cv := range conversions {
		price, currency := "", ""
		if cv.Price != nil {
			price, currency = strconv.FormatFloat(*cv.Price, 'f', 2, 64), cv.Currency
		}
		_ = w.Write([]string{cv.Yclid, cv.Goal, strconv.FormatInt(cv.At.Unix(), 10), price, currency})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("yandexmetrica: encoding conversions: %w", err)
	}
	return buf.Bytes(), nil
}

func decodeUploading(data []byte) (Uploading, error) {
	var parsed struct {
		Uploading *Uploading `json:"uploading"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil || parsed.Uploading == nil {
		return Uploading{}, fmt.Errorf("yandexmetrica: unexpected answer: %.200s", data)
	}
	return *parsed.Uploading, nil
}

// errorBody is the documented error answer.
type errorBody struct {
	Errors []struct {
		ErrorType string `json:"error_type"`
		Message   string `json:"message"`
	} `json:"errors"`
	Message string `json:"message"`
}

func (c *Client) do(ctx context.Context, token, method, path, contentType string, body io.Reader) ([]byte, error) {
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	client := c.HTTPClient
	if client == nil {
		client = httpclient.New(httpclient.AdAPI)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("yandexmetrica: building request: %w", err)
	}
	req.Header.Set("Authorization", "OAuth "+token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yandexmetrica: calling %s: %w", path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("yandexmetrica: reading the answer: %w", err)
	}
	if resp.StatusCode/100 == 2 {
		return data, nil
	}
	msg := strings.TrimSpace(string(data))
	var parsed errorBody
	if json.Unmarshal(data, &parsed) == nil {
		var parts []string
		for _, e := range parsed.Errors {
			parts = append(parts, strings.TrimSpace(e.ErrorType+": "+e.Message))
		}
		if len(parts) > 0 {
			msg = strings.Join(parts, "; ")
		} else if parsed.Message != "" {
			msg = parsed.Message
		}
	}
	if len(msg) > 500 {
		msg = msg[:500]
	}
	return nil, &Error{Status: resp.StatusCode, Message: msg, Permanent: resp.StatusCode == http.StatusBadRequest}
}
