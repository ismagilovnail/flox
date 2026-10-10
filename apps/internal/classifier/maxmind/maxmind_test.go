package maxmind_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
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

// copyFile puts src at dst the way the GeoIP updaters do: written beside
// it, then renamed over it.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst+".tmp", b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dst+".tmp", dst); err != nil {
		t.Fatal(err)
	}
}

// A tracker started before the databases arrived gets geo once they do,
// without a restart; a file replaced on disk is swapped in.
func TestGeoProviderReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "city.mmdb")
	p := maxmind.LazyGeoProvider(path)
	defer p.Close()
	ip := net.ParseIP("81.2.69.160")

	if got, err := p.Lookup(context.Background(), ip); err != nil || got != (classifier.GeoResult{}) {
		t.Fatalf("Lookup before any file = %+v, %v; want empty, no error", got, err)
	}
	if changed, err := p.Reload(); err == nil || changed {
		t.Fatalf("Reload without a file = %v, %v; want an error", changed, err)
	}
	if !p.Built().IsZero() {
		t.Fatalf("Built with nothing loaded = %v, want zero", p.Built())
	}

	copyFile(t, "testdata/GeoLite2-City-Test.mmdb", path)
	if changed, err := p.Reload(); err != nil || !changed {
		t.Fatalf("Reload with the file = %v, %v; want changed", changed, err)
	}
	if got, _ := p.Lookup(context.Background(), ip); got.City != "London" {
		t.Fatalf("Lookup after load = %+v, want London", got)
	}
	if p.Built().IsZero() {
		t.Fatal("Built after load is zero")
	}
	if changed, err := p.Reload(); err != nil || changed {
		t.Fatalf("Reload of the same file = %v, %v; want unchanged", changed, err)
	}

	// Lookups go on while the file is replaced; nothing reads a closed mapping
	// (go test -race).
	var wg sync.WaitGroup
	done := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				if got, err := p.Lookup(context.Background(), ip); err != nil || got.City != "London" {
					t.Errorf("Lookup during reload = %+v, %v", got, err)
					return
				}
			}
		}()
	}
	for i := 0; i < 5; i++ {
		copyFile(t, "testdata/GeoLite2-City-Test.mmdb", path)
		if changed, err := p.Reload(); err != nil || !changed {
			t.Errorf("Reload of a replaced file = %v, %v; want changed", changed, err)
		}
	}
	close(done)
	wg.Wait()

	// A broken replacement keeps the database already loaded.
	if err := os.WriteFile(path+".tmp", []byte("not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Reload(); err == nil {
		t.Fatal("Reload of a broken file: want an error")
	}
	if got, _ := p.Lookup(context.Background(), ip); got.City != "London" {
		t.Fatalf("Lookup after a broken replacement = %+v, want London still", got)
	}
}

func TestASNProviderReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "asn.mmdb")
	p := maxmind.LazyASNProvider(path)
	defer p.Close()
	ip := net.ParseIP("1.0.0.0")

	if got, err := p.Lookup(context.Background(), ip); err != nil || got != (classifier.ASNResult{}) {
		t.Fatalf("Lookup before any file = %+v, %v; want empty, no error", got, err)
	}
	copyFile(t, "testdata/GeoLite2-ASN-Test.mmdb", path)
	if changed, err := p.Reload(); err != nil || !changed {
		t.Fatalf("Reload with the file = %v, %v; want changed", changed, err)
	}
	if got, _ := p.Lookup(context.Background(), ip); got.ASN != "AS15169" {
		t.Fatalf("Lookup after load = %+v, want AS15169", got)
	}
}
