# Kubernetes (tech audit DO3)

Manifests in `infra/k8s`, built with Kustomize (`kubectl apply -k`, no
Helm needed). `docs/deployment.md` stays the platform-neutral reference;
this is its Kubernetes form.

```
infra/k8s/
  base/                      api, tracker, worker, web, edge (Caddy), Ingress, flox-config
  migrate/                   the goose migration Job
  components/
    nats/                    NATS JetStream, 3-server cluster (StatefulSet)
    nats-single/             … reduced to one server
    dev-datastores/          throwaway Postgres, ClickHouse, Redis, MinIO (minikube only)
    prometheus-operator/     PodMonitor
  overlays/
    production/              template: your registry, hosts, external stores
    minikube/                everything in a local cluster
  deploy.sh                  ordered deploy of one overlay
  minikube.sh                up | forward | down for the local cluster
  check.sh                   render every overlay (+ server dry run with KUBE_CONTEXT)
  secrets.env.example        keys of the Secret flox-secrets
```

## What runs

| Workload | Kind | Replicas | Probes | Notes |
| --- | --- | --- | --- | --- |
| `api` | Deployment + HPA 2–6 | 2 | ready `/ready`, live `/health` | Ingress `api.<domain>/v1` only — `/metrics`, `/health` stay inside |
| `tracker` | Deployment + HPA 3–20 | 3 | ready `/ready` (503 while it can serve nothing or shuts down) | spread over nodes and zones; HPA at 60% CPU, scales up at once, down after 5 min; grace 40 s |
| `worker` | Deployment | 2 | `/health` | replicas share the queues and the NATS consumer; grace 60 s |
| `web` | Deployment + HPA 2–6 | 2 | `/api/health` | Ingress `app.<domain>` |
| `edge` | Deployment + LoadBalancer | 1 | TCP | Caddy, on-demand TLS for tracking domains; certificates on a PVC |
| `nats` | StatefulSet | 3 | `/healthz?js-enabled-only=true` | JetStream, 20 Gi each (`max_file_store` 18G ≥ the 10 GiB stream), lame-duck on stop, NetworkPolicy: tracker/worker only |
| `migrate` | Job | — | — | before every rollout, never inside pods (goose has no lock); a prebuilt goose + the migrations, ~50 MB |

Every FLOX container runs as uid 10001, non-root, read-only root
filesystem, no capabilities, `RuntimeDefault` seccomp. Rolling updates
never drop below the replica count (`maxUnavailable: 0`); a 5 s `preStop`
lets the Service drop a pod before it stops. PodDisruptionBudgets keep
node drains from taking several replicas at once.

Configuration: `flox-config` (ConfigMap, non-secret, `base/config.env`,
overlays merge) and `flox-secrets` (Secret, never in git). Each workload
gets only the secret keys it uses — the tracker never sees the ad-account
or TOTP keys.

## Production

Needs in the cluster: an ingress controller (class `nginx`), cert-manager
with a ClusterIssuer `letsencrypt`, metrics-server (HPA), a CNI that
enforces NetworkPolicy, a default StorageClass, and a LoadBalancer
implementation for the edge. Outside it: Postgres, ClickHouse, Redis (with
`maxmemory` + `allkeys-lru`, `docs/deployment.md`), S3 — managed or
operator-run, with their own backups.

1. Build and push the images (`docs/deployment.md`, Build) — including
   `api-migrate` (`--target migrate`) and `web` with
   `NEXT_PUBLIC_API_URL=https://api.<domain>`.
2. Copy `overlays/production`, replace every `example.com`, set the image
   registry and tag in `images/`.
3. Create the Secret from a file kept out of git:
   ```
   kubectl create namespace flox
   kubectl -n flox create secret generic flox-secrets --from-env-file=secrets.env
   ```
   (keys and examples: `infra/k8s/secrets.env.example`; or External
   Secrets / Sealed Secrets producing the same Secret).
4. `infra/k8s/deploy.sh infra/k8s/overlays/production`
5. Point `app.` and `api.` at the ingress, and `TRACKER_DNS_TARGET` at the
   edge Service's external address; ports 80/443 of the edge must be open
   to the internet (ACME).

Each release: bump the tag in `images/`, run `deploy.sh` again — it
replaces the migration Job, waits for it, then rolls the workloads.

`deploy.sh` order: namespace, config, data stores and NATS (waits for
every StatefulSet) → migration Job (waits for success, prints its log,
stops on failure) → workloads (waits for every rollout).

## Local cluster (minikube)

```
infra/k8s/minikube.sh up        # profile "flox": cluster, images built inside it, deploy
infra/k8s/minikube.sh forward   # web :3000, api :8080, tracker :8081, minio :9000
infra/k8s/minikube.sh down      # delete the profile
```

Runs in its own minikube profile and always passes `--context flox`, so
other clusters and the current kubectl context are left alone (in Lens
or k9s, pick the `flox` context and namespace). Uses
`NODE_ENV=development` (plain HTTP through port-forward), one NATS server
(4G store, so the stream is `NATS_STREAM_MAX_MB=3072`),
`components/dev-datastores`, the metrics-server addon (HPAs) and the dev
secrets the compose files use.

MinIO no longer publishes public images (`minio/minio` and
`quay.io/minio/minio` refuse pulls), so the manifest pins
`RELEASE.2025-09-07T16-13-09Z` and `up` loads it from the host's Docker
cache — the image the compose stack pulled earlier. A machine without it
cached cannot run the MinIO part (the compose files have the same
dependency).

A StatefulSet pod that never became Ready (an image it could not pull)
is not replaced when the template changes: delete the pod
(`kubectl --context flox -n flox delete pod minio-0`) and run `deploy.sh`
again.

Verified 2026-10-10: `up` from scratch, migrations to version 67, every
rollout, health endpoints through `forward`, dev fixtures, clicks through
the tracker → NATS `FLOX_EVENTS` → worker → ClickHouse, a postback to
`CPA_ACCEPT`.

## Monitoring

Pods carry `prometheus.io/scrape|port|path` annotations; with the
Prometheus Operator add `components/prometheus-operator` (a PodMonitor).
Alerts are `infra/alerts.yml`: load it as `rule_files`, or wrap its
`groups:` in a `PrometheusRule`. NATS serves its own monitoring on `:8222`
(Service `nats`, port `monitor`); prometheus-nats-exporter turns it into
metrics.

## Known gaps

- **Backups.** `apps/cmd/backup` takes Postgres base backups with WAL-G
  from the live data directory, so it only runs next to a self-hosted
  Postgres (compose). On Kubernetes with a managed Postgres the provider's
  PITR covers Postgres; ClickHouse (`BACKUP … TO S3`) and the files bucket
  have no scheduled backup here yet.
- **One edge replica.** More need shared certificate storage for Caddy.
- **GeoIP.** The tracker mounts an empty `/geoip`; mount the MaxMind
  databases (a PVC kept current by geoipupdate) to get country and ASN.
- **No NATS authentication**, only the NetworkPolicy. Add NATS users and
  put the credentials in `NATS_URL` for clusters without NetworkPolicy.
