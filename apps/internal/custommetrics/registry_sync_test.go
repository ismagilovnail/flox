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
	src, err := os.ReadFile("../../web/src/features/analytics/registry.ts")
	if err != nil {
		t.Skipf("frontend source not available: %v", err)
	}
	block := regexp.MustCompile(`(?s)export const METRICS[^=]*=\s*\[(.*?)\n\];`).FindSubmatch(src)
	if block == nil {
		t.Fatal("METRICS not found in registry.ts")
	}
	var frontend []string
	for _, m := range regexp.MustCompile(`key: "(\w+)"`).FindAllSubmatch(block[1], -1) {
		frontend = append(frontend, string(m[1]))
	}
	backend := append([]string(nil), reportMetricIDs...)
	sort.Strings(frontend)
	sort.Strings(backend)
	if len(frontend) == 0 || !equal(frontend, backend) {
		t.Fatalf("report metrics differ\nfrontend: %v\nbackend:  %v", frontend, backend)
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
