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
    geoip/                   DB-IP databases kept current, copied into every tracker pod
    backup/                  nightly ClickHouse + files-bucket backups, monthly restore drill
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
| `edge` | Deployment + LoadBalancer | 2 | TCP | Caddy, on-demand TLS for tracking domains; certificates on one ReadWriteMany volume shared by the replicas |
| `nats` | StatefulSet | 3 | `/healthz?js-enabled-only=true` | JetStream, 20 Gi each (`max_file_store` 18G ≥ the 10 GiB stream), lame-duck on stop; clients log in (`NATS_PASSWORD`), so do the routes between servers (`NATS_ROUTE_PASSWORD`); NetworkPolicy: tracker/worker only |
| `geoip` | Deployment | 1 | ready `/ready` once both databases are there | downloads DB-IP Lite monthly, serves it to tracker pods; a sidecar in each tracker pod copies a new month in, the tracker swaps it in without a restart |
| `backup` | Deployment | 1 | `/health` | `flox-backup serve` with `BACKUP_POSTGRES=off`: ClickHouse + files bucket nightly, drill monthly; `/metrics` |
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
enforces NetworkPolicy, a default StorageClass, a StorageClass offering
ReadWriteMany (the edge's certificates: EFS, Filestore, Azure Files, NFS,
CephFS, Longhorn RWX — `example-rwx` in the overlay), a LoadBalancer
implementation for the edge, and egress to `download.db-ip.com` (GeoIP).
Outside it: Postgres, ClickHouse, Redis (with
`maxmemory` + `allkeys-lru`, `docs/deployment.md`), S3 — managed or
operator-run, with their own backups.

1. Build and push the images (`docs/deployment.md`, Build) — including
   `api-migrate` (`--target migrate`), `backup-remote`
   (`apps/cmd/backup/Dockerfile --target backup-remote`) and `web` with
   `NEXT_PUBLIC_API_URL=https://api.<domain>`.
2. Copy `overlays/production`, replace every `example` value (hosts, the
   backup bucket, the RWX StorageClass), set the image registry and tag
   in `images/`.
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

`deploy.sh` order: namespace, config, data stores, NATS and the GeoIP
server (waits for every StatefulSet and for `geoip` to hold both
databases) → migration Job (waits for success, prints its log, stops on
failure) → workloads (waits for every rollout).

## Backups

Postgres here is managed, and its provider's point-in-time recovery is
its backup: WAL-G (`docs/backup.md`) needs the server's data directory and
`archive_command`, which a managed Postgres does not give. Turn PITR on
and set its retention at the provider.

ClickHouse and the files bucket are `components/backup`: the same
`flox-backup serve` as the compose sidecar with `BACKUP_POSTGRES=off`, in
the 33 MB `backup-remote` image (no WAL-G, no Postgres). Nightly at
`BACKUP_AT`, the restore drill on `BACKUP_DRILL_DAY`, everything into
`BACKUP_S3_BUCKET` — off-site, created beforehand with versioning or
object lock. The ClickHouse server writes its own backup (`BACKUP … TO
S3`), so it must reach `BACKUP_S3_ENDPOINT` (or set
`BACKUP_CLICKHOUSE_S3_ENDPOINT`). By hand:

```
kubectl -n flox exec deploy/backup -- flox-backup now
kubectl -n flox exec deploy/backup -- flox-backup drill
kubectl -n flox exec deploy/backup -- flox-backup list
```

## GeoIP

`components/geoip`: one `geoip` pod keeps DB-IP Lite's city and ASN
databases on a 1 Gi volume (`update.sh`: daily check, a download when the
month changes, a database replaced only by a complete one) and serves
them in the cluster. Each tracker pod copies them in an init container
before it starts — one vendor download a month, not one per pod, and an
HPA scale-up copies ~130 MB over the pod network. The init container is
best effort, like GeoIP in the tracker itself: with `geoip` unreachable
the pod starts without geo and both logs say so.

A new month needs no restart. The `geoip-sync` sidecar (`sync.sh loop`)
asks the server for `city.month`/`asn.month` hourly and, when they differ
from the pod's, downloads that database and renames it into place; the
tracker checks its files every minute and swaps a changed one in while
clicks go on (log `geoip database loaded`, with the build date). A pod
that started without geo picks it up the same way once `geoip` has it.
DB-IP Lite is CC BY 4.0 — `docs/geoip.md` has the attribution. For
MaxMind instead, replace the `update` container with `geoipupdate`
writing `city.mmdb` and `asn.mmdb` (and a `city.month`/`asn.month` that
changes with each release) to the same volume.

## NATS

Clients log in: user `flox`, password `NATS_PASSWORD` from the Secret,
`NATS_URL=nats://flox:<password>@nats:4222` with the same password (hex or
alphanumeric — it sits in a URL). The client log prints the URL with the
password masked. Routes between the servers (6222) log in too: user
`route`, password `NATS_ROUTE_PASSWORD` (letters and digits — NATS takes
no variables in a route URL, so the pod writes its config with the
password filled in, to its own `/var/run/nats`, and refuses to start when
the key is missing or has other characters). The NetworkPolicy keeps
that port to the NATS pods as well. One server (`nats-single`) has no
routes and needs no route password. Changing a password: update the
keys, then `deploy.sh` — NATS restarts first, and tracker events go
through the Postgres fallback until the trackers roll. A new route
password reaches the servers one at a time: servers on the old and the
new one do not connect, so the stream can lose its quorum for a moment
mid-roll — tracker events then take the Postgres fallback too.

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

The minikube overlay also runs `geoip`, `backup` (into the same MinIO,
bucket `flox-backups`, as the compose stack) and two edge replicas on one
volume (minikube's hostpath provisioner accepts ReadWriteMany on its one
node).

## Monitoring

Pods carry `prometheus.io/scrape|port|path` annotations; with the
Prometheus Operator add `components/prometheus-operator` (a PodMonitor
for api, tracker, worker and backup). `infra/alerts.yml` matches
`job="flox-api"`, `flox-tracker`, `flox-worker`, `flox-backup`: the
PodMonitor sets `job` to `flox-<app>` for that, and an annotation-based
scrape config needs the same relabeling, or the down alerts never fire.
Alerts are `infra/alerts.yml`: load it as `rule_files`, or wrap its
`groups:` in a `PrometheusRule`. NATS serves its own monitoring on `:8222`
(Service `nats`, port `monitor`); prometheus-nats-exporter turns it into
metrics.

## Upgrading from a single-replica edge

The certificate volume was `edge-certificates` (ReadWriteOnce); a claim's
access mode cannot change, so the shared one is a new claim,
`edge-certificates-shared`. Without copying, Caddy issues every tracking
domain's certificate again on its next visit (fine for a few domains;
Let's Encrypt rate-limits many at once). To keep them, before deploying:
scale `edge` to 0, run a pod mounting both claims, `cp -a` the old `/data`
into the new one, then deploy and delete `edge-certificates`.

## Upgrading to route login and GeoIP without restarts

Add `NATS_ROUTE_PASSWORD` to the Secret before deploying (`openssl rand
-hex 24`) — without it the three NATS servers refuse to start. `kubectl
apply` does not delete what a manifest no longer has, so remove the
monthly restart job once:

```
kubectl -n flox delete cronjob,rolebinding,role,serviceaccount geoip-restart-tracker
```

## Known gaps

- **The edge needs ReadWriteMany storage.** On a cluster without it, set
  the edge to 1 replica and the volume to ReadWriteOnce (a patch in the
  overlay) — that loses the second replica, nothing else.
