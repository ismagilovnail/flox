package custommetrics

// The metric catalog a formula's {tokens} are validated against —
// mirrors apps/web/src/lib/custom-metrics-registry.ts's METRIC_CATALOG
// exactly (id, dataSource, insertable). Two independent copies (this one
// and the frontend's) rather than one shared source, same tradeoff
// already accepted for the report dimension whitelist between
// registry.ts and chstore.ReportDimension — kept deliberately small and
// flat so a future drift is easy to spot in review. LTV entries are
// insertable:false, which is what actually enforces §30.5's "LTV not
// allowed in formulas" rule — not a separate check.
type dataSource string

const (
	sourceRegular dataSource = "regular"
	sourcePush    dataSource = "push"
	sourceLTV     dataSource = "ltv"
)

type registryMetric struct {
	id         string
	dataSource dataSource
	insertable bool
}

// reportMetricIDs are the analytics report's metrics — every key of
// features/analytics/registry.ts's METRICS, the numbers each report row
// carries. Formulas on the report builder can use any of them.
// TestReportMetricsMatchTheFrontend keeps the two lists equal (marketer
// audit #12: the funnel metrics were missing here, so a formula the form
// accepted was rejected on save).
var reportMetricIDs = []string{
	"clicks", "uniqueClicks", "conversions", "revenue", "cost", "profit",
	"roi", "roas", "ctr", "cvr", "cpc", "cpa", "epc",
	"pwaInstalls", "registrations", "purchases", "repurchases", "declines", "trash",
	"clickToInstall", "installToReg", "regToPurchase",
	"costPerInstall", "costPerReg", "costPerPurchase",
	"blocked", "bots", "blockedRate", "botRate", "emptySubs", "emptySubsRate",
}

var metricCatalog = buildCatalog()

func buildCatalog() map[string]registryMetric {
	m := map[string]registryMetric{}
	add := func(ds dataSource, insertable bool, ids ...string) {
		for _, id := range ids {
			m[id] = registryMetric{id: id, dataSource: ds, insertable: insertable}
		}
	}

	// Traffic / Performance — features/analytics/registry.ts's METRICS.
	add(sourceRegular, true, reportMetricIDs...)

	// CPA Funnel — catalog-only until apps/tracker emits these, matching
	// the frontend's own `live: false` (that flag doesn't affect formula
	// validity, only whether any surface can compute a live value yet —
	// same reasoning as PUSH below).
	add(sourceRegular, true,
		"cpa_hold", "cpa_accept", "cpa_redep", "cpa_decline", "cpa_trash",
		"reg_to_ftd_rate", "ftd_to_redep_rate", "dep_to_redep",
		"total_deposits", "total_deposit_revenue",
	)

	// Fraud — "bots" is a report metric now (above); "all clicks incl.
	// bots" is not, so it stays catalog-only.
	add(sourceRegular, true, "click_all")

	// Push — no Push module exists yet.
	add(sourcePush, true, "push_sent", "push_delivered", "push_opened", "push_ctr")

	// LTV — insertable:false is deliberate, not a placeholder: §30.5
	// explicitly forbids LTV/Cohort metrics inside custom formulas.
	add(sourceLTV, false,
		"ltv_d0", "ltv_d1_7", "ltv_d8_30", "ltv_d31_90", "ltv_total",
		"ltv_per_ftd", "ltv_per_reg", "lifetime_days",
	)

	return m
}

// surfaceAvailableMetricIDs mirrors SURFACE_AVAILABLE_METRIC_IDS
// exactly: which registry metric ids each "show in" surface can
// actually supply as formula inputs today. offers_table/sources_table
// are empty sets — Offer/Network aren't backed analytics dimensions yet
// (docs/analytics-report-api.md), so neither surface can honestly
// compute anything.
var surfaceAvailableMetricIDs = map[ShowInTarget]map[string]bool{
	TargetReportBuilder: allRegularMetricIDs(),
	TargetCampaignsTable: {
		"clicks": true, "conversions": true, "revenue": true,
		"cost": true, "profit": true, "roi": true,
	},
	TargetOffersTable:  {},
	TargetSourcesTable: {},
}

func allRegularMetricIDs() map[string]bool {
	out := map[string]bool{}
	for _, id := range reportMetricIDs {
		out[id] = true
	}
	return out
}

// surfaceCanCompute mirrors the frontend's own function of the same
// name: a target is only genuinely usable if every metric id the
// formula actually references is available on that surface.
func surfaceCanCompute(target ShowInTarget, usedMetricIDs []string) bool {
	available := surfaceAvailableMetricIDs[target]
	if len(usedMetricIDs) == 0 {
		return false
	}
	for _, id := range usedMetricIDs {
		if !available[id] {
			return false
		}
	}
	return true
}
