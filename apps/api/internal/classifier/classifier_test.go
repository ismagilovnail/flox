package classifier_test

import (
	"context"
	"net"
	"testing"

	"github.com/ismagilovnail/flox/apps/api/internal/classifier"
	"github.com/ismagilovnail/flox/apps/api/internal/routing"
)

func TestHeuristicBotDetector(t *testing.T) {
	d := classifier.HeuristicBotDetector{}
	ctx := context.Background()

	cases := []struct {
		ua      string
		wantBot bool
	}{
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/124.0.0.0 Safari/537.36", false},
		{"Googlebot/2.1 (+http://www.google.com/bot.html)", true},
		{"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)", true},
		{"curl/8.4.0", true},
		{"python-requests/2.31.0", true},
	}

	for _, tc := range cases {
		got, err := d.Detect(ctx, classifier.BotInput{UserAgent: tc.ua})
		if err != nil {
			t.Fatalf("Detect(%q) error: %v", tc.ua, err)
		}
		if got.IsBot != tc.wantBot {
			t.Errorf("Detect(%q).IsBot = %v, want %v", tc.ua, got.IsBot, tc.wantBot)
		}
		if got.IsProxy {
			t.Errorf("Detect(%q).IsProxy = true, want false (no proxy vendor wired up — must never fabricate)", tc.ua)
		}
	}
}

// fakeGeoProvider/fakeASNProvider let the wiring test assert Classify
// actually calls through to the injected providers and maps their output
// onto the right routing.FilterField keys, without needing a real vendor.
type fakeGeoProvider struct{ result classifier.GeoResult }

func (f fakeGeoProvider) Lookup(ctx context.Context, ip net.IP) (classifier.GeoResult, error) {
	return f.result, nil
}

type fakeASNProvider struct{ result classifier.ASNResult }

func (f fakeASNProvider) Lookup(ctx context.Context, ip net.IP) (classifier.ASNResult, error) {
	return f.result, nil
}

func TestClassify_WiresProvidersAndUAIntoRoutingAttributes(t *testing.T) {
	c := classifier.New(
		fakeGeoProvider{result: classifier.GeoResult{Country: "US", Region: "CA", City: "San Francisco"}},
		fakeASNProvider{result: classifier.ASNResult{ASN: "AS15169"}},
		classifier.HeuristicBotDetector{},
	)

	attrs, err := c.Classify(context.Background(), classifier.Input{
		IP:             net.ParseIP("8.8.8.8"),
		UserAgent:      "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 Version/17.4 Mobile/15E148 Safari/604.1",
		AcceptLanguage: "en-US,en;q=0.9,de;q=0.8",
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}

	want := routing.Attributes{
		routing.FieldCountry:        "US",
		routing.FieldRegion:         "CA",
		routing.FieldCity:           "San Francisco",
		routing.FieldDevice:         "mobile",
		routing.FieldPlatform:       "ios",
		routing.FieldOS:             "ios",
		routing.FieldOSVersion:      "17.4",
		routing.FieldBrowser:        "safari",
		routing.FieldBrowserVersion: "17.4",
		routing.FieldLanguage:       "en-US",
		routing.FieldBot:            "0",
		routing.FieldProxy:          "0",
		routing.FieldASN:            "AS15169",
		routing.FieldConnectionType: "unknown",
	}
	for field, wantVal := range want {
		if got := attrs[field]; got != wantVal {
			t.Errorf("attrs[%q] = %q, want %q", field, got, wantVal)
		}
	}

	// Prove this really is routing.Attributes, ready to hand straight to
	// Router.Resolve with no adapter layer in between.
	var _ routing.Attributes = attrs
}

func TestClassify_DefaultsWhenNoProvidersInjected(t *testing.T) {
	c := classifier.New(nil, nil, nil)
	attrs, err := c.Classify(context.Background(), classifier.Input{IP: net.ParseIP("1.1.1.1"), UserAgent: "curl/8.4.0"})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if attrs[routing.FieldCountry] != "" {
		t.Errorf("want empty country from NoopGeoProvider, got %q", attrs[routing.FieldCountry])
	}
	if attrs[routing.FieldBot] != "1" {
		t.Errorf("want bot=1 for curl via HeuristicBotDetector, got %q", attrs[routing.FieldBot])
	}
}
