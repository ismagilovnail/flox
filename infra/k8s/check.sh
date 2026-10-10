#!/usr/bin/env bash
# Static checks for infra/k8s: every overlay renders, the edge Caddyfile is
# still infra/Caddyfile.tracker. With KUBE_CONTEXT set, also a server-side
# dry run of each overlay against that cluster (schemas, admission).
set -euo pipefail
cd "$(dirname "$0")"

cmp -s base/edge/Caddyfile ../Caddyfile.tracker ||
  { echo "base/edge/Caddyfile differs from infra/Caddyfile.tracker — copy it over" >&2; exit 1; }

for overlay in overlays/*/; do
  for dir in "$overlay" "$overlay/migrate"; do
    # Every namespaced object must land in flox: a namespace set only in
    # base misses what overlays add themselves (components, generators).
    stray=$(kubectl kustomize "$dir" | awk '
      function flush() { if (kind != "" && kind != "Namespace" && ns != "flox") print kind "/" name }
      /^---$/ { flush(); kind = name = ns = ""; meta = 0; next }
      /^kind: / { kind = $2 }
      /^metadata:/ { meta = 1; next }
      /^[^ ]/ { meta = 0 }
      meta && /^  name: / { name = $2 }
      meta && /^  namespace: / { ns = $2 }
      END { flush() }')
    [[ -z "$stray" ]] || { echo "$dir: not in namespace flox:" $stray >&2; exit 1; }
    echo "rendered $dir"
    if [[ -n "${KUBE_CONTEXT:-}" ]]; then
      kubectl --context "$KUBE_CONTEXT" apply --dry-run=server -k "$dir" >/dev/null
      echo "  server dry run ok ($KUBE_CONTEXT)"
    fi
  done
done
