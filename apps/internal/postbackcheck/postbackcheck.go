// Package postbackcheck is the dry run of an incoming postback (audit
// 2026-10-15, UX-3 part 2): the operator pastes the URL a network will
// call, and FLOX answers what it would do with it — the tracker's HTTP
// answer, the mapped status, success / duplicate / ignored / error and why,
// the click it would be attributed to, the money in USD, the outgoing
// postback it would queue — without recording anything
// (conversion.Service.Check). docs/onboarding.md "Check a postback".
package postbackcheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/campaignscope"
	"github.com/ismagilovnail/flox/apps/internal/conversion"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// Checker is the conversion engine's dry run; *conversion.Service is the
// real one.
type Checker interface {
	Check(ctx context.Context, p conversion.Postback) (conversion.CheckResult, error)
}

// Attribution is the click a conversion would be attributed to.
type Attribution struct {
	Outcome    string `json:"outcome"`
	Method     string `json:"method,omitempty"`
	ClickID    string `json:"clickId,omitempty"`
	CampaignID string `json:"campaignId,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// Outcome is the dry run's answer.
type Outcome struct {
	NetworkID string `json:"networkId"`
	// HTTPStatus is what the tracker would answer the network: 200 for a
	// postback it handled (whatever the result), 400 for a bad amount or
	// event time, 401 for a wrong or missing secret. Only a 5xx makes a
	// network retry.
	HTTPStatus int `json:"httpStatus"`
	// Result is the tracker's result field: success, duplicate, ignored or
	// error.
	Result          string       `json:"result"`
	Status          string       `json:"status,omitempty"`
	Message         string       `json:"message,omitempty"`
	Attribution     *Attribution `json:"attribution,omitempty"`
	Revenue         *float64     `json:"revenue,omitempty"`
	Currency        string       `json:"currency,omitempty"`
	CurrencyAssumed bool         `json:"currencyAssumed,omitempty"`
	USDValue        *float64     `json:"usdValue,omitempty"`
	OutgoingURL     string       `json:"outgoingUrl,omitempty"`
	// Notes are about the URL itself, not the engine's answer — e.g. a
	// host that is not the tracker's.
	Notes []string `json:"notes"`
}

type Service struct {
	networks conversion.NetworkLookup
	engine   Checker
	// trackerURL is TRACKER_PUBLIC_URL; empty skips the host check.
	trackerURL string
	now        func() time.Time
}

func NewService(networks conversion.NetworkLookup, engine Checker, trackerURL string) *Service {
	return &Service{networks: networks, engine: engine, trackerURL: trackerURL, now: time.Now}
}

// Check runs the postback rawURL describes for the organization orgID. The
// network comes from the URL's /postback/{networkId} path and must be
// the organization's own; another organization's network is not found,
// exactly like one that does not exist.
func (s *Service) Check(ctx context.Context, orgID, rawURL string) (Outcome, error) {
	if orgID == "" {
		return Outcome{}, apierror.Validation("missing organization", nil)
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || strings.TrimSpace(rawURL) == "" {
		return Outcome{}, apierror.Validation("not a URL", map[string]string{"url": "paste the postback URL the network will call"})
	}
	networkID := networkIDFrom(u.Path)
	if networkID == "" {
		return Outcome{}, apierror.Validation("no network in the URL", map[string]string{"url": "the path must end in /postback/{networkId}"})
	}

	network, err := s.networks.ByID(ctx, networkID)
	if errors.Is(err, conversion.ErrNetworkNotFound) || (err == nil && network.OrganizationID != orgID) {
		return Outcome{}, apierror.NotFound("network not found")
	}
	if err != nil {
		return Outcome{}, err
	}

	out := Outcome{NetworkID: network.ID, Notes: s.notes(u)}
	form := u.Query()

	// The tracker's own order (apps/tracker PostbackHandler): the secret,
	// then the parameters, then the engine.
	if !conversion.ValidSecret(form.Get("secret"), network.PostbackSecretHash) {
		out.HTTPStatus, out.Result, out.Message = http.StatusUnauthorized, "error", "invalid or missing secret"
		return out, nil
	}
	p, err := conversion.ParseParams(form, network, s.now())
	if err != nil {
		out.HTTPStatus, out.Result, out.Message = http.StatusBadRequest, "error", strings.TrimPrefix(err.Error(), "conversion: ")
		return out, nil
	}

	res, err := s.engine.Check(ctx, p)
	if err != nil {
		return Outcome{}, fmt.Errorf("postbackcheck: %w", err)
	}
	out.HTTPStatus = http.StatusOK
	out.Result = string(res.Kind)
	out.Status = string(res.Status)
	out.Message = res.Message
	if res.Attribution.Outcome != "" {
		out.Attribution = &Attribution{
			Outcome:    string(res.Attribution.Outcome),
			Method:     string(res.Attribution.Method),
			ClickID:    res.Attribution.Click.ClickID,
			CampaignID: res.Attribution.Click.CampaignID,
			Reason:     res.Attribution.Reason,
		}
	}
	out.Revenue, out.Currency, out.CurrencyAssumed, out.USDValue = res.Revenue, res.Currency, res.CurrencyAssumed, res.USDValue
	out.OutgoingURL = res.OutgoingURL
	// A click of a campaign the member may not read (campaignscope, audit
	// TEAM-1b) reads as an unknown click: the dry run must not tell a
	// Buyer whose campaign it is, nor show its outgoing postback.
	if out.Attribution != nil && out.Attribution.CampaignID != "" {
		scope, err := campaignscope.From(ctx)
		if err != nil {
			return Outcome{}, err
		}
		if !scope.Allows(out.Attribution.CampaignID) {
			out.Attribution = &Attribution{Outcome: string(attribution.OutcomeUnknownClick), ClickID: out.Attribution.ClickID,
				Reason: "no click with this id in your campaigns"}
			out.OutgoingURL = ""
		}
	}
	return out, nil
}

// networkIDFrom reads {networkId} from a path ending in
// /postback/{networkId} (a trailing slash allowed), or "".
func networkIDFrom(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[len(parts)-2] != "postback" {
		return ""
	}
	if id := parts[len(parts)-1]; idgen.IsValid(id) {
		return id
	}
	return ""
}

// notes says what is wrong with the URL beyond its parameters: a network
// that calls another host never reaches FLOX, whatever the dry run says.
func (s *Service) notes(u *url.URL) []string {
	notes := []string{}
	if s.trackerURL == "" || u.Host == "" {
		return notes
	}
	tracker, err := url.Parse(s.trackerURL)
	if err != nil || tracker.Host == "" {
		return notes
	}
	if !strings.EqualFold(u.Host, tracker.Host) {
		notes = append(notes, fmt.Sprintf("the URL's host %s is not the tracker's %s — a network calling it never reaches FLOX", u.Host, tracker.Host))
	}
	return notes
}

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Register mounts POST /check. The caller applies tenant.Middleware and
// the read gate: the dry run writes nothing.
func (h *Handler) Register(r chi.Router) {
	r.Post("/check", h.check)
}

type checkRequest struct {
	URL string `json:"url"`
}

func (h *Handler) check(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req checkRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid request body", nil))
		return
	}
	out, err := h.svc.Check(r.Context(), orgID, req.URL)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
