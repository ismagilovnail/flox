package ltv

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

const dateLayout = "2006-01-02"

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Register mounts every /ltv route onto r. The caller applies
// tenant.Middleware first — same contract as every other handler in this
// codebase.
func (h *Handler) Register(r chi.Router) {
	r.Get("/ftd-cohorts", h.ftdCohorts)
	r.Get("/reg-cohorts", h.regCohorts)
}

type windowResponse struct {
	RevenueUSD float64 `json:"revenueUsd"`
	Complete   bool    `json:"complete"`
}

func windowsResponse(windows map[WindowKey]WindowResult) map[string]windowResponse {
	out := make(map[string]windowResponse, len(windows))
	for k, v := range windows {
		out[string(k)] = windowResponse{RevenueUSD: v.RevenueUSD, Complete: v.Complete}
	}
	return out
}

type baseResponse struct {
	Bucket                 string                    `json:"bucket"`
	CampaignID             string                    `json:"campaignId,omitempty"`
	NetworkID              string                    `json:"networkId,omitempty"`
	Country                string                    `json:"country,omitempty"`
	AnchorCount            int                       `json:"anchorCount"`
	Windows                map[string]windowResponse `json:"windows"`
	LTVTotalUSD            float64                   `json:"ltvTotalUsd"`
	LTVPerAnchorUSD        float64                   `json:"ltvPerAnchorUsd"`
	LifetimeDaysAvg        float64                   `json:"lifetimeDaysAvg,omitempty"`
	HasLifetimeData        bool                      `json:"hasLifetimeData"`
	RedepUniqueCount       int                       `json:"redepUniqueCount"`
	TotalDeposits          int                       `json:"totalDeposits"`
	TotalDepositRevenueUSD float64                   `json:"totalDepositRevenueUsd"`
}

func toBaseResponse(k CohortKey, b base) baseResponse {
	return baseResponse{
		Bucket: k.Bucket, CampaignID: k.CampaignID, NetworkID: k.NetworkID, Country: k.Country,
		AnchorCount: b.AnchorCount, Windows: windowsResponse(b.Windows),
		LTVTotalUSD: b.LTVTotalUSD, LTVPerAnchorUSD: b.LTVPerAnchorUSD,
		LifetimeDaysAvg: b.LifetimeDaysAvg, HasLifetimeData: b.HasLifetimeData,
		RedepUniqueCount: b.RedepUniqueCount, TotalDeposits: b.TotalDeposits, TotalDepositRevenueUSD: b.TotalDepositRevenueUSD,
	}
}

type ftdCohortResponse struct {
	baseResponse
	FTDToRedepRate float64 `json:"ftdToRedepRate"`
	DepToRedepRate float64 `json:"depToRedepRate"`
}

type regCohortResponse struct {
	baseResponse
	RegToFTDRate float64 `json:"regToFtdRate"`
}

func (h *Handler) ftdCohorts(w http.ResponseWriter, r *http.Request) {
	orgID, period, filter, ok := h.parseParams(w, r)
	if !ok {
		return
	}
	cohorts, err := h.svc.FTDCohorts(r.Context(), orgID, filter, period, time.Now().UTC())
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	resp := make([]ftdCohortResponse, len(cohorts))
	for i, c := range cohorts {
		resp[i] = ftdCohortResponse{baseResponse: toBaseResponse(c.Key, c.base), FTDToRedepRate: c.FTDToRedepRate, DepToRedepRate: c.DepToRedepRate}
	}
	writeJSON(w, http.StatusOK, map[string]any{"cohorts": resp})
}

func (h *Handler) regCohorts(w http.ResponseWriter, r *http.Request) {
	orgID, period, filter, ok := h.parseParams(w, r)
	if !ok {
		return
	}
	cohorts, err := h.svc.RegCohorts(r.Context(), orgID, filter, period, time.Now().UTC())
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	resp := make([]regCohortResponse, len(cohorts))
	for i, c := range cohorts {
		resp[i] = regCohortResponse{baseResponse: toBaseResponse(c.Key, c.base), RegToFTDRate: c.RegToFTDRate}
	}
	writeJSON(w, http.StatusOK, map[string]any{"cohorts": resp})
}

// parseParams reads ?period=&from=&to=&campaignId=&networkId=&country=.
// from/to default to the last 90 days when omitted — §26.5 itself says a
// report needs "≥ 90-day range for full windows," so that's the useful
// default view, not just an arbitrary one. On a parse error it writes the
// response itself and returns ok=false.
func (h *Handler) parseParams(w http.ResponseWriter, r *http.Request) (orgID string, period CohortPeriod, filter chstore.LTVFilter, ok bool) {
	orgID, _ = tenant.OrgID(r.Context())
	q := r.URL.Query()

	period = CohortPeriod(q.Get("period"))
	if period == "" {
		period = PeriodDay
	}

	to := time.Now().UTC()
	from := to.AddDate(0, 0, -90)
	if v := q.Get("from"); v != "" {
		parsed, err := time.Parse(dateLayout, v)
		if err != nil {
			apierror.Write(w, h.logger, apierror.Validation("invalid from date, want YYYY-MM-DD", nil))
			return "", "", chstore.LTVFilter{}, false
		}
		from = parsed
	}
	if v := q.Get("to"); v != "" {
		parsed, err := time.Parse(dateLayout, v)
		if err != nil {
			apierror.Write(w, h.logger, apierror.Validation("invalid to date, want YYYY-MM-DD", nil))
			return "", "", chstore.LTVFilter{}, false
		}
		// chstore.LTVFilter's anchor query is a half-open `event_at >= from
		// AND event_at < to` range (internal/chstore/ltv.go) — a bare
		// date-only `to` parses as that day's midnight, which would exclude
		// every anchor event from `to` itself. End-of-day makes a same-day
		// `to` actually include today, matching internal/conversions'
		// handler, which applies the identical adjustment for the identical
		// reason.
		to = parsed.Add(24*time.Hour - time.Nanosecond)
	}

	filter = chstore.LTVFilter{
		From: from, To: to,
		CampaignID: q.Get("campaignId"),
		NetworkID:  q.Get("networkId"),
		Country:    q.Get("country"),
	}
	return orgID, period, filter, true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
