#!/bin/sh
# Keeps DB-IP Lite's city and ASN databases (docs/geoip.md) current in
# /data as city.mmdb and asn.mmdb. DB-IP republishes them monthly under a
# new name; early in a month the new one may not be out yet, so last month
# is tried too. A database is replaced only by a complete, non-empty one.
# Checks once a day; downloads only when the month changed.
set -u
cd /data
fetch() { # kind month
  url="https://download.db-ip.com/free/dbip-$1-lite-$2.mmdb.gz"
  rm -f "/tmp/$1.gz" "/tmp/$1.mmdb"
  wget -q -T 120 -O "/tmp/$1.gz" "$url" &&
    gunzip -c "/tmp/$1.gz" > "/tmp/$1.mmdb" &&
    [ -s "/tmp/$1.mmdb" ] &&
    mv "/tmp/$1.mmdb" "$1.mmdb.new" && mv "$1.mmdb.new" "$1.mmdb" &&
    echo "$2" > "$1.month" &&
    echo "geoip: $1 updated to $2 ($(wc -c < "$1.mmdb") bytes)"
}
while :; do
  this=$(date -u +%Y-%m)
  day=$(date -u +%d)
  last=$(date -u -d "@$(( $(date -u +%s) - ${day#0} * 86400 ))" +%Y-%m)
  for kind in city asn; do
    have=$(cat "$kind.month" 2>/dev/null || true)
    [ "$have" = "$this" ] && continue
    fetch "$kind" "$this" || { [ "$have" != "$last" ] && fetch "$kind" "$last"; } ||
      echo "geoip: no $kind database for $this${have:+ (keeping $have)}" >&2
  done
  rm -f /tmp/*.gz /tmp/*.mmdb
  # Ready once both exist: trackers copy them at start.
  [ -s city.mmdb ] && [ -s asn.mmdb ] && date -u +%FT%TZ > ready
  sleep 86400
done
