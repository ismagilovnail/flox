# FLOX

**Track. Route. Optimize.**

FLOX is a production-grade SaaS platform for traffic tracking, campaign
routing, and conversion/LTV analytics — built for affiliate and performance
marketing teams (including iGaming CPA flows).

Full requirements: [`docs/FLOX-master-prompt-v3.md`](docs/FLOX-master-prompt-v3.md)
(authoritative spec). Operating manual + phase tracker: [`CLAUDE.md`](CLAUDE.md).

---

## Status

Early scaffolding stage — Phase 1 (Product Foundation). No application code
exists yet. See [`CLAUDE.md`](CLAUDE.md) for the current phase and
[`ROADMAP.md`](ROADMAP.md) for the full build order.

## Repository layout

```
apps/
  web/       Next.js frontend (control plane UI)
  api/       Go control-plane API
  tracker/   Go hot-path click/redirect service
  worker/    Go async event/postback processor
packages/
  ui/        shared UI components
  config/    shared config (eslint/tsconfig/tailwind presets)
  types/     shared TypeScript types
docs/        architecture, domain model, event model, routing, LTV, etc.
infra/       docker-compose, deployment
```

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the full architecture and the
shared-domain-logic strategy.

## Local development

The whole stack (web, api, tracker, worker, Postgres, ClickHouse, Redis,
MinIO, MailHog, Prometheus) runs from `infra/docker-compose.dev.yml`;
api/tracker/worker/web hot-reload. Tooling: Docker, plus Go (`apps/go.mod`)
and Node.js on the host for `make check` and the dev commands below.

```
make up                       # start everything
make dev-reset CONFIRM=yes    # DESTRUCTIVE: wipe Postgres, ClickHouse, Redis, MinIO
make fixtures                 # dev accounts + a working demo setup
make help                     # every target
```

`make fixtures` (`apps/cmd/devfixtures`) needs an empty database and goes
through the real services. It creates:

| Organization | Accounts (password `flox-demo-2026`, override with `FIXTURES_PASSWORD`) |
|---|---|
| FLOX Demo | `owner@`, `admin@`, `manager@`, `buyer@`, `analyst@`, `viewer@flox.local` — one per role |
| FLOX Other | `other-owner@flox.local` — a second tenant for isolation checks |

…and in FLOX Demo a tracking domain `track.localhost`, a traffic source, a
network with status mappings (`reg`/`ftd`/`redep`/`rejected`), an offer and
an active campaign behind `http://track.localhost:8081/t/demo`. It prints
the network's postback URL with its secret — shown only once.

Environment variables are documented in [`.env.example`](.env.example) and
[`docs/environment.md`](docs/environment.md).

## Documentation

See [`docs/`](docs/) for architecture, domain model, event model, routing,
LTV/cohorts, metrics registry, cost ingestion, domains, multitenancy, custom
metrics, tags, referral, API, and deployment docs (index in
[`docs/FLOX-master-prompt-v3.md`](docs/FLOX-master-prompt-v3.md) §76).
