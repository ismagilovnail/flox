package classifier_test

import (
	"context"
	"net"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/classifier"
)

// BenchmarkClassify measures Classifier.Classify with the default (Noop
// geo/ASN, heuristic bot) providers apps/tracker actually wires up today
// (tracker/main.go: classifier.New(nil, nil, nil)) — Phase 31's benchmark
// list names this as one of the five things on or near the §41 hot path;
// with no real geo/ASN vendor integrated yet, this is pure CPU (UA regex
// parsing), not I/O, so it should be the cheapest of the five.
func BenchmarkClassify(b *testing.B) {
	c := classifier.New(nil, nil, nil)
	ctx := context.Background()
	in := classifier.Input{
		IP:             net.ParseIP("203.0.113.42"),
		UserAgent:      "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Mobile Safari/537.36",
		AcceptLanguage: "en-US,en;q=0.9,de;q=0.8",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Classify(ctx, in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseUserAgent(b *testing.B) {
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifier.ParseUserAgent(ua)
	}
}
