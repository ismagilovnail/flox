// Package maxmind implements classifier.GeoProvider and
// classifier.ASNProvider against local MaxMind GeoLite2/GeoIP2 .mmdb
// database files — the real vendor swapped in behind those interfaces
// per CLAUDE.md non-negotiable #11 ("providers behind interfaces, no
// vendor lock-in"). apps/internal/classifier.NoopGeoProvider/
// NoopASNProvider remain the honest default when no database path is
// configured (see docs/click-log.md's "what this does NOT change" for
// why those were still the dev default before this package existed).
//
// City and ASN come from separate MaxMind databases (GeoLite2-City.mmdb,
// GeoLite2-ASN.mmdb) — MaxMind's own split, not this package's choice —
// so this exposes two independent providers, each opening its own file.
//
// Lookups are in-memory, memory-mapped reads (geoip2.Open mmaps the
// file), not network calls — safe on the tracker's hot path (CLAUDE.md
// #9). Reload opens the file again when it changed on disk (replaced by a
// rename, as the GeoIP updaters do) and swaps it in while lookups go on;
// the old mapping is closed once no lookup holds it. An IP simply absent
// from the database is not an error in maxminddb's own Lookup semantics
// (it leaves the result zero-valued) — this package only returns an error
// for a genuine read/decode failure, never for "not found," so a lookup
// miss degrades to the same empty GeoResult/ASNResult the Noop providers
// already return rather than breaking classification for that visit.
package maxmind

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/oschwald/geoip2-golang"

	"github.com/ismagilovnail/flox/apps/internal/classifier"
)

// db is one .mmdb file, reopened when it changes on disk.
type db struct {
	path string
	kind string // "city" or "ASN", for errors

	mu     sync.RWMutex // lookups hold it read; a swap or Close holds it write
	reader *geoip2.Reader
	stamp  stamp
}

// stamp identifies a version of the file: a rename puts a new inode with
// its own mtime and size there.
type stamp struct {
	info os.FileInfo
}

func (s stamp) same(info os.FileInfo) bool {
	return s.info != nil && os.SameFile(s.info, info) &&
		s.info.ModTime().Equal(info.ModTime()) && s.info.Size() == info.Size()
}

// reload opens the file if it is new or changed since the last load.
// On any error the database already loaded stays in use.
func (d *db) reload() (bool, error) {
	info, err := os.Stat(d.path)
	if err != nil {
		return false, fmt.Errorf("maxmind: opening %s database %q: %w", d.kind, d.path, err)
	}
	d.mu.RLock()
	unchanged := d.stamp.same(info)
	d.mu.RUnlock()
	if unchanged {
		return false, nil
	}
	reader, err := geoip2.Open(d.path)
	if err != nil {
		return false, fmt.Errorf("maxmind: opening %s database %q: %w", d.kind, d.path, err)
	}
	d.mu.Lock()
	old := d.reader
	d.reader, d.stamp = reader, stamp{info: info}
	d.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return true, nil
}

// built is when the loaded database was built (its metadata), zero when
// none is loaded.
func (d *db) built() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.reader == nil {
		return time.Time{}
	}
	return time.Unix(int64(d.reader.Metadata().BuildEpoch), 0).UTC()
}

func (d *db) close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.reader == nil {
		return nil
	}
	err := d.reader.Close()
	d.reader = nil
	return err
}

// GeoProvider reads country/region/city from a GeoLite2-City (or
// GeoIP2-City, or DB-IP city) database.
type GeoProvider struct{ db *db }

// NewGeoProvider opens the .mmdb at path. The file stays memory-mapped
// until Close or until Reload swaps in a newer one.
func NewGeoProvider(path string) (*GeoProvider, error) {
	p := LazyGeoProvider(path)
	if _, err := p.Reload(); err != nil {
		return nil, err
	}
	return p, nil
}

