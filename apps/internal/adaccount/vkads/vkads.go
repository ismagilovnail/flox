// Package vkads implements adaccount.CostProvider against the VK Ads API
// (VK Реклама, ads.vk.ru; audit UNI-3 part 2; §74, §27-COST): the
// account's currency from user.json, its campaigns (ad plans — "кампании"
// in VK's interface) from ad_plans.json, their daily spend from
// statistics/ad_plans/day.json. Written against VK's documentation
// (ads.vk.ru/doc/api) and tested against a fake server only — there is no
// VK Ads account here (docs/ad-account-connections.md "VK Ads").
//
// Credentials: the API client's id and secret (VK's cabinet, "Доступ к
// API"); for an agency, AdAccountID is the client's username. VK's access
// tokens live a day, and at most 5 may exist per client and account at
// once, so a token is not minted per sync: the one minted first is kept
// (Credentials.SaveTokens), refreshed when VK refuses it, and minted again
// only when the refresh is refused too.
package vkads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/adaccount"
	"github.com/ismagilovnail/flox/apps/internal/httpclient"
)

const DefaultBaseURL = "https://ads.vk.ru"

const dateLayout = "2006-01-02"

const (
	// maxIDsPerRequest: "В одном запросе можно получить статистику не более
	// чем по 200 объектам".
	maxIDsPerRequest = 200
	plansPerPage     = 50
	maxPages         = 200
)

type Adapter struct {
	BaseURL    string
	HTTPClient *http.Client
}

func New() *Adapter {
	return &Adapter{BaseURL: DefaultBaseURL, HTTPClient: httpclient.New(httpclient.AdAPI)}
}

var _ adaccount.CostProvider = (*Adapter)(nil)

// Error is a refusal by VK, in its documented {"error": {"code",
// "message"}} shape.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("vkads: refused (%d %s): %s", e.Status, e.Code, e.Message)
}

