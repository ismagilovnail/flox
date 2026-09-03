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
// file once at startup), not network calls — safe on the tracker's hot
// path (CLAUDE.md #9). An IP simply absent from the database is not an
// error in maxminddb's own Lookup semantics (it leaves the result
// zero-valued) — this package only returns an error for a genuine
// read/decode failure, never for "not found," so a lookup miss degrades
// to the same empty GeoResult/ASNResult the Noop providers already
// return rather than breaking classification for that visit.
package maxmind

import (
	"context"
	"fmt"
	"net"

	"github.com/oschwald/geoip2-golang"

	"github.com/ismagilovnail/flox/apps/internal/classifier"
)

// GeoProvider reads country/region/city from a GeoLite2-City (or
// GeoIP2-City) database.
type GeoProvider struct {
	reader *geoip2.Reader
}

// NewGeoProvider opens the .mmdb at path. The file stays memory-mapped
// for the life of the returned provider — call Close on shutdown.
func NewGeoProvider(path string) (*GeoProvider, error) {
	reader, err := geoip2.Open(path)
	if err != nil {
		return nil, fmt.Errorf("maxmind: opening city database %q: %w", path, err)
	}
	return &GeoProvider{reader: reader}, nil
}

func (p *GeoProvider) Close() error { return p.reader.Close() }

func (p *GeoProvider) Lookup(_ context.Context, ip net.IP) (classifier.GeoResult, error) {
	if ip == nil {
		return classifier.GeoResult{}, nil
	}
	record, err := p.reader.City(ip)
	if err != nil {
		return classifier.GeoResult{}, fmt.Errorf("maxmind: city lookup: %w", err)
	}
	var region string
	if len(record.Subdivisions) > 0 {
		region = record.Subdivisions[0].IsoCode
	}
	return classifier.GeoResult{
		Country: record.Country.IsoCode,
		Region:  region,
		City:    record.City.Names["en"],
	}, nil
}

// ASNProvider reads the autonomous system number from a GeoLite2-ASN (or
// GeoIP2-ISP) database.
type ASNProvider struct {
	reader *geoip2.Reader
}

// NewASNProvider opens the .mmdb at path. The file stays memory-mapped
// for the life of the returned provider — call Close on shutdown.
func NewASNProvider(path string) (*ASNProvider, error) {
	reader, err := geoip2.Open(path)
	if err != nil {
		return nil, fmt.Errorf("maxmind: opening ASN database %q: %w", path, err)
	}
	return &ASNProvider{reader: reader}, nil
}

func (p *ASNProvider) Close() error { return p.reader.Close() }

func (p *ASNProvider) Lookup(_ context.Context, ip net.IP) (classifier.ASNResult, error) {
	if ip == nil {
		return classifier.ASNResult{}, nil
	}
	record, err := p.reader.ASN(ip)
	if err != nil {
		return classifier.ASNResult{}, fmt.Errorf("maxmind: asn lookup: %w", err)
	}
	if record.AutonomousSystemNumber == 0 {
		return classifier.ASNResult{}, nil
	}
	return classifier.ASNResult{ASN: fmt.Sprintf("AS%d", record.AutonomousSystemNumber)}, nil
}
