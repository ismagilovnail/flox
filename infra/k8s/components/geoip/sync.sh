#!/bin/sh
# Copies the GeoIP databases from the geoip server (update.sh) into
# /geoip, the tracker's volume, whenever the server has a month this pod
# lacks. Each file is renamed into place complete; the tracker notices the
# new file within a minute and swaps it in (docs/geoip.md). Best effort:
# with the server unreachable the tracker keeps what it has, or runs
# without geo. `sync.sh` copies once (init container), `sync.sh loop`
# checks hourly (sidecar).
set -u
server=http://geoip:8080
sync() {
  for f in city asn; do
    month=$(wget -q -T 10 -O - "$server/$f.month") || month=
    have=$(cat "/geoip/$f.month" 2>/dev/null || true)
    [ -n "$month" ] && [ "$month" = "$have" ] && [ -s "/geoip/$f.mmdb" ] && continue
    if wget -q -T 120 -O "/geoip/$f.tmp" "$server/$f.mmdb" && [ -s "/geoip/$f.tmp" ]; then
      mv "/geoip/$f.tmp" "/geoip/$f.mmdb" && echo "$month" > "/geoip/$f.month" &&
        echo "geoip: $f ${month:-?} copied from $server"
    else
      rm -f "/geoip/$f.tmp"
      echo "geoip: no $f database from $server${have:+ (keeping $have)}" >&2
    fi
  done
}
sync
[ "${1:-}" = loop ] || exit 0
while sleep 3600; do sync; done
