# apps/tracker

Go hot-path click/redirect service — separate binary, same Go module as
`apps/api`, shares `internal/routing` and `internal/classifier`. Target:
p50 < 20ms / p95 < 50ms redirect latency (excl. third-party calls). Scaffolded
starting Phase 21 (Tracking Engine). Placeholder directory until then.
