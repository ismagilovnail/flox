# Environment Documentation (§61, Phase 33)

`.env.example` at the repo root tracks the full variable surface — this
doc is what actually changes between dev, staging, and production, and
why. Every variable below is read by `apps/internal/config.Load`/
`LoadTracker`/`LoadWorker` (Go) or Next.js directly (web); nothing here is
invented for this doc.

## What actually changes behavior per environment

Grepping the Go codebase for `cfg.Env`/`NODE_ENV` turns up exactly **one**
behavior gate, in `apps/api/main.go`:

```go
authHandler := auth.NewHandler(authSvc, logger, cfg.Env != "development")
```

`cfg.Env != "development"` becomes `secureCookies` — whether the session
cookie (`apps/internal/auth/handler.go`'s `setSessionCookie`) is marked
`Secure`. That's the entire runtime behavior difference `NODE_ENV`
controls in this codebase. Everything else that differs between dev and
production is which *values* the other variables hold, not code paths.

**This has one hard operational consequence**: set `NODE_ENV=production`
(or anything other than `development`) only once TLS is actually
terminated in front of `apps/api` — a browser drops a `Secure` cookie
sent over plain HTTP outright, which breaks every authenticated request.
`docker-compose.test.yml` deliberately stays `NODE_ENV=development` (unset
→ default) for exactly this reason: it has no TLS termination in front of
it. A real deployment needs a reverse proxy/load balancer doing TLS
(`docs/deployment.md`) before flipping this.

`apps/web`'s own `NODE_ENV=production` (set inside `apps/web/Dockerfile`,
unconditionally) is standard Next.js build behavior — React production
mode, minification — not a FLOX-specific gate; it's unrelated to the
`secureCookies` question above and always correct in a built image.

## Variable reference

### App

| Variable | Dev value | Staging/production |
|---|---|---|
| `NODE_ENV` | `development` (default) | `production` — **only after** TLS is live in front of `apps/api` (see above) |
| `APP_URL` | `http://localhost:3000` | The real public URL of `apps/web`, exactly as a browser's `Origin` header will read it (`apps/internal/tenant.RequireSameOrigin`, §54 CSRF, compares this literally — scheme/host/port must match exactly, not just the hostname) |
| `VISITOR_KEY` | dev value in compose | 32+ random bytes, base64, the same on every tracker and the worker, never rotated. The key of visitors' pseudonyms for unique clicks (`internal/visitorkey`, `docs/retention.md`). Unset: unique clicks fall back to raw IP + user agent, which `RETENTION_IP_DAYS` wipes |
| `RETENTION_IP_DAYS` | `90` | Days after which visitors' IP and user agent are wiped from ClickHouse (column TTL); `0` keeps them |
| `RETENTION_SHORTEN_AFTER_DAYS` | `30` | Days an organization stays on a plan that keeps raw clicks for less time (a downgrade, a lapsed payment) before the shorter period applies to its rows (`internal/retention.Plans`, `docs/retention.md`); a longer period applies within the hour |
| `API_PUBLIC_URL` | unset | `apps/api`'s public origin as a browser reaches it — the same value as the web build's `NEXT_PUBLIC_API_URL`. Optional. Every binary refuses to start when `S3_PUBLIC_ENDPOINT` or `TRACKER_PUBLIC_URL` has the same origin (scheme, host, port) as `APP_URL` or this: uploaded landings and gallery files would run with the panel's rights (`config.checkUntrustedOrigins`, audit SEC-5) |
| `API_URL` / `TRACKER_URL` / `WORKER_URL` | `http://localhost:8080/8081/8082` | Only the **port digits** are read (`config.parsePort`) — these decide each binary's bind port, nothing else. Safe to leave as the container's internal port even behind a reverse proxy that exposes a different public URL. |
| `LOG_LEVEL` | `info` | `info` in production too; `debug` only for active incident investigation — every log line is JSON (`apps/internal/logging`), fine for either volume in most log pipelines |

**Container-internal URLs, dev.** The dev values in this reference
(`localhost:5432`, `localhost:8123`, …) are what the HOST needs — `.env`,
the goose CLI, `go test`, a browser hitting `apps/web`. Inside
`infra/docker-compose.dev.yml`'s own network, `api`/`tracker`/`worker`
reach `postgres`/`clickhouse`/`redis`/`minio`/`mailhog` by compose service
name instead (`postgres://flox:flox@postgres:5432/flox`, etc. — set as
each service's own `environment:` block in that file, not read from
`.env`), same pattern `docker-compose.test.yml` already used. No host
networking is involved anywhere in either file.

