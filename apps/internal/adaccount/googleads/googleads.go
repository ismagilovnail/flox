// Package googleads implements adaccount.CostProvider against the real
// Google Ads API (REST, v17, searchStream/search over GAQL) — §74/
// §27-COST, ad-spend import Phase C. Like facebookads/tiktokads, this
// project has no live Google Ads developer-token/OAuth-app credentials
// to exercise it against a real account; it's verified structurally in
// googleads_test.go against a fake httptest.Server, matching the real
// endpoints' documented request/response shape (confirmed via
// AskUserQuestion before this phase started — see
// docs/ad-account-connections.md).
//
// Unlike Facebook/TikTok, Google Ads has no long-lived access token to
// paste directly — its access tokens expire in ~1 hour. Every sync run
// first exchanges the stored refresh token for a short-lived access
// token (the real OAuth 2.0 refresh-token grant), then calls the Ads
// API with it. Nothing is written back to storage: the refresh token
// itself never expires under normal use, so there's nothing to persist
// from this exchange.
package googleads

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/httpclient"
)

// defaultClient serves an Adapter built without New: never http.DefaultClient,
// which has no timeout (internal/httpclient).
var defaultClient = httpclient.New(httpclient.AdAPI)

// DefaultOAuthBaseURL/DefaultAdsBaseURL are the real Google endpoints
// this adapter was written against. Both are exported on Adapter so a
// test can point them at separate httptest.Servers instead.
const (
	DefaultOAuthBaseURL = "https://oauth2.googleapis.com"
	DefaultAdsBaseURL   = "https://googleads.googleapis.com/v17"
)

const dateLayout = "2006-01-02"

// maxPages mirrors facebookads.maxPages/tiktokads.maxPages's own
// reasoning: bounds pagination against a misbehaving nextPageToken chain
// so a sync can't hang forever on the bounded [from, to] range it
// actually requests.
const maxPages = 500

// Adapter implements adaccount.CostProvider against the Google Ads API.
// Zero value is usable directly via New(); BaseURL fields/HTTPClient are
// exported so a test can substitute fake servers/clients.
type Adapter struct {
	OAuthBaseURL string
	AdsBaseURL   string
	HTTPClient   *http.Client
}

func New() *Adapter {
	return &Adapter{OAuthBaseURL: DefaultOAuthBaseURL, AdsBaseURL: DefaultAdsBaseURL, HTTPClient: httpclient.New(httpclient.AdAPI)}
}

var _ adaccount.CostProvider = (*Adapter)(nil)

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type searchResponse struct {
	Results []struct {
		Campaign struct {
			ID string `json:"id"`
		} `json:"campaign"`
		Customer struct {
			CurrencyCode string `json:"currencyCode"`
			// TimeZone is the account's IANA zone, the one segments.date
			// is cut in (tech audit AN4).
			TimeZone string `json:"timeZone"`
		} `json:"customer"`
		Segments struct {
			Date string `json:"date"`
		} `json:"segments"`
		Metrics struct {
			CostMicros string `json:"costMicros"`
		} `json:"metrics"`
	} `json:"results"`
	NextPageToken string `json:"nextPageToken"`
	Error         *struct {
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// DailySpendByCampaign exchanges the stored refresh token for a
// short-lived access token, then runs one GAQL query per page, following
// nextPageToken until Google reports none or maxPages is hit.
// customer.currency_code is queried alongside each row (a customer-level
// field Google Ads allows joining onto a campaign-rooted query) rather
// than a separate call, unlike tiktokads' own two-call shape — Google
// Ads' GAQL supports it directly.
func (a *Adapter) DailySpendByCampaign(ctx context.Context, creds adaccount.Credentials, from, to time.Time) ([]adaccount.DailyCampaignSpendRecord, error) {
	client := a.HTTPClient
	if client == nil {
		client = defaultClient
	}
	oauthBaseURL := a.OAuthBaseURL
	if oauthBaseURL == "" {
		oauthBaseURL = DefaultOAuthBaseURL
	}
	adsBaseURL := a.AdsBaseURL
	if adsBaseURL == "" {
		adsBaseURL = DefaultAdsBaseURL
	}

	accessToken, err := refreshAccessToken(ctx, client, oauthBaseURL, creds)
	if err != nil {
		return nil, err
	}

	customerID := strings.ReplaceAll(creds.AdAccountID, "-", "")
	query := fmt.Sprintf(
		`SELECT campaign.id, customer.currency_code, customer.time_zone, segments.date, metrics.cost_micros
		 FROM campaign
		 WHERE segments.date BETWEEN '%s' AND '%s'`,
		from.Format(dateLayout), to.Format(dateLayout),
	)

	var records []adaccount.DailyCampaignSpendRecord
	pageToken := ""
	for page := 0; page < maxPages; page++ {
		body, err := json.Marshal(map[string]string{"query": query, "pageToken": pageToken})
		if err != nil {
			return nil, fmt.Errorf("googleads: encoding search request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			adsBaseURL+"/customers/"+customerID+"/googleAds:search", strings.NewReader(string(body)))
		if err != nil {
			return nil, fmt.Errorf("googleads: building search request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("developer-token", creds.DeveloperToken)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("googleads: calling search: %w", err)
		}
		var parsed searchResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&parsed)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("googleads: decoding search response (status %d): %w", resp.StatusCode, decodeErr)
		}
		if parsed.Error != nil {
			return nil, fmt.Errorf("googleads: search api error (%s): %s", parsed.Error.Status, parsed.Error.Message)
		}

		for _, row := range parsed.Results {
			date, err := time.Parse(dateLayout, row.Segments.Date)
			if err != nil {
				return nil, fmt.Errorf("googleads: parsing segments.date %q: %w", row.Segments.Date, err)
			}
			costMicros, err := strconv.ParseFloat(row.Metrics.CostMicros, 64)
			if err != nil {
				return nil, fmt.Errorf("googleads: parsing metrics.costMicros %q for campaign %s: %w", row.Metrics.CostMicros, row.Campaign.ID, err)
			}
			records = append(records, adaccount.DailyCampaignSpendRecord{
				Date:               date,
				ExternalCampaignID: row.Campaign.ID,
				Amount:             costMicros / 1_000_000, // Google Ads reports cost in micros: 1,000,000 micros = 1 unit of the account's currency.
				Currency:           row.Customer.CurrencyCode,
				Timezone:           row.Customer.TimeZone,
			})
		}

		if parsed.NextPageToken == "" {
			break
		}
		pageToken = parsed.NextPageToken
	}

	return records, nil
}

// refreshAccessToken performs the real OAuth 2.0 refresh-token grant —
// the credential this package stores never includes a usable long-lived
// access token, only the means to mint short-lived ones on demand.
func refreshAccessToken(ctx context.Context, client *http.Client, oauthBaseURL string, creds adaccount.Credentials) (string, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {creds.ClientID},
		"client_secret": {creds.ClientSecret},
		"refresh_token": {creds.RefreshToken},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthBaseURL+"/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("googleads: building token refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("googleads: calling token refresh: %w", err)
	}
	defer resp.Body.Close()

	var parsed tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("googleads: decoding token refresh response (status %d): %w", resp.StatusCode, err)
	}
	if parsed.Error != "" {
		return "", fmt.Errorf("googleads: token refresh failed (%s): %s", parsed.Error, parsed.ErrorDescription)
	}
	if parsed.AccessToken == "" {
		return "", fmt.Errorf("googleads: token refresh returned no access_token (status %d)", resp.StatusCode)
	}
	return parsed.AccessToken, nil
}
