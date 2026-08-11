# Architecture

See [`/ARCHITECTURE.md`](../ARCHITECTURE.md) at the repo root for the
canonical, stable architecture summary (layers, repo layout, data stores,
shared-domain-logic decision, invariants).

This document accumulates deeper technical detail as each backend phase
lands (Phase 16+): module boundaries inside `internal/`, request lifecycle
through `apps/tracker`, the event pipeline `apps/tracker → queue →
apps/worker → ClickHouse`, and how `apps/api`/`apps/tracker`/`apps/worker`
share `internal/routing` and `internal/classifier` without duplicating
decision logic. Empty until Phase 16.
