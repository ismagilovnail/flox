// Package facebookads implements adaccount.CostProvider against the real
// Facebook Marketing API (Graph API Insights, level=campaign) — §74/
// §27-COST, ad-spend import Phase B. This project has no live Meta app
// credentials to exercise it against a real ad account; it's verified
// structurally in facebookads_test.go against a fake httptest.Server
// standing in for graph.facebook.com, matching the real endpoint's
// documented request/response shape (confirmed via AskUserQuestion
// before Phase B started — see docs/ad-account-connections.md).
package facebookads

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/httpclient"
)

// defaultClient serves an Adapter built without New: never http.DefaultClient,
// which has no timeout (internal/httpclient).
var defaultClient = httpclient.New(httpclient.AdAPI)

// DefaultBaseURL is the real Facebook Graph API host + the API version
// this adapter was written against. Adapter.BaseURL exists specifically
// so a test can point it at an httptest.Server instead.
const DefaultBaseURL = "https://graph.facebook.com/v19.0"

const dateLayout = "2006-01-02"

// maxPages bounds pagination so a misbehaving or malicious paging.next
// chain (e.g. one that loops back on itself) can't hang a sync
// indefinitely — no real ad account needs more than this many days'
// worth of campaign-insight pages for the bounded [from, to] ranges a
// sync actually requests (§27-COST syncs run daily against a short
// lookback window, never an unbounded history backfill).
const maxPages = 500

// Adapter implements adaccount.CostProvider against the Facebook
// Marketing API's Insights endpoint. Zero value is usable directly via
// New(); BaseURL/HTTPClient are exported so a test can substitute a fake
// server/client without needing a constructor option for every field.
type Adapter struct {
	BaseURL    string
	HTTPClient *http.Client
}

func New() *Adapter {
	return &Adapter{BaseURL: DefaultBaseURL, HTTPClient: httpclient.New(httpclient.AdAPI)}
}

var _ adaccount.CostProvider = (*Adapter)(nil)

// insightsResponse mirrors the subset of the real Graph API Insights
// response shape this adapter reads. spend arrives as a JSON string
// (Facebook's own convention for money fields, not a float — avoids the
// API silently losing cents to JSON float encoding on their end), hence
// json:"spend" being a string here rather than a float64.
type insightsResponse struct {
	Data []struct {
		CampaignID      string `json:"campaign_id"`
		Spend           string `json:"spend"`
		AccountCurrency string `json:"account_currency"`
		DateStart       string `json:"date_start"`
	} `json:"data"`
	Paging struct {
		Next string `json:"next"`
	} `json:"paging"`
	Error *struct {
		Message   string `json:"message"`
		Type      string `json:"type"`
		Code      int    `json:"code"`
		FBTraceID string `json:"fbtrace_id"`
	} `json:"error"`
}

// DailySpendByCampaign calls GET /act_{adAccountId}/insights with
// level=campaign and time_increment=1 (one row per calendar day per
// campaign — the exact breakdown adaccount.CostProvider's own doc
// comment requires, since cost_entries needs both a day and a specific
// FLOX campaign to write against), following paging.next until Facebook
// reports no further page or maxPages is hit.
func (a *Adapter) DailySpendByCampaign(ctx context.Context, creds adaccount.Credentials, from, to time.Time) ([]adaccount.DailyCampaignSpendRecord, error) {
	client := a.HTTPClient
	if client == nil {
		client = defaultClient
	}
	baseURL := a.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	timeRange, err := json.Marshal(map[string]string{
		"since": from.Format(dateLayout),
		"until": to.Format(dateLayout),
	})
	if err != nil {
		return nil, fmt.Errorf("facebookads: encoding time_range: %w", err)
	}

	q := url.Values{}
	q.Set("level", "campaign")
	q.Set("time_increment", "1")
	q.Set("fields", "campaign_id,spend,account_currency")
	q.Set("time_range", string(timeRange))
	q.Set("access_token", creds.AccessToken)
	q.Set("limit", "500")

	next := baseURL + "/act_" + url.PathEscape(creds.AdAccountID) + "/insights?" + q.Encode()

	var records []adaccount.DailyCampaignSpendRecord
	for page := 0; next != "" && page < maxPages; page++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, fmt.Errorf("facebookads: building request: %w", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("facebookads: calling insights: %w", err)
		}

		var body insightsResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("facebookads: decoding insights response (status %d): %w", resp.StatusCode, decodeErr)
		}
		if body.Error != nil {
			return nil, fmt.Errorf("facebookads: graph api error (code %d, type %s, fbtrace_id %s): %s",
				body.Error.Code, body.Error.Type, body.Error.FBTraceID, body.Error.Message)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("facebookads: insights returned status %d with no error body", resp.StatusCode)
		}

		for _, row := range body.Data {
			date, err := time.Parse(dateLayout, row.DateStart)
			if err != nil {
				return nil, fmt.Errorf("facebookads: parsing date_start %q: %w", row.DateStart, err)
			}
			amount, err := strconv.ParseFloat(row.Spend, 64)
			if err != nil {
				return nil, fmt.Errorf("facebookads: parsing spend %q for campaign %s: %w", row.Spend, row.CampaignID, err)
			}
			records = append(records, adaccount.DailyCampaignSpendRecord{
				Date:               date,
				ExternalCampaignID: row.CampaignID,
				Amount:             amount,
				Currency:           row.AccountCurrency,
			})
		}

		next = body.Paging.Next
	}

	if len(records) > 0 {
		// Insights dates are calendar days in the account's own zone,
		// which the rows do not carry — one more call (tech audit AN4).
		// Best effort: spend is right without it, only the report's
		// "different zone" note needs it.
		tz := a.accountTimezone(ctx, client, baseURL, creds)
		for i := range records {
			records[i].Timezone = tz
		}
	}
	return records, nil
}

// accountTimezone reads GET /act_{id}?fields=timezone_name, "" on any
// failure.
func (a *Adapter) accountTimezone(ctx context.Context, client *http.Client, baseURL string, creds adaccount.Credentials) string {
	q := url.Values{}
	q.Set("fields", "timezone_name")
	q.Set("access_token", creds.AccessToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		baseURL+"/act_"+url.PathEscape(creds.AdAccountID)+"?"+q.Encode(), nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var body struct {
		TimezoneName string `json:"timezone_name"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&body) != nil {
		return ""
	}
	return body.TimezoneName
}