func refusal(status int, data []byte) error {
	var api struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &api) == nil && api.Error != nil {
		return &Error{Status: status, Code: api.Error.Code, Message: api.Error.Message}
	}
	// The token endpoint answers in OAuth's shape: "error" is a string.
	var oauth struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if json.Unmarshal(data, &oauth) == nil && oauth.Error != "" {
		return &Error{Status: status, Code: oauth.Error, Message: oauth.Description}
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

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// oauth posts form to the token endpoint and returns the pair.
func (a *Adapter) oauth(ctx context.Context, form url.Values) (tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL()+"/api/v2/oauth2/token.json", strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, fmt.Errorf("vkads: building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client().Do(req)
	if err != nil {
		return tokenResponse{}, fmt.Errorf("vkads: calling the token endpoint: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return tokenResponse{}, fmt.Errorf("vkads: reading the token answer: %w", err)
	}
	var tr tokenResponse
	if resp.StatusCode/100 != 2 || json.Unmarshal(data, &tr) != nil || tr.AccessToken == "" {
		err := refusal(resp.StatusCode, data)
		if resp.StatusCode == http.StatusForbidden {
			// "По достижении лимита в ответ на попытку получить новый токен
			// будет возвращена ошибка с HTTP-кодом 403."
			return tokenResponse{}, fmt.Errorf("%w (VK allows 5 tokens per API client and account: delete the old ones — oauth2/token/delete.json — and sync again)", err)
		}
		return tokenResponse{}, err
	}
	return tr, nil
}

// mint gets a new token: client_credentials for the API client's own
// account, agency_client_credentials for an agency's client (AdAccountID
// is the client's username).
func (a *Adapter) mint(ctx context.Context, creds adaccount.Credentials) (tokenResponse, error) {
	form := url.Values{"client_id": {creds.ClientID}, "client_secret": {creds.ClientSecret}}
	if creds.AdAccountID != "" {
		form.Set("grant_type", "agency_client_credentials")
		form.Set("agency_client_name", creds.AdAccountID)
	} else {
		form.Set("grant_type", "client_credentials")
	}
	return a.oauth(ctx, form)
}

func (a *Adapter) refresh(ctx context.Context, creds adaccount.Credentials) (tokenResponse, error) {
	return a.oauth(ctx, url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {creds.RefreshToken},
		"client_id": {creds.ClientID}, "client_secret": {creds.ClientSecret},
	})
}

// session holds the token for one sync and renews it once on a refusal.
type session struct {
	a       *Adapter
	creds   adaccount.Credentials
	renewed bool
}

func (s *session) save(ctx context.Context, tr tokenResponse) error {
	if tr.RefreshToken == "" {
		tr.RefreshToken = s.creds.RefreshToken
	}
	s.creds.AccessToken, s.creds.RefreshToken = tr.AccessToken, tr.RefreshToken
	if s.creds.SaveTokens != nil {
		if err := s.creds.SaveTokens(ctx, tr.AccessToken, tr.RefreshToken); err != nil {
			return fmt.Errorf("vkads: keeping the new token: %w", err)
		}
	}
	return nil
}

// renew refreshes the token, or mints a new one when there is no refresh
// token or VK refuses it.
func (s *session) renew(ctx context.Context) error {
	if s.creds.RefreshToken != "" {
		if tr, err := s.a.refresh(ctx, s.creds); err == nil {
			return s.save(ctx, tr)
		}
	}
	tr, err := s.a.mint(ctx, s.creds)
	if err != nil {
		return err
	}
	return s.save(ctx, tr)
}

// get calls path with query, decoding the answer into out.
func (s *session) get(ctx context.Context, path string, query url.Values, out any) error {
	if s.creds.AccessToken == "" {
		if err := s.renew(ctx); err != nil {
			return err
		}
		s.renewed = true
	}
	for {
		u := s.a.baseURL() + path
		if len(query) > 0 {
			u += "?" + query.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return fmt.Errorf("vkads: building request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+s.creds.AccessToken)
		resp, err := s.a.client().Do(req)
		if err != nil {
			return fmt.Errorf("vkads: calling %s: %w", path, err)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("vkads: reading %s: %w", path, err)
		}
		if resp.StatusCode == http.StatusUnauthorized && !s.renewed {
			// A day-old token, or one refreshed elsewhere: renew once.
			s.renewed = true
			if err := s.renew(ctx); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode/100 != 2 {
			return refusal(resp.StatusCode, data)
		}
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("vkads: decoding %s: %w", path, err)
		}
		return nil
	}
}

// DailySpendByCampaign reads the account's currency, every ad plan
// (deleted ones too: their past spend counts), then their daily spend in
// batches of 200.
func (a *Adapter) DailySpendByCampaign(ctx context.Context, creds adaccount.Credentials, from, to time.Time) ([]adaccount.DailyCampaignSpendRecord, error) {
	if creds.ClientID == "" || creds.ClientSecret == "" {
		return nil, errors.New("vkads: the connection has no API client id/secret")
	}
	s := &session{a: a, creds: creds}

	var user struct {
		Currency string `json:"currency"`
	}
	if err := s.get(ctx, "/api/v3/user.json", nil, &user); err != nil {
		return nil, err
	}
	if user.Currency == "" {
		return nil, errors.New("vkads: user.json returned no currency")
	}

	var ids []string
	for page := 0; page < maxPages; page++ {
		var plans struct {
			Count int `json:"count"`
			Items []struct {
				ID int64 `json:"id"`
			} `json:"items"`
		}
		q := url.Values{
			"limit": {strconv.Itoa(plansPerPage)}, "offset": {strconv.Itoa(page * plansPerPage)},
			"_status__in": {"active,blocked,deleted"},
		}
		if err := s.get(ctx, "/api/v2/ad_plans.json", q, &plans); err != nil {
			return nil, err
		}
		for _, p := range plans.Items {
			ids = append(ids, strconv.FormatInt(p.ID, 10))
		}
		if len(plans.Items) < plansPerPage || len(ids) >= plans.Count {
			break
		}
	}

	var out []adaccount.DailyCampaignSpendRecord
	for start := 0; start < len(ids); start += maxIDsPerRequest {
		batch := ids[start:min(start+maxIDsPerRequest, len(ids))]
		var stats struct {
			Items []struct {
				ID   json.Number `json:"id"`
				Rows []struct {
					Date string `json:"date"`
					Base struct {
						Spent json.Number `json:"spent"`
					} `json:"base"`
				} `json:"rows"`
			} `json:"items"`
		}
		q := url.Values{
			"id": {strings.Join(batch, ",")}, "date_from": {from.Format(dateLayout)}, "date_to": {to.Format(dateLayout)},
			"metrics": {"base"},
		}
		if err := s.get(ctx, "/api/v2/statistics/ad_plans/day.json", q, &stats); err != nil {
			return nil, err
		}
		for _, item := range stats.Items {
			for _, row := range item.Rows {
				day, err := time.Parse(dateLayout, row.Date)
				if err != nil {
					return nil, fmt.Errorf("vkads: statistics date %q: %w", row.Date, err)
				}
				amount, err := strconv.ParseFloat(row.Base.Spent.String(), 64)
				if err != nil {
					return nil, fmt.Errorf("vkads: statistics spent %q: %w", row.Base.Spent, err)
				}
				out = append(out, adaccount.DailyCampaignSpendRecord{
					Date: day, ExternalCampaignID: item.ID.String(), Amount: amount, Currency: user.Currency,
				})
			}
		}
	}
	return out, nil
}
