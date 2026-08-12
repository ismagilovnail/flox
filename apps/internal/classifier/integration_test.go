package classifier_test

import (
	"context"
	"net"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/classifier"
	"github.com/ismagilovnail/flox/apps/internal/routing"
)

// TestClassifyIntoRouting proves the "classify request" step of §39's
// ROUTING ENGINE ORDER really does feed straight into "evaluate filters"
// with no adapter/translation layer in between — a stream set filtering
// on routing.FieldDevice matches attributes this package produced.
func TestClassifyIntoRouting(t *testing.T) {
	c := classifier.New(
		fakeGeoProvider{result: classifier.GeoResult{Country: "US"}},
		fakeASNProvider{},
		classifier.HeuristicBotDetector{},
	)

	attrs, err := c.Classify(context.Background(), classifier.Input{
		IP:        net.ParseIP("203.0.113.7"),
		UserAgent: "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 Chrome/124.0.0.0 Mobile Safari/537.36",
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}

	set := routing.StreamSet{
		ID: "mobile-us", Priority: 1, Status: routing.StreamSetActive,
		RootFilter: routing.FilterGroup{Joiner: routing.JoinAND, Children: []routing.FilterNode{
			routing.FilterCondition{Field: routing.FieldCountry, Operator: routing.OpIs, Value: "US"},
			routing.FilterCondition{Field: routing.FieldDevice, Operator: routing.OpIs, Value: "mobile"},
			routing.FilterCondition{Field: routing.FieldBot, Operator: routing.OpIs, Value: "0"},
		}},
		Flows: []routing.Flow{{ID: "f1", Name: "f1", Active: true, Weight: 100, Destination: routing.Destination{Kind: routing.DestinationRedirect, URL: "https://match.example"}}},
	}
	cfg := routing.RoutingConfig{CampaignID: "c1", StreamSets: []routing.StreamSet{set}}

	engine := &routing.Engine{}
	result, err := engine.Resolve(context.Background(), routing.RequestContext{Attributes: attrs, Config: cfg})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.Destination != "https://match.example" {
		t.Fatalf("classified attributes didn't satisfy the stream set filter, got %+v (attrs: %v)", result, attrs)
	}
}
