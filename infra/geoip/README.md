# infra/geoip

Real geo/ASN `.mmdb` databases live here (see `docs/geoip.md`). Both
files are `.gitignore`d — license-restricted, downloaded per-deployment,
never committed.

**Currently configured: DB-IP Lite** (no registration needed, CC BY 4.0):

- `dbip-city-lite.mmdb`
- `dbip-asn-lite.mmdb`

Re-download monthly for fresher data:

```
curl -o dbip-city-lite.mmdb.gz "https://download.db-ip.com/free/dbip-city-lite-$(date +%Y-%m).mmdb.gz" && gunzip -f dbip-city-lite.mmdb.gz
curl -o dbip-asn-lite.mmdb.gz  "https://download.db-ip.com/free/dbip-asn-lite-$(date +%Y-%m).mmdb.gz"  && gunzip -f dbip-asn-lite.mmdb.gz
```

To use MaxMind GeoLite2 instead (needs a free MaxMind account — see
`docs/geoip.md`), drop the files in here under these same two names (the
reader detects the vendor from the file's own metadata, not the
filename) and restart the `tracker` service.

This directory is bind-mounted read-only into the `tracker` service at
`/geoip` (`infra/docker-compose.dev.yml`); if the configured files don't
exist, the tracker falls back to its Noop geo/ASN providers automatically
(a warning log at startup, not a crash).
