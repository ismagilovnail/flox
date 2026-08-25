// Package tiktokads implements adaccount.CostProvider against the real
// TikTok Business API (Marketing API v1.3) — §74/§27-COST, ad-spend
// import Phase B. Like facebookads, this project has no live TikTok
// developer app credentials to exercise it against a real advertiser
// account; it's verified structurally in tiktokads_test.go against a
// fake httptest.Server, matching the real endpoints' documented
// request/response shape (confirmed via AskUserQuestion before Phase B
// started — see docs/ad-account-connections.md).
package tiktokads

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
)

// DefaultBaseURL is the real TikTok Business API host + the API version
// this adapter was written against. Adapter.BaseURL exists specifically
// so a test can point it at an httptest.Server instead.
const DefaultBaseURL = "https://business-api.tiktok.com/open_api/v1.3"

const (
	dateLayout     = "2006-01-02"
	statDayLayout  = "2006-01-02 15:04:05"
	pageSize       = 1000
	accessTokenHdr = "Access-Token"
)

// maxPages mirrors facebookads.maxPages's reasoning: bounds pagination
// against a misbehaving total_page value so a sync can't hang forever on
// the bounded [from, to] ranges it actually requests.
const maxPages = 500

// Adapter implements adaccount.CostProvider against the TikTok Business
// API. Zero value is usable directly via New(); BaseURL/HTTPClient are
// exported so a test can substitute a fake server/client.
type Adapter struct {
	BaseURL    string
	HTTPClient *http.Client
}

func New() *Adapter {
	return &Adapter{BaseURL: DefaultBaseURL, HTTPClient: http.DefaultClient}
}

var _ adaccount.CostProvider = (*Adapter)(nil)

type advertiserInfoResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		List []struct {
			AdvertiserID string `json:"advertiser_id"`
			Currency     string `json:"currency"`
		} `json:"list"`
	} `json:"data"`
}

type integratedReportResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		List []struct {
			Dimensions struct {
				CampaignID  string `json:"campaign_id"`
				StatTimeDay string `json:"stat_time_day"`
			} `json:"dimensions"`
			Metrics struct {
				Spend string `json:"spend"`
			} `json:"metrics"`
		} `json:"list"`
		PageInfo struct {
			Page      int `json:"page"`
			TotalPage int `json:"total_page"`
		} `json:"page_info"`
	} `json:"data"`
}

// DailySpendByCampaign fetches the advertiser account's own currency
// (TikTok's reporting endpoint reports spend in the advertiser's native
// currency but does not echo the currency itself per row — it's an
// account-level property, unlike Facebook's account_currency field)
// and then the daily-per-campaign spend report, following pagination
// until TikTok reports no further page or maxPages is hit.
func (a *Adapter) DailySpendByCampaign(ctx context.Context, creds adaccount.Credentials, from, to time.Time) ([]adaccount.DailyCampaignSpendRecord, error) {
	client := a.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	baseURL := a.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	currency, err := a.fetchCurrency(ctx, client, baseURL, creds)
	if err != nil {
		return nil, err
	}

	dimensions, err := json.Marshal([]string{"campaign_id", "stat_time_day"})
	if err != nil {
		return nil, fmt.Errorf("tiktokads: encoding dimensions: %w", err)
	}
	metrics, err := json.Marshal([]string{"spend"})
	if err != nil {
		return nil, fmt.Errorf("tiktokads: encoding metrics: %w", err)
	}

	var records []adaccount.DailyCampaignSpendRecord
	for page := 1; page <= maxPages; page++ {
		q := url.Values{}
		q.Set("advertiser_id", creds.AdAccountID)
		q.Set("report_type", "BASIC")
		q.Set("data_level", "AUCTION_CAMPAIGN")
		q.Set("dimensions", string(dimensions))
		q.Set("metrics", string(metrics))
		q.Set("start_date", from.Format(dateLayout))
		q.Set("end_date", to.Format(dateLayout))
		q.Set("page", strconv.Itoa(page))
		q.Set("page_size", strconv.Itoa(pageSize))

		reqURL := baseURL + "/report/integrated/get/?" + q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("tiktokads: building report request: %w", err)
		}
		req.Header.Set(accessTokenHdr, creds.AccessToken)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("tiktokads: calling integrated report: %w", err)
		}
		var body integratedReportResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("tiktokads: decoding report response (status %d): %w", resp.StatusCode, decodeErr)
		}
		if body.Code != 0 {
			return nil, fmt.Errorf("tiktokads: report api error (code %d): %s", body.Code, body.Message)
		}

		for _, row := range body.Data.List {
			date, err := time.Parse(statDayLayout, row.Dimensions.StatTimeDay)
			if err != nil {
				return nil, fmt.Errorf("tiktokads: parsing stat_time_day %q: %w", row.Dimensions.StatTimeDay, err)
			}
			amount, err := strconv.ParseFloat(row.Metrics.Spend, 64)
			if err != nil {
				return nil, fmt.Errorf("tiktokads: parsing spend %q for campaign %s: %w", row.Metrics.Spend, row.Dimensions.CampaignID, err)
			}
			records = append(records, adaccount.DailyCampaignSpendRecord{
				Date:               date,
				ExternalCampaignID: row.Dimensions.CampaignID,
				Amount:             amount,
				Currency:           currency,
			})
		}

		if body.Data.PageInfo.Page >= body.Data.PageInfo.TotalPage {
			break
		}
	}

	return records, nil
}

// fetchCurrency looks up the advertiser account's own currency via
// GET /advertiser/info/ — a separate call because TikTok's reporting
// endpoint does not include currency in its per-row response.
func (a *Adapter) fetchCurrency(ctx context.Context, client *http.Client, baseURL string, creds adaccount.Credentials) (string, error) {
	ids, err := json.Marshal([]string{creds.AdAccountID})
	if err != nil {
		return "", fmt.Errorf("tiktokads: encoding advertiser_ids: %w", err)
	}

	q := url.Values{}
	q.Set("advertiser_ids", string(ids))
	reqURL := baseURL + "/advertiser/info/?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("tiktokads: building advertiser info request: %w", err)
	}
	req.Header.Set(accessTokenHdr, creds.AccessToken)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("tiktokads: calling advertiser info: %w", err)
	}
	defer resp.Body.Close()

	var body advertiserInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("tiktokads: decoding advertiser info response (status %d): %w", resp.StatusCode, err)
	}
	if body.Code != 0 {
		return "", fmt.Errorf("tiktokads: advertiser info api error (code %d): %s", body.Code, body.Message)
	}
	if len(body.Data.List) == 0 {
		return "", fmt.Errorf("tiktokads: advertiser info returned no results for account %s", creds.AdAccountID)
	}
	return body.Data.List[0].Currency, nil
}
