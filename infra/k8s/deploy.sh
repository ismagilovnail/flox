#!/usr/bin/env bash
# Deploys one overlay in the order FLOX needs (docs/kubernetes.md):
#   1. namespace, config, Secret (if the overlay has one), data stores, NATS
#   2. the Postgres migration Job — waits for it to succeed
#   3. the workloads, then waits for every rollout
#
#   infra/k8s/deploy.sh infra/k8s/overlays/production
#   KUBE_CONTEXT=flox infra/k8s/deploy.sh infra/k8s/overlays/minikube
set -euo pipefail

overlay=${1:?usage: deploy.sh <overlay dir>}
ns=flox
kc=(kubectl)
[[ -n "${KUBE_CONTEXT:-}" ]] && kc+=(--context "$KUBE_CONTEXT")
apps='app.kubernetes.io/name notin (api,tracker,worker,web,edge)'

echo "==> context: $("${kc[@]}" config current-context 2>/dev/null || true)${KUBE_CONTEXT:+ (forced: $KUBE_CONTEXT)}"

echo "==> 1/3 namespace, config, stores"
"${kc[@]}" apply -k "$overlay" -l "$apps"
if ! "${kc[@]}" -n "$ns" get secret flox-secrets >/dev/null 2>&1; then
  echo "Secret $ns/flox-secrets is missing — create it first (infra/k8s/secrets.env.example)." >&2
  exit 1
fi
for sts in $("${kc[@]}" -n "$ns" get statefulset -o name); do
  "${kc[@]}" -n "$ns" rollout status "$sts" --timeout=10m
done

echo "==> 2/3 migrations"
"${kc[@]}" -n "$ns" delete job migrate --ignore-not-found --wait=true
"${kc[@]}" apply -k "$overlay/migrate"
for _ in $(seq 1 180); do
  succeeded=$("${kc[@]}" -n "$ns" get job migrate -o jsonpath='{.status.succeeded}')
  failed=$("${kc[@]}" -n "$ns" get job migrate -o jsonpath='{.status.conditions[?(@.type=="Failed")].status}')
  if [[ "$succeeded" == "1" ]]; then break; fi
  if [[ "$failed" == "True" ]]; then
    "${kc[@]}" -n "$ns" logs job/migrate --tail=50 >&2 || true
    echo "migration Job failed" >&2
    exit 1
  fi
  sleep 5
done
[[ "$succeeded" == "1" ]] || { echo "migration Job did not finish in 15 minutes" >&2; exit 1; }
"${kc[@]}" -n "$ns" logs job/migrate --tail=3

echo "==> 3/3 workloads"
"${kc[@]}" apply -k "$overlay"
for d in api tracker worker web edge; do
  "${kc[@]}" -n "$ns" rollout status "deployment/$d" --timeout=10m
done
echo "==> done"
