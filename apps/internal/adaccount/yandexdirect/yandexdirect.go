// Package yandexdirect implements adaccount.CostProvider against the
// Yandex Direct API (audit UNI-3 part 1; §74, §27-COST): the account's
// currency from the Clients service, daily spend per campaign from the
// Reports service (CAMPAIGN_PERFORMANCE_REPORT). Written against the
// documented requests and tested against a fake server only — there is
// no Direct account here (docs/ad-account-connections.md "Yandex Direct").
//
// Credentials: an OAuth token (AccessToken) and the advertiser's Direct
// login (AdAccountID), sent as Client-Login — required when the token is
// an agency's, harmless for the advertiser's own.
package yandexdirect

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/httpclient"
)

const DefaultBaseURL = "https://api.direct.yandex.com"

const dateLayout = "2006-01-02"

// maxWait bounds how long one sync waits for an offline report; Direct
// suggests the interval (retryIn), FLOX caps it.
const (
	maxWait          = 5 * time.Minute
	defaultRetryIn   = 5 * time.Second
	maxRetryInterval = 60 * time.Second
)

type Adapter struct {
	BaseURL    string
	HTTPClient *http.Client
	// Wait sleeps between polls of an offline report and Now reads the
	// clock; tests replace both.
	Wait func(ctx context.Context, d time.Duration) error
	Now  func() time.Time
}

func New() *Adapter {
	return &Adapter{BaseURL: DefaultBaseURL, HTTPClient: httpclient.New(httpclient.AdAPI)}
}

var _ adaccount.CostProvider = (*Adapter)(nil)

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Error is a refusal by Direct, in its documented error shape.
type Error struct {
	Status  int
	Code    int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("yandexdirect: refused (%d, error %d): %s", e.Status, e.Code, e.Message)
}

type apiError struct {
	Error *struct {
		RequestID   string `json:"request_id"`
		ErrorCode   any    `json:"error_code"`
		ErrorString string `json:"error_string"`
		ErrorDetail string `json:"error_detail"`
	} `json:"error"`
}

func refusal(status int, data []byte) error {
	var parsed apiError
	if json.Unmarshal(data, &parsed) == nil && parsed.Error != nil {
		code, _ := strconv.Atoi(fmt.Sprint(parsed.Error.ErrorCode))
		msg := strings.TrimSpace(strings.TrimSuffix(parsed.Error.ErrorString+". "+parsed.Error.ErrorDetail, ". "))
		return &Error{Status: status, Code: code, Message: msg}
	}
	msg := strings.TrimSpace(string(data))
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return &Error{Status: status, Message: msg}
}

func (a *Adapter) client() *http.Client {
	if a.HTTPClient != nil {
		return a.HTTPClient
	}
	return httpclient.New(httpclient.AdAPI)
}

func (a *Adapter) baseURL() string {
	if a.BaseURL != "" {
		return a.BaseURL
	}
	return DefaultBaseURL
}

