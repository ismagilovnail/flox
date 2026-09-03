package maxmind

import "testing"

type subdivision = struct {
	Names     map[string]string `maxminddb:"names"`
	IsoCode   string            `maxminddb:"iso_code"`
	GeoNameID uint              `maxminddb:"geoname_id"`
}

func TestRegionOfPrefersIsoCode(t *testing.T) {
	subs := []subdivision{{IsoCode: "ENG", Names: map[string]string{"en": "England"}}}
	if got := regionOf(subs); got != "ENG" {
		t.Fatalf("regionOf = %q, want ENG", got)
	}
}

// DB-IP Lite's free tier leaves IsoCode empty — regionOf must still
// surface the English name rather than an empty region.
func TestRegionOfFallsBackToNameWhenIsoCodeEmpty(t *testing.T) {
	subs := []subdivision{{IsoCode: "", Names: map[string]string{"en": "California"}}}
	if got := regionOf(subs); got != "California" {
		t.Fatalf("regionOf = %q, want California", got)
	}
}

func TestRegionOfEmptyWhenNoSubdivisions(t *testing.T) {
	if got := regionOf(nil); got != "" {
		t.Fatalf("regionOf(nil) = %q, want empty", got)
	}
}

// A country with nested subdivisions (e.g. London -> England -> West
// Berkshire) reports the top-level one, not the more specific one nested
// after it.
func TestRegionOfUsesTopLevelSubdivision(t *testing.T) {
	subs := []subdivision{
		{IsoCode: "ENG", Names: map[string]string{"en": "England"}},
		{IsoCode: "WBK", Names: map[string]string{"en": "West Berkshire"}},
	}
	if got := regionOf(subs); got != "ENG" {
		t.Fatalf("regionOf = %q, want ENG (top-level, not the nested WBK)", got)
	}
}