### PostgreSQL

`DATABASE_URL` — dev: the throwaway `flox`/`flox`/`flox` credentials in
`infra/docker-compose.dev.yml`. **Staging/production: a real managed
Postgres instance, real generated credentials, `sslmode=require` (or
stricter) instead of dev's implicit disable.** Never reuse the dev/test
compose passwords anywhere real — they're published in this repo.

### ClickHouse

`CLICKHOUSE_URL`/`_DATABASE`/`_USER`/`_PASSWORD` — dev runs with an empty
password (`infra/docker-compose.dev.yml`'s `CLICKHOUSE_PASSWORD: ""`) and
`CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT: 1` (lets the default user manage
other users — fine for a single-developer local container, wrong for
anything real). Production needs a real password and TLS: `https://` in
`CLICKHOUSE_URL` (`apps/internal/chconn`; before the 2026-10-06 tech audit
the scheme was ignored and an `https://` URL was dialed in plain HTTP).

**Query limits per binary** (`apps/internal/chconn`, tech audit CH4) — so
one heavy report cannot hold the ClickHouse that postback attribution
needs:

| Binary | `max_execution_time` | `max_memory_usage` |
|---|---|---|
| `apps/api` (reports) | 30 s | 4 GiB |
| `apps/tracker` (attribution, offer caps) | 10 s | 1 GiB |
| `apps/worker` (inserts, backfills, reconciliation) | none | server default |

Override per container with `CLICKHOUSE_MAX_EXECUTION_TIME` (seconds) and
`CLICKHOUSE_MAX_MEMORY_USAGE` (bytes); `0` = no limit. A report stopped
at a limit answers **422 `query_too_heavy`** ("narrow the date range or
remove a breakdown"), translated in the UI, never retried by it. With a
time limit, a query whose caller went away (the user left the page, the
API's 30 s request timeout fired) is cancelled on ClickHouse too.
Connections dial with a 5 s timeout, at most 20 per binary; uncompressed — with LZ4 over HTTP the driver loses the server's error text on a failed insert, which the dead-letter logic reads.

These limits are sent by the client. In production also give each
binary its own ClickHouse user and settings profile, so the server
enforces them whatever a client sends:

```sql
CREATE SETTINGS PROFILE flox_api SETTINGS max_execution_time = 30, max_memory_usage = 4294967296;
CREATE USER flox_api IDENTIFIED WITH sha256_password BY '…' SETTINGS PROFILE 'flox_api';
GRANT SELECT, INSERT, CREATE TABLE, ALTER ON flox.* TO flox_api;
-- the same for flox_tracker (10 s, 1 GiB) and flox_worker (no limits)
```

### Redis

`REDIS_URL` — dev: no auth (`infra/docker-compose.dev.yml`'s `redis`
service sets none). Production: a real password (`redis://:password@host:
6379/0`) — CLAUDE.md's own stance on Redis (cache/rate-limit/sticky-cache/
postback-dedup only, never sticky's source of truth) already means a
Redis outage degrades rather than corrupts (`apps/internal/conversion`,
`apps/tracker/sticky.go` both documented fallback paths), but that's not
a reason to leave it unauthenticated on a real network.
It also needs `maxmemory` and an evicting policy (`allkeys-lru`) —
`docs/deployment.md`, "Redis memory".

### NATS

`NATS_URL` (tracker, worker) — the JetStream event stream
(`docs/event-stream.md`). Dev/test compose: `nats://nats:4222`; from the
host `nats://localhost:4222` (test: `4223`). Unset: events go through the
Postgres `event_queue` as before. Production: the cluster's client URL,
with credentials (`nats://user:pass@host:4222`) if the server requires
them — the Kubernetes NATS does (`NATS_PASSWORD`, `docs/kubernetes.md`);
logs mask the password. `NATS_STREAM_REPLICAS` — stream replicas, `1` (default) for a single
server, `3` in a cluster. `NATS_STREAM_MAX_MB` — the stream's size on disk,
`10240` (default); it must fit the server's `max_file_store`, otherwise
JetStream refuses to create the stream and every event goes through the
Postgres fallback.

### S3-compatible object storage

Content Gallery files (`apps/internal/storage`; `apps/api` creates the
bucket on start). The dev/test values (MinIO's
`flox-minio-admin`/`flox-minio-secret`) are throwaway compose-internal
credentials; replace both in any real deployment.

| Variable | Default | What it does |
|---|---|---|
| `FFMPEG_PATH` / `FFPROBE_PATH` (worker) | `ffmpeg` / `ffprobe` (on `PATH`) | The tools the worker converts gallery videos (MOV, AVI, MKV…) to MP4 with; the worker image ships them. Not found, or no object storage: the worker logs a warning and such videos stay "Converting…". `docs/content-gallery.md` "Transcoding" |

### Backups

Read by the backup sidecar (`apps/cmd/backup`, `docs/backup.md`).

| Variable | Dev value | Staging/production |
|---|---|---|
| `BACKUP_S3_ENDPOINT` / `_REGION` / `_BUCKET` / `_ACCESS_KEY_ID` / `_SECRET_ACCESS_KEY` | dev MinIO, bucket `flox-backups` | An **off-site** bucket — another provider or region than the databases and `S3_BUCKET`. Refused if equal to `S3_BUCKET` on the same host. Credentials limited to that bucket. |
| `BACKUP_CLICKHOUSE_S3_ENDPOINT` | `BACKUP_S3_ENDPOINT` | Set only when the ClickHouse server reaches the bucket by another URL than the sidecar |
| `BACKUP_POSTGRES` | `walg` | `off`: no Postgres backup here (a managed Postgres's own PITR); ClickHouse and files only |
| `BACKUP_DATABASE_URL` | `DATABASE_URL` | A role allowed `pg_backup_start`/`pg_backup_stop`/`pg_switch_wal` (superuser, or granted them) |
| `BACKUP_AT` | `03:00` | Daily run, UTC |
| `BACKUP_KEEP_POSTGRES` / `BACKUP_KEEP_CLICKHOUSE` | `14` / `14` | Base backups / ClickHouse backups kept. The Postgres PITR window is the oldest kept base backup to now. |
| `BACKUP_DRILL_DAY` | `1` | Day of the month (1–28) the restore drill follows the backup; `0` turns the scheduled drill off |
| Postgres server: `WALG_S3_PREFIX`, `AWS_ENDPOINT`, `AWS_REGION`, `AWS_S3_FORCE_PATH_STYLE`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` | see `infra/docker-compose.dev.yml` | Same bucket, prefix `postgres`. Read by `archive_command = wal-g wal-push %p` |

### Auth / sessions

No secret to configure — session and invite tokens are random
(`crypto/rand`), never signed or encrypted, and stored hashed (SHA-256).
There is no JWT signing key anywhere in this system to rotate or protect.
See `docs/auth.md`.

### SMTP / email

`SMTP_URL`/`MAIL_FROM` — consumed by `apps/internal/mailer` (a real SMTP
client, stdlib `net/smtp`), used by `apps/internal/auth`'s signup
email-verification flow (`docs/auth.md`'s own "Email verification"
section) — the one place this codebase sends a real outbound email. Team
invites still generate a copyable link instead (a deliberate product
choice, not an infrastructure gap — see that doc). Dev runs
`infra/docker-compose.dev.yml`'s `mailhog` container as a catch-all SMTP
sink (`smtp://localhost:1025`; inbox web UI at `http://localhost:8025`,
or its REST API at `http://localhost:8025/api/v2/messages`). Point
`SMTP_URL` at a real provider (SES, Postmark, SMTP relay, …) — with
credentials in the URL, `smtp://user:pass@host:port`, if it needs auth —
only for staging/production; `MAIL_FROM` should also become a real,
deliverability-sane From address at that point (SPF/DKIM configured for
its domain), not `floxlink.io`'s placeholder value.

### SMS (two-factor codes)

`SMS_WEBHOOK_URL` / `SMS_WEBHOOK_SECRET` — optional. When set,
`apps/internal/smswebhook` posts SMS two-factor codes there as
`{"to", "text"}`, signed with `X-Flox-Timestamp` and
`X-Flox-Signature` = hex(HMAC-SHA256(secret, timestamp + "." + body)); the
receiver forwards them to an SMS provider. The secret must be at least 16
characters; setting only one of the two fails startup. Unset: SMS is not
offered as a second factor (email and the app still are). See
`docs/auth.md` "Second factor: email and SMS".

### Tracker click limit

`TRACKER_IP_CLICK_LIMIT` is read by `apps/tracker`: clicks per client IP (an IPv6 /64) per minute, counted per
replica. The default is 600; 0 turns the limit off. Over it, a visitor is sent on (to the campaign's fallback, or to
the destination of a landing or postlanding link) and nothing is recorded. The client IP is resolved through
`TRUSTED_PROXIES`, so without it every visitor behind the proxy shares one IP. See `docs/security.md`.

### YooKassa (plan payments)

`YOOKASSA_SHOP_ID` / `YOOKASSA_SECRET_KEY` are optional and read by `apps/api` and `apps/worker`. When both are
set, an owner can pay for a plan in Settings › Plan. When either is missing, plans are not sold online (the page
says to write to the team).

`YOOKASSA_API_URL` defaults to `https://api.yookassa.ru/v3`; tests point it at a fake server.

Receipts (54-FZ):

- `YOOKASSA_RECEIPT=on` sends a receipt with each payment, for "receipts by YooKassa" or a cash register connected
  to the shop. The customer is the paying owner's email.
- `YOOKASSA_VAT_CODE` sets the item's `vat_code` (default 1, no VAT).
- `YOOKASSA_TAX_SYSTEM_CODE` sets `tax_system_code` (0 = not sent).

In the YooKassa dashboard, set the HTTP notification URL to `<API_PUBLIC_URL>/v1/payment-notifications/yookassa`
for the `payment.succeeded` and `payment.canceled` events. The notifications are not signed, so FLOX reads each
payment back through the API before applying it. Without notifications (dev with no public URL), apps/worker reads
pending payments back every 10 minutes, and the payer's return page reads its own payment at once.
See `docs/billing.md` "Payments".

### OpenTelemetry

`OTEL_EXPORTER_OTLP_ENDPOINT` — empty/unset is a valid, intentional value
(`apps/internal/telemetry.Setup`'s own doc: a no-op when unset, not an
error). Dev runs with no OTel collector; point this at a real OTLP
endpoint (Jaeger, Tempo, a vendor) for production tracing. Prometheus
metrics (`docs/observability.md`) are a completely separate signal, always
on regardless of this variable — `GET /metrics` on all three Go binaries
needs no configuration at all.

### Retention

| Variable | Dev value | Staging/production |
|---|---|---|
| `RETENTION_FAILED_POSTBACKS_DAYS` (worker) | `90` | Days a postback that was not recorded (duplicate/error/ignored) is kept in Postgres; `0` keeps them. Successes are never deleted. `docs/retention.md` |
| `RETENTION_PIXEL_DELIVERIES_DAYS` (worker) | `30` | Days a sent or dead-lettered pixel delivery (it holds the visitor's IP and user agent) is kept in Postgres; `0` keeps them. Rows still to send are never deleted. `docs/pixels.md` |
| `RETENTION_PUSH_DELIVERIES_DAYS` (worker) | `30` | Days a finished push delivery is kept; its broadcast keeps the counts. `0` keeps them. `docs/push.md` |
| `TELEGRAM_WEBHOOK_BASE_URL` (api) | empty | The tracker's public **https** origin Telegram calls (`…/tg/{botId}`). Unset: Settings → Integrations shows Telegram bots as unavailable and connecting one is refused (503). `docs/telegram.md` |
| `TELEGRAM_API_URL` (api) | `https://api.telegram.org` | The Bot API the api calls when a bot is connected or removed; tests point it at a fake. |
| `RETENTION_POSTBACK_LOG_MONTHS` (worker) | `13` | TTL of ClickHouse's postback attempt log, applied by the worker at start; `0` removes it. Lowering it deletes older log rows at the next merges. |

### FX rates

| Variable | Dev value | Staging/production |
|---|---|---|
| `FX_PROVIDER` | `ecb,cbr` (default) | A comma list of `ecb` (the ECB) and `cbr` (the Bank of Russia: RUB, UAH, KZT and other currencies the ECB lacks). `none` only when `fx_rates` is filled by other means — then non-USD amounts get a USD value only from those rows. Anything else fails `apps/worker` at boot. |

Neither needs a key; `apps/worker` needs outbound HTTPS to
`www.ecb.europa.eu` and `www.cbr.ru`. See `docs/fx.md`.

### Error tracking

| Variable | Dev value | Staging/production |
|---|---|---|
| `SENTRY_DSN` | empty (off) | The DSN of a tracker that speaks the Sentry protocol — Sentry, or self-hosted GlitchTip and similar. Read by api, tracker and worker. |
| `SENTRY_ENVIRONMENT` | empty (= `NODE_ENV`) | Label for the events, e.g. `production`. |

With a DSN, every ERROR log record (with its attributes) and every panic in
an HTTP handler or a worker loop is sent, asynchronously; visitor IPs and
headers are not attached. A malformed DSN fails the binary at boot.
`internal/errtrack`.

### Pixels

| Variable | Default | Purpose |
|---|---|---|
| `PIXEL_ENCRYPTION_KEY` (api, worker) | dev key in compose / `.env.example` | **Required** — both refuse to start without it. 32 random bytes, base64. Encrypts pixels' ad platform access tokens (AES-256-GCM): the api writes them, the worker reads them back to send events (`docs/pixels.md`). Its own key, separate from the integration and ad account keys. Changing it makes every saved token unreadable — each pixel's token has to be pasted again. |
| `SHOP_WEBHOOK_ENCRYPTION_KEY` (api, tracker) | dev key in compose / `.env.example` | Optional. 32 random bytes, base64. Encrypts the keys Shopify and WooCommerce stores sign their order webhooks with (and Tilda sites' API keys) (AES-256-GCM): the api writes them, the tracker reads them back to check a webhook (`docs/shop-connections.md`). Unset: the source page says stores cannot be connected, and a webhook is answered 503 and logged. A malformed value stops the api at startup; the tracker logs it and refuses webhooks. Changing it makes every saved key unreadable — each store's key has to be pasted again. |
| `PUSH_VAPID_PUBLIC_KEY` (api, tracker, worker), `PUSH_VAPID_PRIVATE_KEY`, `PUSH_VAPID_SUBJECT` (worker) | dev pair in compose / `.env.example` | Web Push VAPID key pair (base64url, as webpush-go's `GenerateVAPIDKeys` writes them) and a `mailto:`/`https:` contact. Unset: PWAs never ask for notifications, the api refuses broadcasts (503), the worker leaves them queued. One pair per deployment; changing it ends every existing subscription. `docs/push.md` |

### Alerts

| Variable | Default | Purpose |
|---|---|---|
| `INTEGRATION_ENCRYPTION_KEY` (worker) | empty | The api's key for integration credentials. The worker needs the same value to read the Slack webhook alerts are sent to (`docs/alerts.md`). Without it the worker logs a warning at start, alerts still appear in the app, and every Slack delivery is recorded as failed. |
| `STICKY_COOKIE_KEY` (tracker) | dev key in compose / `.env.example` | **Required** — the tracker refuses to start without it. 32+ random bytes, base64 (`head -c 32 /dev/urandom \| base64`). Signs sticky cookies (`docs/routing.md`, tech audit T5). Same value on every tracker replica and never rotated casually: a new key ignores every existing cookie and re-buckets returning visitors. |

### Domain/DNS providers, Facebook/TikTok ad accounts

All blank in `.env.example` on purpose — every one of these is a real
external vendor credential CLAUDE.md non-negotiable #11 requires stay
behind an interface (no vendor lock-in in core logic), and none has ever
been wired to a live account in this project (`internal/adaccount/
facebookads`/`tiktokads` are structurally real but only ever benchmarked
against fixtures — `docs/performance.md`). Fill in real values only when
actually connecting a real vendor account; the code path degrades
gracefully with them empty — confirmed directly against a real
`apps/worker` log line with no ad accounts connected: `"scheduled ad
spend sync run finished" connections_attempted=0`, not an error.

## Secrets handling

Nothing in this list is a signing key or encryption key (see "Auth /
sessions" above) — every secret here is a plain credential (a database
password, a vendor API key). Standard practice applies: never commit
`.env`, inject real values via your deployment platform's secret store
(Kubernetes Secret, a cloud provider's parameter store, Docker Swarm
secret, etc.) rather than baking them into an image or compose file.
`docker-compose.test.yml`'s hardcoded values are acceptable *only* because
they're throwaway, disposable, local-only credentials — the same
`docker-compose.dev.yml` has always used.
