#!/usr/bin/env bash
# The whole FLOX stack in a local minikube profile `flox` (docs/kubernetes.md).
#   infra/k8s/minikube.sh up       start the cluster, build images into it, deploy
#   infra/k8s/minikube.sh forward  port-forward web :3000, api :8080, tracker :8081, minio :9000
#   infra/k8s/minikube.sh down     delete the profile and everything in it
# Never touches your other kube contexts: every call names --context flox.
set -euo pipefail
cd "$(dirname "$0")/../.."
profile=flox

case "${1:-}" in
  up)
    minikube status -p "$profile" >/dev/null 2>&1 ||
      minikube start -p "$profile" --driver="${MINIKUBE_DRIVER:-kvm2}" --cpus=6 --memory=8192 --disk-size=40g --keep-context
    # HPAs need CPU metrics.
    minikube -p "$profile" addons enable metrics-server >/dev/null
    eval "$(minikube -p "$profile" docker-env)"
    docker build -q -f apps/api/Dockerfile --target api -t flox/api:local apps/
    docker build -q -f apps/api/Dockerfile --target migrate -t flox/api-migrate:local apps/
    docker build -q -f apps/tracker/Dockerfile --target tracker -t flox/tracker:local apps/
    docker build -q -f apps/worker/Dockerfile --target worker -t flox/worker:local apps/
    # MinIO has no public images any more: take the one the compose stack
    # already pulled (docker images minio/minio) under its release tag.
    minio=minio/minio:RELEASE.2025-09-07T16-13-09Z
    if ! docker image inspect "$minio" >/dev/null 2>&1; then
      eval "$(minikube -p "$profile" docker-env --unset)"
      docker image inspect "$minio" >/dev/null 2>&1 ||
        docker tag minio/minio:latest "$minio" 2>/dev/null ||
        { echo "no local $minio (or minio/minio:latest) to load into minikube" >&2; exit 1; }
      minikube -p "$profile" image load "$minio"
      eval "$(minikube -p "$profile" docker-env)"
    fi
    docker build -q -f apps/web/Dockerfile -t flox/web:local --build-arg NEXT_PUBLIC_API_URL=http://localhost:8080 apps/web
    KUBE_CONTEXT="$profile" infra/k8s/deploy.sh infra/k8s/overlays/minikube
    ;;
  forward)
    k=(kubectl --context "$profile" -n flox port-forward)
    "${k[@]}" svc/web 3000:3000 & "${k[@]}" svc/api 8080:8080 &
    "${k[@]}" svc/tracker 8081:8081 & "${k[@]}" svc/minio 9000:9000 &
    trap 'kill $(jobs -p)' EXIT
    wait
    ;;
  down)
    minikube delete -p "$profile"
    ;;
  *)
    echo "usage: $0 up|forward|down" >&2
    exit 2
    ;;
esac
