package analytics

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

// Register mounts every /analytics route onto r. The caller applies
// tenant.Middleware first — same contract as campaign.Handler.Register.
func (h *Handler) Register(r chi.Router) {
	r.Get("/campaigns/{campaignId}/daily", h.campaignDaily)
	r.Get("/campaigns/{campaignId}/daily-revenue", h.campaignDailyRevenue)

	// Phase 35's multi-dimension surface. POST+JSON body, not GET+query —
	// dims/filters/sort are structured, variable-length shapes, the same
	// reason routingsimulate.Handler's /simulate is POST (see that
	// package's own handler.go).
	// These five are POST only because they carry a JSON query body —
	// they read ClickHouse and write nothing, so they carry no
	// requireWrite gate. Nor are they gated on analytics.read: every
	// one of the six roles holds it (migration 00001), so the check
	// would reject nobody and read as a control that isn't one.
	r.Post("/report", h.report)
	r.Post("/timeseries", h.timeseries)
	r.Post("/breakdown", h.breakdown)
	r.Post("/funnel", h.funnel)
	r.Post("/dashboard", h.dashboard)
	r.Post("/dimension-values", h.dimensionValues)
}

type dailyCountResponse struct {
	Day        string `json:"day"`
	Type       string `json:"type"`
	EventCount uint64 `json:"eventCount"`
}

type dailyRevenueResponse struct {
	Day        string  `json:"day"`
	Type       string  `json:"type"`
	EventCount uint64  `json:"eventCount"`
	RevenueUSD float64 `json:"revenueUsd"`
}

// campaignDaily answers GET /analytics/campaigns/{campaignId}/daily?from=YYYY-MM-DD&to=YYYY-MM-DD
// — click/filter volume from click_events_daily_campaign.
func (h *Handler) campaignDaily(w http.ResponseWriter, r *http.Request) {
	orgID, campaignID, from, to, ok := h.parseRangeParams(w, r)
	if !ok {
		return
	}

	counts, err := h.svc.CampaignDaily(r.Context(), orgID, campaignID, from, to)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}

	resp := make([]dailyCountResponse, len(counts))
	for i, c := range counts {
		resp[i] = dailyCountResponse{Day: c.Day.Format(dateLayout), Type: string(c.Type), EventCount: c.EventCount}
	}
	writeJSON(w, http.StatusOK, map[string]any{"counts": resp})
}

// campaignDailyRevenue answers GET /analytics/campaigns/{campaignId}/daily-revenue?from=&to=
// — conversion counts and USD revenue from conversion_events_daily_campaign.
func (h *Handler) campaignDailyRevenue(w http.ResponseWriter, r *http.Request) {
	orgID, campaignID, from, to, ok := h.parseRangeParams(w, r)
	if !ok {
		return
	}

	revenue, err := h.svc.CampaignDailyRevenue(r.Context(), orgID, campaignID, from, to)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}

	resp := make([]dailyRevenueResponse, len(revenue))
	for i, rv := range revenue {
		resp[i] = dailyRevenueResponse{Day: rv.Day.Format(dateLayout), Type: string(rv.Type), EventCount: rv.EventCount, RevenueUSD: rv.RevenueUSD}
	}
	writeJSON(w, http.StatusOK, map[string]any{"revenue": resp})
}

