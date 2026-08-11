# apps/worker

Go async event/postback processor — separate binary, same Go module as
`apps/api`, shares `internal/routing` and `internal/classifier`. Consumes the
event queue written by `apps/tracker` and persists to ClickHouse; handles
outgoing postback retries. Scaffolded starting Phase 24 (Postback Engine).
Placeholder directory until then.
