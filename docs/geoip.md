# Geo/ASN provider

`apps/internal/classifier.GeoProvider`/`ASNProvider` (§74/§75, CLAUDE.md
non-negotiable #11 — "providers behind interfaces, no vendor lock-in")
had only `NoopGeoProvider`/`NoopASNProvider` until this phase — country/
city/ASN always rendered empty for real traffic (see `docs/click-log.md`,
which surfaced this gap: the Click Log page showed real IPs but `—` for
geo). `apps/internal/classifier/maxmind` is the real implementation,
against local `.mmdb` files (`github.com/oschwald/geoip2-golang`) — the
package name is `maxmind`, but it reads **either** MaxMind's own
GeoLite2/GeoIP2 databases **or** DB-IP's, unmodified; `geoip2-golang`
recognizes both formats natively.

## Currently configured: DB-IP Lite (no registration)

The dev stack ships pointed at **DB-IP Lite** — `infra/docker-compose.
dev.yml`'s default `GEOIP_CITY_DB_PATH`/`GEOIP_ASN_DB_PATH` are `/geoip/
dbip-city-lite.mmdb` / `/geoip/dbip-asn-lite.mmdb`. Chosen because MaxMind
signup blocked this account's IP (VPN and non-VPN both); DB-IP requires
no account at all and downloads directly over HTTPS:

```
https://download.db-ip.com/free/dbip-city-lite-<YYYY-MM>.mmdb.gz
https://download.db-ip.com/free/dbip-asn-lite-<YYYY-MM>.mmdb.gz
```

(`<YYYY-MM>` is the current year-month — the files are republished
monthly under a new name; re-download periodically for fresher data, same
staleness tradeoff MaxMind's own free tier has.) License: Creative
Commons Attribution 4.0 — free for any use, including commercial,
provided DB-IP is credited somewhere in the product (a footer/about/
credits page) if this is ever shown to people outside your own org; see
https://db-ip.com/db/lite.php for the exact attribution text.

To refresh or switch to real MaxMind data instead, place the files (any
name — just match the two `GEOIP_*_DB_PATH` env vars) in `infra/geoip/`
and restart the `tracker` service:
`docker compose -f infra/docker-compose.dev.yml restart tracker`.

## Alternative: MaxMind GeoLite2 (more accurate, needs a free account)

MaxMind's databases are somewhat more accurate than DB-IP's free Lite
tier, but require a MaxMind account + license key — their EULA prohibits
redistributing the actual files, so neither this ships in the repo.

1. Create a free account: https://www.maxmind.com/en/geolite2/signup
2. **My Account → Manage License Keys** → generate one.
3. **My Account → Download Databases** (or a direct
   `https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-City&license_key=<key>&suffix=tar.gz`
   URL) — get `GeoLite2-City` and `GeoLite2-ASN` in `.mmdb` format, not
   CSV.
4. Place both files in `infra/geoip/` and update
   `infra/docker-compose.dev.yml`'s two `GEOIP_*_DB_PATH` values (or just
   name the files `dbip-city-lite.mmdb`/`dbip-asn-lite.mmdb` to match the
   existing defaults — the reader doesn't care about the filename or
   vendor, only the file's own internal `DatabaseType` metadata).
5. Restart the `tracker` service.

Neither this repo nor this session created a MaxMind account — that step
is the user's own to take if ever wanted; DB-IP Lite is what's actually
live right now.

## Kubernetes

`infra/k8s/components/geoip` downloads DB-IP Lite once a month inside the
cluster and copies it into each tracker pod at start, as `city.mmdb` and
`asn.mmdb` (`docs/kubernetes.md`, GeoIP).

## `infra/geoip/` is gitignored either way

Both vendors' terms require re-downloading rather than redistributing —
`.gitignore` excludes `infra/geoip/*.mmdb` regardless of which vendor's
files are in there. This does NOT cover
`apps/internal/classifier/maxmind/testdata/*.mmdb` — MaxMind's own public
CC0/Apache test fixtures, real test data that belongs in git.

## Behavior when unset or unreadable

Best-effort at startup, the same stance `apps/tracker/main.go` already
takes for ClickHouse/Redis (CLAUDE.md #9 — the redirect hot path must
never be blocked by a dependency): a missing/invalid path logs a warning
and falls back to `NoopGeoProvider`/`NoopASNProvider`, never crashes the
tracker. A per-request lookup miss (an IP absent from the database — e.g.
private/reserved ranges) is not an error either; it returns the same
empty result a Noop provider would, not a fabricated value (CLAUDE.md's
"no fake APIs that look real"). DB-IP Lite's free tier also leaves
subdivision `IsoCode` empty (unlike GeoLite2) — `regionOf` in
`apps/internal/classifier/maxmind/maxmind.go` falls back to the English
name in that case rather than dropping region data that exists.

## Verified

`apps/internal/classifier/maxmind`'s unit tests run against MaxMind's own
public test fixtures (`testdata/*.mmdb`, from the
[MaxMind-DB](https://github.com/maxmind/MaxMind-DB) repo, Apache-2.0,
committed — synthetic sample data, needs no license key): real IP →
country/region/city and IP → ASN lookups against known fixture values,
the "IP not in database ⇒ empty result, not an error" case, and
`regionOf`'s ISO-code/name-fallback logic. `go build/vet/test -count=1
./...` clean.

Real, live DB-IP Lite data was verified end-to-end against the actual dev
stack: downloaded the two files into `infra/geoip/`, recreated the
`tracker` container, hit a real tracking link (`curl` with a spoofed
`X-Forwarded-For: 8.8.8.8`) through the real redirect path, confirmed the
resulting `click_events` row in ClickHouse carried `country=US,
region=California, city=Mountain View, asn=AS15169` — correct for that
IP — and confirmed the same row renders correctly in the Click Log page
in a real browser session. This is the databases' live, current state in
`infra/geoip/`, not a fixture that was cleaned up afterward.