// parseRangeParams reads {campaignId}, ?from=, ?to= — from/to default to
// the last 7 days when omitted, matching a dashboard's most common first
// load. On a parse error it writes the response itself and returns
// ok=false.
func (h *Handler) parseRangeParams(w http.ResponseWriter, r *http.Request) (orgID, campaignID string, from, to time.Time, ok bool) {
	orgID, _ = tenant.OrgID(r.Context())
	campaignID = chi.URLParam(r, "campaignId")

	to = time.Now().UTC()
	from = to.AddDate(0, 0, -7)
	if v := r.URL.Query().Get("from"); v != "" {
		parsed, err := time.Parse(dateLayout, v)
		if err != nil {
			apierror.Write(w, h.logger, apierror.Validation("invalid from date, want YYYY-MM-DD", nil))
			return "", "", time.Time{}, time.Time{}, false
		}
		from = parsed
	}
	if v := r.URL.Query().Get("to"); v != "" {
		parsed, err := time.Parse(dateLayout, v)
		if err != nil {
			apierror.Write(w, h.logger, apierror.Validation("invalid to date, want YYYY-MM-DD", nil))
			return "", "", time.Time{}, time.Time{}, false
		}
		to = parsed
	}
	return orgID, campaignID, from, to, true
}

// --- Phase 35: Report/Timeseries/Breakdown/Funnel/Dashboard ---

// metricValuesResponse is MetricValues with explicit JSON tags —
// MetricValues itself stays tag-free since apps/internal/analytics's own
// callers (LTV, tests) never serialize it directly, and a pointer field
// marshals to `null` on its own, which is exactly what CLAUDE.md #6's "no
// cost data on file" contract needs on the wire.
type metricValuesResponse struct {
	Clicks       uint64   `json:"clicks"`
	UniqueClicks uint64   `json:"uniqueClicks"`
	Conversions  *uint64  `json:"conversions"`
	Revenue      *float64 `json:"revenue"`
	Cost         *float64 `json:"cost"`
	Profit       *float64 `json:"profit"`
	ROI          *float64 `json:"roi"`
	ROAS         *float64 `json:"roas"`
	CTR          float64  `json:"ctr"`
	CVR          float64  `json:"cvr"`
	CPC          *float64 `json:"cpc"`
	CPA          *float64 `json:"cpa"`
	EPC          float64  `json:"epc"`

	PwaInstalls     uint64   `json:"pwaInstalls"`
	Registrations   *uint64  `json:"registrations"`
	Purchases       *uint64  `json:"purchases"`
	Repurchases     *uint64  `json:"repurchases"`
	ClickToInstall  *float64 `json:"clickToInstall"`
	InstallToReg    *float64 `json:"installToReg"`
	RegToPurchase   *float64 `json:"regToPurchase"`
	CostPerInstall  *float64 `json:"costPerInstall"`
	CostPerReg      *float64 `json:"costPerReg"`
	CostPerPurchase *float64 `json:"costPerPurchase"`
}

func metricValuesToResponse(m MetricValues) metricValuesResponse {
	return metricValuesResponse{
		Clicks: m.Clicks, UniqueClicks: m.UniqueClicks, Conversions: m.Conversions,
		Revenue: m.Revenue, Cost: m.Cost, Profit: m.Profit, ROI: m.ROI, ROAS: m.ROAS,
		CTR: m.CTR, CVR: m.CVR, CPC: m.CPC, CPA: m.CPA, EPC: m.EPC,
		PwaInstalls: m.PwaInstalls, Registrations: m.Registrations, Purchases: m.Purchases, Repurchases: m.Repurchases,
		ClickToInstall: m.ClickToInstall, InstallToReg: m.InstallToReg, RegToPurchase: m.RegToPurchase,
		CostPerInstall: m.CostPerInstall, CostPerReg: m.CostPerReg, CostPerPurchase: m.CostPerPurchase,
	}
}

// reportRowResponse serializes a ReportRow — Day set (Dims omitted) for a
// timeseries row, Dims set (Day omitted) for a report/breakdown/dashboard-
// breakdown row, mirroring ReportRow's own doc comment.
type reportRowResponse struct {
	Day     *string              `json:"day,omitempty"`
	Dims    map[string]string    `json:"dims,omitempty"`
	Metrics metricValuesResponse `json:"metrics"`
}