func (a *Adapter) newRequest(ctx context.Context, path string, creds adaccount.Credentials, body any) (*http.Request, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("yandexdirect: encoding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL()+path, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("yandexdirect: building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if creds.AdAccountID != "" {
		req.Header.Set("Client-Login", creds.AdAccountID)
	}
	return req, nil
}

// Currency is the advertiser's account currency (Clients.get), the one
// report money is in.
func (a *Adapter) Currency(ctx context.Context, creds adaccount.Credentials) (string, error) {
	req, err := a.newRequest(ctx, "/json/v5/clients", creds, map[string]any{
		"method": "get",
		"params": map[string]any{"FieldNames": []string{"Login", "Currency"}},
	})
	if err != nil {
		return "", err
	}
	resp, err := a.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("yandexdirect: calling clients.get: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("yandexdirect: reading clients.get: %w", err)
	}
	// Direct answers an API error with HTTP 200 and an error body.
	var parsed struct {
		apiError
		Result struct {
			Clients []struct {
				Currency string `json:"Currency"`
			} `json:"Clients"`
		} `json:"result"`
	}
	if resp.StatusCode/100 != 2 || json.Unmarshal(data, &parsed) != nil || parsed.Error != nil {
		return "", refusal(resp.StatusCode, data)
	}
	if len(parsed.Result.Clients) == 0 || parsed.Result.Clients[0].Currency == "" {
		return "", errors.New("yandexdirect: clients.get returned no currency (an agency token without the client's login?)")
	}
	return parsed.Result.Clients[0].Currency, nil
}

// DailySpendByCampaign reads the account's currency, then one campaign
// report for [from, to] by day, money in currency units without VAT,
// waiting for it when Direct builds it offline.
func (a *Adapter) DailySpendByCampaign(ctx context.Context, creds adaccount.Credentials, from, to time.Time) ([]adaccount.DailyCampaignSpendRecord, error) {
	currency, err := a.Currency(ctx, creds)
	if err != nil {
		return nil, err
	}
	data, err := a.report(ctx, creds, from, to)
	if err != nil {
		return nil, err
	}
	return parseReport(data, currency)
}

func (a *Adapter) report(ctx context.Context, creds adaccount.Credentials, from, to time.Time) ([]byte, error) {
	wait := a.Wait
	if wait == nil {
		wait = sleep
	}
	now := a.Now
	if now == nil {
		now = time.Now
	}
	dateFrom, dateTo := from.Format(dateLayout), to.Format(dateLayout)
	body := map[string]any{"params": map[string]any{
		"SelectionCriteria": map[string]string{"DateFrom": dateFrom, "DateTo": dateTo},
		"FieldNames":        []string{"Date", "CampaignId", "Cost"},
		// The name must be unique per parameter set: these depend only on
		// the dates.
		"ReportName":    "flox-spend-" + dateFrom + "-" + dateTo,
		"ReportType":    "CAMPAIGN_PERFORMANCE_REPORT",
		"DateRangeType": "CUSTOM_DATE",
		"Format":        "TSV",
		"IncludeVAT":    "NO",
	}}
	deadline := now().Add(maxWait)
	for {
		req, err := a.newRequest(ctx, "/json/v501/reports", creds, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("processingMode", "auto")
		req.Header.Set("returnMoneyInMicros", "false")
		req.Header.Set("skipReportHeader", "true")
		req.Header.Set("skipReportSummary", "true")
		resp, err := a.client().Do(req)
		if err != nil {
			return nil, fmt.Errorf("yandexdirect: calling reports: %w", err)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("yandexdirect: reading report: %w", err)
		}
		switch resp.StatusCode {
		case http.StatusOK:
			return data, nil
		case http.StatusCreated, http.StatusAccepted:
			retryIn := defaultRetryIn
			if s, err := strconv.Atoi(resp.Header.Get("retryIn")); err == nil && s > 0 {
				retryIn = min(time.Duration(s)*time.Second, maxRetryInterval)
			}
			if now().Add(retryIn).After(deadline) {
				return nil, fmt.Errorf("yandexdirect: the report for %s..%s was not ready within %s; the next sync asks again", dateFrom, dateTo, maxWait)
			}
			if err := wait(ctx, retryIn); err != nil {
				return nil, err
			}
		default:
			return nil, refusal(resp.StatusCode, data)
		}
	}
}

// parseReport reads the TSV: a column header row (its names, so the
// column order is not assumed), then Date, CampaignId, Cost per row.
func parseReport(data []byte, currency string) ([]adaccount.DailyCampaignSpendRecord, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = '\t'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("yandexdirect: reading report: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	col := map[string]int{}
	for i, name := range rows[0] {
		col[strings.TrimSpace(name)] = i
	}
	di, okD := col["Date"]
	ci, okC := col["CampaignId"]
	mi, okM := col["Cost"]
	if !okD || !okC || !okM {
		return nil, fmt.Errorf("yandexdirect: unexpected report columns %q", rows[0])
	}
	var out []adaccount.DailyCampaignSpendRecord
	for _, row := range rows[1:] {
		if len(row) <= max(di, ci, mi) {
			continue
		}
		day, err := time.Parse(dateLayout, row[di])
		if err != nil {
			return nil, fmt.Errorf("yandexdirect: report date %q: %w", row[di], err)
		}
		amount, err := strconv.ParseFloat(strings.ReplaceAll(row[mi], ",", "."), 64)
		if err != nil {
			return nil, fmt.Errorf("yandexdirect: report cost %q: %w", row[mi], err)
		}
		out = append(out, adaccount.DailyCampaignSpendRecord{
			Date: day, ExternalCampaignID: row[ci], Amount: amount, Currency: currency,
		})
	}
	return out, nil
}
