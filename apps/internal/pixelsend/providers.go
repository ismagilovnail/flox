package pixelsend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/ismagilovnail/flox/apps/internal/macro"
	"github.com/ismagilovnail/flox/apps/internal/pixel"
)

// Senders is one Sender per provider that can send (pixel.Provider.Sends).
func Senders(client HTTPClient) map[pixel.Provider]Sender {
	return map[pixel.Provider]Sender{
		pixel.ProviderFacebook: &Facebook{Client: client},
		pixel.ProviderTikTok:   &TikTok{Client: client},
		pixel.ProviderSnapchat: &Snapchat{Client: client},
		pixel.ProviderGeneric:  &Generic{Client: client},
	}
}

// --- Facebook (Meta) Conversions API -------------------------------------

// FacebookGraphURL is the Graph API version the Conversions API is called
// on.
const FacebookGraphURL = "https://graph.facebook.com/v21.0"

// Facebook sends to the Meta Conversions API: POST {pixel}/events with the
// access token as a query parameter, one event per request.
type Facebook struct {
	Client  HTTPClient
	BaseURL string // FacebookGraphURL when empty
}

func (f *Facebook) Send(ctx context.Context, c pixel.Credentials, ev pixel.Event) Outcome {
	if c.PixelID == "" {
		return errNotConfigured("pixel id is empty")
	}
	if c.AccessToken == "" {
		return errNotConfigured("access token is not set")
	}
	user := map[string]any{"external_id": []string{sha256Hex(ev.ClickID)}}
	if ev.IP != "" {
		user["client_ip_address"] = ev.IP
	}
	if ev.UserAgent != "" {
		user["client_user_agent"] = ev.UserAgent
	}
	if ev.FBClickID != "" {
		// fbc: the click id with the time of the click it arrived on.
		user["fbc"] = fmt.Sprintf("fb.1.%d.%s", ev.ClickAt.UnixMilli(), ev.FBClickID)
	}
	data := map[string]any{
		"event_name":    pixel.EventName(c.Provider, c.EventNames, string(ev.Type)),
		"event_time":    ev.EventAt.Unix(),
		"event_id":      ev.SourceEventID,
		"action_source": "website",
		"user_data":     user,
	}
	if amount, currency, ok := value(ev); ok {
		data["custom_data"] = map[string]any{"value": amount, "currency": currency}
	}
	body := map[string]any{"data": []any{data}}
	if c.TestEventCode != "" {
		body["test_event_code"] = c.TestEventCode
	}

	base := f.BaseURL
	if base == "" {
		base = FacebookGraphURL
	}
	endpoint := base + "/" + url.PathEscape(c.PixelID) + "/events?access_token=" + url.QueryEscape(c.AccessToken)
	code, resp, err := postJSON(ctx, f.Client, endpoint, nil, body)
	if err != nil || (code >= 200 && code < 300) {
		return httpOutcome(code, err, "")
	}
	var fbErr struct {
		Error struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(resp, &fbErr)
	out := httpOutcome(code, nil, truncate(fbErr.Error.Message, 300))
	// Graph API throttling comes back as 400 with these codes.
	switch fbErr.Error.Code {
	case 1, 2, 4, 17, 32, 341, 613:
		out.Retry = true
	}
	return out
}

// --- TikTok Events API ----------------------------------------------------

// TikTokEventsURL is the Events API 2.0 endpoint.
const TikTokEventsURL = "https://business-api.tiktok.com/open_api/v1.3/event/track/"

// TikTok sends to the TikTok Events API. It answers HTTP 200 for errors
// too, with a non-zero code in the body.
type TikTok struct {
	Client   HTTPClient
	Endpoint string // TikTokEventsURL when empty
}

func (t *TikTok) Send(ctx context.Context, c pixel.Credentials, ev pixel.Event) Outcome {
	if c.PixelID == "" {
		return errNotConfigured("pixel code is empty")
	}
	if c.AccessToken == "" {
		return errNotConfigured("access token is not set")
	}
	user := map[string]any{"external_id": sha256Hex(ev.ClickID)}
	if ev.TTClickID != "" {
		user["ttclid"] = ev.TTClickID
	}
	if ev.IP != "" {
		user["ip"] = ev.IP
	}
	if ev.UserAgent != "" {
		user["user_agent"] = ev.UserAgent
	}
	data := map[string]any{
		"event":      pixel.EventName(c.Provider, c.EventNames, string(ev.Type)),
		"event_time": ev.EventAt.Unix(),
		"event_id":   ev.SourceEventID,
		"user":       user,
	}
	if amount, currency, ok := value(ev); ok {
		data["properties"] = map[string]any{"value": amount, "currency": currency}
	}
	body := map[string]any{
		"event_source":    "web",
		"event_source_id": c.PixelID,
		"data":            []any{data},
	}
	if c.TestEventCode != "" {
		body["test_event_code"] = c.TestEventCode
	}

	endpoint := t.Endpoint
	if endpoint == "" {
		endpoint = TikTokEventsURL
	}
	code, resp, err := postJSON(ctx, t.Client, endpoint, map[string]string{"Access-Token": c.AccessToken}, body)
	if err != nil || code < 200 || code >= 300 {
		return httpOutcome(code, err, truncate(string(resp), 300))
	}
	var answer struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(resp, &answer); err != nil {
		return Outcome{StatusCode: code, Err: fmt.Errorf("unreadable response: %w", err), Retry: true}
	}
	if answer.Code == 0 {
		return Outcome{StatusCode: code}
	}
	return Outcome{
		StatusCode: code,
		Err:        fmt.Errorf("platform error %d: %s", answer.Code, truncate(answer.Message, 300)),
		// 40100: rate limited; 5xxxx: TikTok's own failure.
		Retry: answer.Code == 40100 || answer.Code >= 50000,
	}
}

// --- Snapchat Conversions API ---------------------------------------------

// SnapchatEventsURL is the Conversions API v3 base.
const SnapchatEventsURL = "https://tr.snapchat.com/v3"

// Snapchat sends to the Snap Conversions API v3, access token as a query
// parameter. Snap has no test event codes.
type Snapchat struct {
	Client  HTTPClient
	BaseURL string // SnapchatEventsURL when empty
}

func (s *Snapchat) Send(ctx context.Context, c pixel.Credentials, ev pixel.Event) Outcome {
	if c.PixelID == "" {
		return errNotConfigured("pixel id is empty")
	}
	if c.AccessToken == "" {
		return errNotConfigured("access token is not set")
	}
	user := map[string]any{"external_id": []string{sha256Hex(ev.ClickID)}}
	if ev.IP != "" {
		user["client_ip_address"] = ev.IP
	}
	if ev.UserAgent != "" {
		user["client_user_agent"] = ev.UserAgent
	}
	data := map[string]any{
		"event_name":    pixel.EventName(c.Provider, c.EventNames, string(ev.Type)),
		"event_time":    ev.EventAt.Unix(),
		"event_id":      ev.SourceEventID,
		"action_source": "WEB",
		"user_data":     user,
	}
	if amount, currency, ok := value(ev); ok {
		data["custom_data"] = map[string]any{"value": amount, "currency": currency}
	}

	base := s.BaseURL
	if base == "" {
		base = SnapchatEventsURL
	}
	endpoint := base + "/" + url.PathEscape(c.PixelID) + "/events?access_token=" + url.QueryEscape(c.AccessToken)
	code, resp, err := postJSON(ctx, s.Client, endpoint, nil, map[string]any{"data": []any{data}})
	return httpOutcome(code, err, truncate(string(resp), 300))
}

// --- Generic --------------------------------------------------------------

// Generic calls the pixel's URL template (GET) with its macros filled in —
// for any platform or in-house endpoint that takes a server-to-server
// pixel as a URL. {status} is the FLOX event type.
type Generic struct {
	Client HTTPClient
}

func (g *Generic) Send(ctx context.Context, c pixel.Credentials, ev pixel.Event) Outcome {
	if c.URL == "" {
		return errNotConfigured("URL is empty")
	}
	v := ev.Macros
	v.ClickID = ev.ClickID
	v.Status = pixel.EventName(c.Provider, c.EventNames, string(ev.Type))
	if v.Revenue == "" && ev.Revenue != nil {
		v.Revenue = strconv.FormatFloat(*ev.Revenue, 'f', -1, 64)
		v.Currency = ev.Currency
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, macro.ResolveURL(c.URL, v), nil)
	if err != nil {
		return Outcome{Err: fmt.Errorf("invalid URL: %w", err)}
	}
	code, _, err := do(g.Client, req)
	return httpOutcome(code, err, "")
}
