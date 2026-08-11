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

Not yet available — the frontend (`apps/web`) and backend (`apps/api`,
`apps/tracker`, `apps/worker`) have not been scaffolded. This section will be
filled in as each phase lands:

- **Phase 2–15**: frontend on Next.js, run against mock API contracts.
- **Phase 16+**: Go backend (`apps/api`), PostgreSQL, ClickHouse, Redis.
- **Phase 33**: `docker-compose.dev.yml` for full local stack (web, api,
  tracker, worker, postgres, clickhouse, redis, object storage).

Required tooling once scaffolding lands: Node.js 24.x, Go 1.26.x, Docker.
Environment variables are documented in [`.env.example`](.env.example).

## Documentation

See [`docs/`](docs/) for architecture, domain model, event model, routing,
LTV/cohorts, metrics registry, cost ingestion, domains, multitenancy, custom
metrics, tags, referral, API, and deployment docs (index in
[`docs/FLOX-master-prompt-v3.md`](docs/FLOX-master-prompt-v3.md) §76).
