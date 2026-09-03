# infra/geoip

Drop MaxMind GeoLite2 `.mmdb` files here to enable real geo/ASN
classification in the dev stack (see `docs/geoip.md`):

- `GeoLite2-City.mmdb`
- `GeoLite2-ASN.mmdb`

Both are `.gitignore`d (license-restricted — download them with your own
free MaxMind account, never commit them). This directory is bind-mounted
read-only into the `tracker` service at `/geoip`
(`infra/docker-compose.dev.yml`); until the files exist here, the tracker
falls back to its Noop geo/ASN providers automatically (a warning log at
startup, not a crash).
