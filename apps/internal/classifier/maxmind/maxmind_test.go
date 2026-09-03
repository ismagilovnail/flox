package maxmind_test

import (
	"context"
	"net"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/classifier"
	"github.com/ismagilovnail/flox/apps/internal/classifier/maxmind"
)

// The two .mmdb files under testdata/ are MaxMind's own public test
// fixtures (github.com/maxmind/MaxMind-DB test-data/, CC0-licensed sample
// data — not a real GeoLite2 database, no license key required), so this
// test needs no MaxMind account and runs against known, fixed IP->result
// mappings baked into those fixtures.

func TestGeoProviderLookup(t *testing.T) {
	p, err := maxmind.NewGeoProvider("testdata/GeoLite2-City-Test.mmdb")
	if err != nil {
		t.Fatalf("NewGeoProvider: %v", err)
	}
	defer p.Close()

	got, err := p.Lookup(context.Background(), net.ParseIP("81.2.69.160"))
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	want := classifier.GeoResult{Country: "GB", Region: "ENG", City: "London"}
	if got != want {
		t.Fatalf("Lookup(81.2.69.160) = %+v, want %+v", got, want)
	}
}

// An IP absent from the database is not an error in maxminddb's own
// Lookup semantics — this must degrade to the same empty GeoResult the
// Noop provider returns, not fail classification for the visit.
func TestGeoProviderLookupMiss(t *testing.T) {
	p, err := maxmind.NewGeoProvider("testdata/GeoLite2-City-Test.mmdb")
	if err != nil {
		t.Fatalf("NewGeoProvider: %v", err)
	}
	defer p.Close()

	got, err := p.Lookup(context.Background(), net.ParseIP("203.0.113.1"))
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != (classifier.GeoResult{}) {
		t.Fatalf("Lookup(203.0.113.1) = %+v, want the zero value (not found)", got)
	}
}

func TestASNProviderLookup(t *testing.T) {
	p, err := maxmind.NewASNProvider("testdata/GeoLite2-ASN-Test.mmdb")
	if err != nil {
		t.Fatalf("NewASNProvider: %v", err)
	}
	defer p.Close()

	got, err := p.Lookup(context.Background(), net.ParseIP("1.0.0.0"))
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	want := classifier.ASNResult{ASN: "AS15169"}
	if got != want {
		t.Fatalf("Lookup(1.0.0.0) = %+v, want %+v", got, want)
	}
}

func TestASNProviderLookupMiss(t *testing.T) {
	p, err := maxmind.NewASNProvider("testdata/GeoLite2-ASN-Test.mmdb")
	if err != nil {
		t.Fatalf("NewASNProvider: %v", err)
	}
	defer p.Close()

	got, err := p.Lookup(context.Background(), net.ParseIP("203.0.113.1"))
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != (classifier.ASNResult{}) {
		t.Fatalf("Lookup(203.0.113.1) = %+v, want the zero value (not found)", got)
	}
}

func TestNewGeoProviderInvalidPath(t *testing.T) {
	if _, err := maxmind.NewGeoProvider("testdata/does-not-exist.mmdb"); err == nil {
		t.Fatal("NewGeoProvider with a missing file: want an error, not a panic")
	}
}

// classifier.GeoProvider/ASNProvider are the interfaces
// apps/internal/classifier.Classifier composes against — this is a
// compile-time check that the real providers satisfy them, the same
// contract NoopGeoProvider/NoopASNProvider already do.
var (
	_ classifier.GeoProvider = (*maxmind.GeoProvider)(nil)
	_ classifier.ASNProvider = (*maxmind.ASNProvider)(nil)
)
