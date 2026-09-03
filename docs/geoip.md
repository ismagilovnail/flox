# Geo/ASN provider (MaxMind GeoLite2)

`apps/internal/classifier.GeoProvider`/`ASNProvider` (§74/§75, CLAUDE.md
non-negotiable #11 — "providers behind interfaces, no vendor lock-in")
had only `NoopGeoProvider`/`NoopASNProvider` until this phase — country/
city/ASN always rendered empty for real traffic (see `docs/click-log.md`,
which surfaced this gap: the Click Log page showed real IPs but `—` for
geo). `apps/internal/classifier/maxmind` is the real implementation,
against local MaxMind GeoLite2 `.mmdb` files
(`github.com/oschwald/geoip2-golang`).

## Why this needs a manual step

MaxMind's GeoLite2 databases are free but require a MaxMind account and a
license key to download — their EULA prohibits redistributing the actual
database files, so **no `.mmdb` ships in this repo**, and this can't be
automated on your behalf (creating third-party accounts is out of scope
for an agent to do for you). This is the one remaining manual step; all
the code wiring is done.

## Setup

1. Create a free MaxMind account: https://www.maxmind.com/en/geolite2/signup
2. Generate a license key: **My Account → Manage License Keys**.
3. Download `GeoLite2-City` and `GeoLite2-ASN` (the `.mmdb` binary format,
   not CSV) — either from the web UI, or via MaxMind's `geoipupdate` tool
   for scripted/recurring refreshes (recommended for production — these
   databases update roughly weekly).
4. Place both files in `infra/geoip/`:
   ```
   infra/geoip/GeoLite2-City.mmdb
   infra/geoip/GeoLite2-ASN.mmdb
   ```
   (gitignored — see `infra/geoip/README.md`.)
5. Restart the `tracker` service: `docker compose -f infra/docker-compose.dev.yml restart tracker`.

No other config change is needed — `infra/docker-compose.dev.yml`
already bind-mounts `infra/geoip/` into the `tracker` container at
`/geoip` and points `GEOIP_CITY_DB_PATH`/`GEOIP_ASN_DB_PATH` at the two
files. Outside the dev compose stack (staging/production), set those two
env vars to wherever the deployment puts the files
(`.env.example` documents them).

## Behavior when unset or unreadable

Best-effort at startup, the same stance `apps/tracker/main.go` already
takes for ClickHouse/Redis (CLAUDE.md #9 — the redirect hot path must
never be blocked by a dependency): a missing/invalid path logs a warning
and falls back to `NoopGeoProvider`/`NoopASNProvider`, never crashes the
tracker. A per-request lookup miss (an IP absent from the database — e.g.
private/reserved ranges) is not an error either; it returns the same
empty result a Noop provider would, not a fabricated value (CLAUDE.md's
"no fake APIs that look real").

## Verified

`apps/internal/classifier/maxmind`'s tests run against MaxMind's own
public test fixtures (`testdata/*.mmdb`, from the
[MaxMind-DB](https://github.com/maxmind/MaxMind-DB) repo, Apache-2.0,
committed — these are synthetic sample data, not a real GeoLite2 database,
and need no license key) — real IP → country/region/city and IP → ASN
lookups against known fixture values, plus the "IP not in database ⇒
empty result, not an error" case. `go build/vet/test -count=1 ./...`
clean. Real GeoLite2 data was not obtained or tested against in this
session (no MaxMind account available) — that's the one piece that
needs your own account, per above.
