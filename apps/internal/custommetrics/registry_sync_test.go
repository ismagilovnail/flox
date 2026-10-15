package custommetrics

import (
	"os"
	"regexp"
	"sort"
	"testing"
)

// The frontend builds its formula catalog from the analytics registry's
// METRICS; this side keeps its own list (registry.go). When they differ, the
// form accepts a formula the API then rejects as an unknown metric — what
// happened with the funnel metrics (marketer audit #12).
func TestReportMetricsMatchTheFrontend(t *testing.T) {
	src, err := os.ReadFile("../../web/src/lib/analytics-registry.ts")
	if err != nil {
		t.Skipf("frontend source not available: %v", err)
	}
	block := regexp.MustCompile(`(?s)export const METRICS[^=]*=\s*\[(.*?)\n\];`).FindSubmatch(src)
	if block == nil {
		t.Fatal("METRICS not found in analytics-registry.ts")
	}
	// The report's push columns are METRICS entries too, but a separate
	// data source: they must be exactly the push catalog.
	var frontend, frontendPush []string
	for _, m := range regexp.MustCompile(`key: "(\w+)"`).FindAllSubmatch(block[1], -1) {
		if id := string(m[1]); metricCatalog[id].dataSource == sourcePush {
			frontendPush = append(frontendPush, id)
		} else {
			frontend = append(frontend, id)
		}
	}
	backend := append([]string(nil), reportMetricIDs...)
	sort.Strings(frontend)
	sort.Strings(backend)
	if len(frontend) == 0 || !equal(frontend, backend) {
		t.Fatalf("report metrics differ\nfrontend: %v\nbackend:  %v", frontend, backend)
	}
	backendPush := append([]string(nil), pushMetricIDs...)
	sort.Strings(frontendPush)
	sort.Strings(backendPush)
	if !equal(frontendPush, backendPush) {
		t.Fatalf("report push metrics differ\nfrontend: %v\nbackend:  %v", frontendPush, backendPush)
	}
	for _, id := range backendPush {
		if !surfaceAvailableMetricIDs[TargetReportBuilder][id] {
			t.Errorf("%s is not available on the report builder", id)
		}
	}
	for _, id := range backend {
		m, ok := metricCatalog[id]
		if !ok || !m.insertable || m.dataSource != sourceRegular {
			t.Errorf("%s: %+v, want an insertable regular metric", id, m)
		}
		if !surfaceAvailableMetricIDs[TargetReportBuilder][id] {
			t.Errorf("%s is not available on the report builder", id)
		}
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSurfacesMatchTheFrontend keeps surfaceAvailableMetricIDs equal to
// custom-metrics-registry.ts's SURFACE_AVAILABLE_METRIC_IDS for every
// surface listed there as literal ids (report_builder is all of METRICS,
// checked above). A metric added to one side only made the server drop a
// target the form had offered (emptySubsRate on the campaigns table).
func TestSurfacesMatchTheFrontend(t *testing.T) {
	src, err := os.ReadFile("../../web/src/lib/custom-metrics-registry.ts")
	if err != nil {
		t.Skipf("frontend source not available: %v", err)
	}
	block := regexp.MustCompile(`(?s)export const SURFACE_AVAILABLE_METRIC_IDS[^=]*=\s*\{(.*?)\n\};`).FindSubmatch(src)
	if block == nil {
		t.Fatal("SURFACE_AVAILABLE_METRIC_IDS not found")
	}
	surfaces := regexp.MustCompile(`(\w+): new Set\((?:\[([^\]]*)\])?\)`).FindAllSubmatch(block[1], -1)
	seen := map[ShowInTarget]bool{}
	for _, m := range surfaces {
		target := ShowInTarget(m[1])
		seen[target] = true
		var frontend []string
		for _, id := range regexp.MustCompile(`"(\w+)"`).FindAllSubmatch(m[2], -1) {
			frontend = append(frontend, string(id[1]))
		}
		var backend []string
		for id := range surfaceAvailableMetricIDs[target] {
			backend = append(backend, id)
		}
		sort.Strings(frontend)
		sort.Strings(backend)
		if !equal(frontend, backend) {
			t.Errorf("%s differs\nfrontend: %v\nbackend:  %v", target, frontend, backend)
		}
	}
	for target := range surfaceAvailableMetricIDs {
		if target != TargetReportBuilder && !seen[target] {
			t.Errorf("%s has no literal id list in the frontend", target)
		}
	}
}