func reportRowToResponse(row ReportRow) reportRowResponse {
	resp := reportRowResponse{Metrics: metricValuesToResponse(row.Metrics)}
	if row.Day != nil {
		d := row.Day.Format(dateLayout)
		resp.Day = &d
	}
	if len(row.DimValues) > 0 {
		resp.Dims = make(map[string]string, len(row.DimValues))
		for k, v := range row.DimValues {
			resp.Dims[string(k)] = v
		}
	}
	return resp
}

func reportRowsToResponse(rows []ReportRow) []reportRowResponse {
	out := make([]reportRowResponse, len(rows))
	for i, row := range rows {
		out[i] = reportRowToResponse(row)
	}
	return out
}

type reportFilterRequest struct {
	Dimension string `json:"dimension"`
	Value     string `json:"value"`
}

func parseReportFilters(in []reportFilterRequest) []chstore.ReportFilter {
	out := make([]chstore.ReportFilter, len(in))
	for i, f := range in {
		out[i] = chstore.ReportFilter{Dimension: chstore.ReportDimension(f.Dimension), Value: f.Value}
	}
	return out
}

func parseReportDims(in []string) []chstore.ReportDimension {
	out := make([]chstore.ReportDimension, len(in))
	for i, d := range in {
		out[i] = chstore.ReportDimension(d)
	}
	return out
}

// parseDateRangeBody parses two YYYY-MM-DD body fields — the JSON-body
// counterpart of parseRangeParams' query-string parsing above, for the
// endpoints below that take from/to alongside other structured fields a
// query string can't express cleanly.
func parseDateRangeBody(fromStr, toStr string) (from, to time.Time, err error) {
	from, err = time.Parse(dateLayout, fromStr)
	if err != nil {
		return time.Time{}, time.Time{}, apierror.Validation("invalid from date, want YYYY-MM-DD", map[string]string{"from": "invalid"})
	}
	to, err = time.Parse(dateLayout, toStr)
	if err != nil {
		return time.Time{}, time.Time{}, apierror.Validation("invalid to date, want YYYY-MM-DD", map[string]string{"to": "invalid"})
	}
	return from, to, nil
}

type reportRequest struct {
	Dims    []string              `json:"dims"`
	Filters []reportFilterRequest `json:"filters"`
	From    string                `json:"from"`
	To      string                `json:"to"`
	Sort    struct {
		Key string `json:"key"`
		Dir string `json:"dir"`
	} `json:"sort"`
	Timezone string `json:"timezone"`
}

