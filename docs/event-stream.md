# Event stream (NATS JetStream)

Since 2026-10-10 the tracker's events reach ClickHouse through NATS
JetStream instead of the Postgres `event_queue`, when `NATS_URL` is set.
Package: `apps/internal/eventstream`.

```
tracker ─ eventbuf ─► NATS stream FLOX_EVENTS ─► worker ─► ClickHouse
              │  (refused)                          │ (10 failed inserts)
              └────────────► Postgres event_queue ◄─┘
                               ▲
   postback (conversion) ──────┘  one transaction with the postback row
```

## Why

The end-to-end load test (`docs/performance.md`, "End-to-end capacity")
showed the Postgres queue as the ceiling: at 8k events/s Postgres spent a
full core on `event_queue` alone, at 12k/s the tracker's buffer overflowed
and a third of the clicks were lost. Through the stream the same machine
carries 20k/s with no loss and Postgres idle (table below).

## What goes where

| Events | Path |
| --- | --- |
| Everything the tracker buffers (`SOURCE_CLICK`, `LAND_*`, `PWA_*`, push, TG …) | NATS stream |
| The same, when NATS certainly refused them | Postgres `event_queue` (fallback) |
| Conversions (`CPA_*` from postbacks) | Postgres `event_queue`, always |

Conversions stay in Postgres on purpose: a CPA event commits in one
transaction with its `postbacks` row and its outgoing delivery (audit D1,
CLAUDE.md #3 — dedup key and status progression). Their volume is orders
of magnitude below clicks. The worker reads both the stream and
`event_queue`.

## Exactly once in ClickHouse

The stream delivers at least once; ClickHouse must not count a click
twice. Two ids make every retry harmless:

- **One message = one tracker batch**, at most 512 KB (a bigger batch is
  split greedily). Its `Nats-Msg-Id` is `<first event id>-<event count>`.
  A retry of the rest of a batch starts on a message boundary and splits
  into the same messages with the same ids, so JetStream's duplicate window
  (10 min) drops a republished message.
- **The worker inserts each message on its own** under
  `insert_deduplication_token = nats:<message id>:<table>` (audit D2,
  `chstore.WithBatchID`). A redelivery after a lost ack re-sends the same
  block and ClickHouse skips it. The worker never merges messages into one
  insert — a redelivered message could then land in a different block
  under a different token.

## Failure handling

**Tracker publish** (`Sink`):

- *Certainly not stored* — no connection, never connected yet, no stream,
  stream answered with an error (full, too large): the events go to
  `event_queue` at once (`flox_eventstream_fallback_total`). Publishes
  never wait in a reconnect buffer.
- *Unknown* — the publish timed out (2 s): the message may be in the
  stream, so it is retried on NATS only, with the same id. Writing it to
  Postgres too would count its clicks twice. After 30 s of failures it
  goes to Postgres anyway: by then losing the events (the 10k buffer fills)
  is worse than a possible duplicate batch.

**Worker consume** (`Consumer`, 4 workers, 8 messages per fetch):

- Insert OK → ack. Insert failed → redelivered after 1 s, 2 s, 4 s … 1 min.
- 10 failed deliveries → the events move to `event_queue`, whose flusher
  splits the batch and dead-letters the event ClickHouse keeps rejecting
  (audit Q2). Known gap: if an earlier attempt had already written part of
  the message (one table of several), the handoff writes that part again
  under another token. It takes ClickHouse rejecting the same batch ten
  times after accepting part of it.
- Not an event batch → terminated, counted
  (`flox_eventstream_undecodable_total`, critical alert).
- Shutdown: a message already pulled is still written and acked; anything
  unacked is redelivered after the ack wait (1 min).

## Stream settings

`EnsureTopology` creates or updates them; both binaries call it at start
and retry in the background while NATS is down.

| Setting | Value | Why |
| --- | --- | --- |
| Name / subject | `FLOX_EVENTS` / `flox.events` | |
| Retention | work queue | a message stays until the worker acks it, even with no consumer yet |
| Discard | new | a full stream refuses publishes (→ Postgres) instead of discarding unread events |
| Max bytes | `NATS_STREAM_MAX_MB` (10240) | about 10M events of backlog; must fit the server's `max_file_store`, or JetStream refuses the stream (`insufficient storage resources`) and every event goes to Postgres |
| Max age | 7 days | |
| Replicas | `NATS_STREAM_REPLICAS` (1) | 3 in a production cluster |
| Duplicate window | 10 min | |
| Consumer | durable `clickhouse-writer`, explicit ack, ack wait 1 min, unlimited deliveries, 1000 in flight | |

A second reader of the same events (bot detection, live dashboards) needs
its own stream sourcing `FLOX_EVENTS` — a work-queue stream allows one
consumer per subject.

## Running it

- Dev/test compose: `nats` service (`nats:2.10-alpine -js`), tracker and
  worker get `NATS_URL=nats://nats:4222`. From the host:
  `nats://localhost:4222` (test compose: `4223`).
- Without `NATS_URL` everything works as before through `event_queue`.
- Tests: `NATS_URL=nats://localhost:4222 go test ./internal/eventstream/`
  (skipped without it; each test uses its own stream, apart from the
  running worker's).
- Production needs: a JetStream cluster of 3 with persistent volumes,
  `NATS_STREAM_REPLICAS=3`, and the NATS server's own monitoring
  (`:8222`, or prometheus-nats-exporter). `docs/deployment.md`.

## Metrics and alerts

Tracker: `flox_eventstream_published_total`, `_fallback_total`,
`_publish_errors_total{kind=refused|unknown}`. Worker:
`flox_eventstream_consumed_total`, `_retried_total`, `_handed_off_total`,
`_undecodable_total`, `_pending_messages`. Alerts in `infra/alerts.yml`
(group `flox.events`): `EventStreamFallingBackToPostgres`,
`EventStreamPublishUnanswered`, `EventStreamBacklogGrowing`,
`EventStreamHandedOffToPostgres`, `EventStreamUndecodableMessages`.
`EventLossDetected` still covers the tracker's buffer.

## Measured (2026-10-10)

Same machine and method as `docs/performance.md`, one tracker, one worker,
60 s per step, every click counted in ClickHouse after the backlog drained:

| Rate | Per minute | p99 | Arrived in ClickHouse | CPU: tracker / Postgres / ClickHouse / NATS |
|---:|---:|---:|---|---|
| 8,000/s | 480k | 0.41 ms | 480,000 / 480,000 | 72% / 0.2% / 42% / 3% |
| 12,000/s | 720k | 0.53 ms | 720,003 / 720,003 | 98% / 0.5% / 72% / 5% |
| 16,000/s | 960k | 0.60 ms | 960,000 / 960,000 | 148% / 0% / 152% / 7% |
| 20,000/s | 1.2M | 1.26 ms | 1,200,000 / 1,200,000 | 169% / 0.4% / 110% / 8% |
| 25,000/s | 1.5M | 4.4 ms | 1,496,077 / 1,499,397 — 3,320 dropped | 221% / 0% / 168% / 11% |

At 25k/s the tracker itself is the limit again: its single flush loop
publishes batch after batch. Beyond ~20k/s per instance, add tracker
replicas — each has its own buffer and writer.