// LazyGeoProvider loads nothing yet: lookups return the empty result
// until a Reload finds the file.
func LazyGeoProvider(path string) *GeoProvider {
	return &GeoProvider{db: &db{path: path, kind: "city"}}
}

// Reload swaps in the file at the path if it changed since the last
// load; changed reports a swap. An error keeps the current database.
func (p *GeoProvider) Reload() (changed bool, err error) { return p.db.reload() }

// Built is the loaded database's build time (zero when none is loaded).
func (p *GeoProvider) Built() time.Time { return p.db.built() }

func (p *GeoProvider) Close() error { return p.db.close() }

func (p *GeoProvider) Lookup(_ context.Context, ip net.IP) (classifier.GeoResult, error) {
	if ip == nil {
		return classifier.GeoResult{}, nil
	}
	p.db.mu.RLock()
	defer p.db.mu.RUnlock()
	if p.db.reader == nil {
		return classifier.GeoResult{}, nil
	}
	record, err := p.db.reader.City(ip)
	if err != nil {
		return classifier.GeoResult{}, fmt.Errorf("maxmind: city lookup: %w", err)
	}
	return classifier.GeoResult{
		Country: record.Country.IsoCode,
		Region:  regionOf(record.Subdivisions),
		City:    record.City.Names["en"],
	}, nil
}

// regionOf takes geoip2.City's Subdivisions and reads the top-level one
// (index 0 — e.g. "England" for a London IP, not the more specific
// county nested after it), preferring its ISO code when the database has
// one (GeoLite2 always does) and falling back to its English name
// otherwise. DB-IP Lite — a no-registration-required alternative source,
// see docs/geoip.md — leaves IsoCode empty on its free tier; the name
// fallback keeps region data that exists instead of silently dropping it.
func regionOf(subdivisions []struct {
	Names     map[string]string `maxminddb:"names"`
	IsoCode   string            `maxminddb:"iso_code"`
	GeoNameID uint              `maxminddb:"geoname_id"`
}) string {
	if len(subdivisions) == 0 {
		return ""
	}
	top := subdivisions[0]
	if top.IsoCode != "" {
		return top.IsoCode
	}
	return top.Names["en"]
}

// ASNProvider reads the autonomous system number from a GeoLite2-ASN (or
// GeoIP2-ISP, or DB-IP ASN) database.
type ASNProvider struct{ db *db }

// NewASNProvider opens the .mmdb at path. The file stays memory-mapped
// until Close or until Reload swaps in a newer one.
func NewASNProvider(path string) (*ASNProvider, error) {
	p := LazyASNProvider(path)
	if _, err := p.Reload(); err != nil {
		return nil, err
	}
	return p, nil
}

// LazyASNProvider loads nothing yet: lookups return the empty result
// until a Reload finds the file.
func LazyASNProvider(path string) *ASNProvider {
	return &ASNProvider{db: &db{path: path, kind: "ASN"}}
}

// Reload swaps in the file at the path if it changed since the last
// load; changed reports a swap. An error keeps the current database.
func (p *ASNProvider) Reload() (changed bool, err error) { return p.db.reload() }

// Built is the loaded database's build time (zero when none is loaded).
func (p *ASNProvider) Built() time.Time { return p.db.built() }

func (p *ASNProvider) Close() error { return p.db.close() }

func (p *ASNProvider) Lookup(_ context.Context, ip net.IP) (classifier.ASNResult, error) {
	if ip == nil {
		return classifier.ASNResult{}, nil
	}
	p.db.mu.RLock()
	defer p.db.mu.RUnlock()
	if p.db.reader == nil {
		return classifier.ASNResult{}, nil
	}
	record, err := p.db.reader.ASN(ip)
	if err != nil {
		return classifier.ASNResult{}, fmt.Errorf("maxmind: asn lookup: %w", err)
	}
	if record.AutonomousSystemNumber == 0 {
		return classifier.ASNResult{}, nil
	}
	return classifier.ASNResult{ASN: fmt.Sprintf("AS%d", record.AutonomousSystemNumber)}, nil
}