// report answers POST /analytics/report — the report builder table's
// endpoint (0-4 dims, filter, sort, every §50 metric per group).
func (h *Handler) report(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req reportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	from, to, err := parseDateRangeBody(req.From, req.To)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}

	rows, err := h.svc.Report(r.Context(), orgID, ReportInput{
		Dims:     parseReportDims(req.Dims),
		Filters:  parseReportFilters(req.Filters),
		From:     from,
		To:       to,
		Sort:     SortSpec{Key: req.Sort.Key, Dir: req.Sort.Dir},
		Timezone: req.Timezone,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": reportRowsToResponse(rows)})
}

type timeseriesRequest struct {
	Filters  []reportFilterRequest `json:"filters"`
	From     string                `json:"from"`
	To       string                `json:"to"`
	Timezone string                `json:"timezone"`
}

// timeseries answers POST /analytics/timeseries — one row per day, every
// metric included so the client can switch which one it plots without a
// second request (TimeseriesInput's own doc comment).
func (h *Handler) timeseries(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req timeseriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	from, to, err := parseDateRangeBody(req.From, req.To)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}

	rows, err := h.svc.Timeseries(r.Context(), orgID, TimeseriesInput{
		Filters:  parseReportFilters(req.Filters),
		From:     from,
		To:       to,
		Timezone: req.Timezone,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": reportRowsToResponse(rows)})
}

type breakdownRequest struct {
	Dim     string                `json:"dim"`
	Metric  string                `json:"metric"`
	Filters []reportFilterRequest `json:"filters"`
	From    string                `json:"from"`
	To      string                `json:"to"`
	Limit   int                   `json:"limit"`
}

// breakdown answers POST /analytics/breakdown — the bar chart's endpoint
// (exactly one dimension, sorted by one metric descending, top-N).
func (h *Handler) breakdown(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req breakdownRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	from, to, err := parseDateRangeBody(req.From, req.To)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}

	rows, err := h.svc.Breakdown(r.Context(), orgID, BreakdownInput{
		Dim:     chstore.ReportDimension(req.Dim),
		Metric:  req.Metric,
		Filters: parseReportFilters(req.Filters),
		From:    from,
		To:      to,
		Limit:   req.Limit,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": reportRowsToResponse(rows)})
}

type rangeRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type funnelRequest struct {
	Filters  []reportFilterRequest `json:"filters"`
	From     string                `json:"from"`
	To       string                `json:"to"`
	Timezone string                `json:"timezone"`
}

type funnelStepResponse struct {
	Step  string `json:"step"`
	Value uint64 `json:"value"`
}

// funnel answers POST /analytics/funnel — the §43 funnel's per-stage
// totals for the requested range.
func (h *Handler) funnel(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req funnelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	from, to, err := parseDateRangeBody(req.From, req.To)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}

	steps, err := h.svc.Funnel(r.Context(), orgID, from, to, parseReportFilters(req.Filters), req.Timezone)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	resp := make([]funnelStepResponse, len(steps))
	for i, s := range steps {
		resp[i] = funnelStepResponse{Step: s.Step, Value: s.Value}
	}
	writeJSON(w, http.StatusOK, map[string]any{"steps": resp})
}

type dashboardResponse struct {
	Totals       metricValuesResponse `json:"totals"`
	Daily        []reportRowResponse  `json:"daily"`
	TopCampaigns []reportRowResponse  `json:"topCampaigns"`
	TopCountries []reportRowResponse  `json:"topCountries"`
	TopFlows     []reportRowResponse  `json:"topFlows"`
}

// dashboard answers POST /analytics/dashboard — /overview's own endpoint
// (KPI totals, a daily series, three Top-N breakdowns; see DashboardResult's
// own doc comment on why there's no Top Offers here).
func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req rangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	from, to, err := parseDateRangeBody(req.From, req.To)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}

	result, err := h.svc.Dashboard(r.Context(), orgID, from, to)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, dashboardResponse{
		Totals:       metricValuesToResponse(result.Totals),
		Daily:        reportRowsToResponse(result.Daily),
		TopCampaigns: reportRowsToResponse(result.TopCampaigns),
		TopCountries: reportRowsToResponse(result.TopCountries),
		TopFlows:     reportRowsToResponse(result.TopFlows),
	})
}

type dimensionValuesRequest struct {
	Dim  string `json:"dim"`
	From string `json:"from"`
	To   string `json:"to"`
}

// dimensionValues answers POST /analytics/dimension-values — the report
// builder's filter-value dropdown's own endpoint (docs/analytics-report-
// api.md). A dimension outside chstore.ValidDimension's 15 (the 2 still-
// unbacked ones) surfaces as the same apierror.Validation every other
// endpoint here already gives for an unsupported dimension; the frontend
// falls back to its own mock pool for those rather than treating this
// as a hard failure.
func (h *Handler) dimensionValues(w http.ResponseWriter, r *http.Request) {
	orgID, _ := tenant.OrgID(r.Context())
	var req dimensionValuesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, h.logger, apierror.Validation("invalid JSON body", nil))
		return
	}
	from, to, err := parseDateRangeBody(req.From, req.To)
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}

	values, err := h.svc.DimensionValues(r.Context(), orgID, DimensionValuesInput{
		Dim:  chstore.ReportDimension(req.Dim),
		From: from,
		To:   to,
	})
	if err != nil {
		apierror.Write(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"values": values})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
