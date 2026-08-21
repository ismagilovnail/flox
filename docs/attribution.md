# Attribution

Which click did this conversion come from?

This is the join that decides who gets paid, so the governing rule (§44) is
**do not invent attribution when there is insufficient evidence**. It is a rule
about money, not tidiness: an invented link credits a traffic source that did
not earn the conversion, and the buyer then scales the wrong campaign with real
budget. An honest "unattributed" is a support ticket. A confident wrong answer
is a bad decision made repeatedly, and nothing in any report flags it.

## Identifiers (§42, §44)

| Identifier | Who minted it | Strength |
|---|---|---|
| `click_id` | FLOX, at redirect time, handed to the network as a `{click_id}` macro | Strong — we issued it |
| `external_click_id` | the ad network (`fbclid`, `ttclid`, or a partner's own) | Weak — caller-supplied on the click side, and **not reliably unique** |

§42's note applies directly here: a share of Facebook/Instagram clicks arrive
with empty subs and no network id at all, depending on how the link was opened.
Missing stays empty; it never becomes a placeholder, and never "unknown
campaign".

## Decision order

Evidence is tried strongest-first, and the search stops at the first identifier
that yields a definite answer.

```
click_id present?
  ├── yes → look it up (scoped to the organization)
  │         ├── found     → ATTRIBUTED (method = click_id)
  │         └── not found → UNKNOWN_CLICK          ← does NOT fall through
  └── no  → external_click_id present?
            ├── no  → NO_IDENTIFIER
            └── yes → look it up (scoped to the organization)
                      ├── exactly one match → ATTRIBUTED (method = external_click_id)
                      ├── no match          → UNKNOWN_CLICK
                      └── several matches   → AMBIGUOUS
```

Two branches are deliberate and worth stating plainly, because both look like
missed opportunities to attribute one more conversion:

**A present-but-unmatched `click_id` does not fall back to the external id.**
The network echoed back an identifier we minted and we cannot find it. That
claim is suspect, and quietly re-matching it on a weaker, caller-supplied field
would paper over precisely the case worth investigating.

**Several clicks sharing one `external_click_id` is ambiguity, not a
tiebreak.** Network click ids repeat in practice — a redirect chain, a prefetch
and a genuine second visit all produce the same `fbclid`. Preferring "the most
recent" would look sensible and would credit the wrong click a fraction of the
time, invisibly. That is the exact behaviour §44 forbids.

## Outcomes

A closed, small set, so it can be a low-cardinality column and a dashboard
filter — "today's unattributed postbacks, grouped by reason" is the question an
operator actually asks.

| Outcome | Meaning | Typical cause |
|---|---|---|
| `attributed` | joined to exactly one click | — |
| `no_identifier` | neither identifier was present | postback template misconfigured in the network's dashboard |
| `unknown_click` | identifiers present, matched nothing | expired/purged click, forged postback, wrong organization |
| `ambiguous_external_click_id` | one network id, several clicks | redirect chain / prefetch / repeat visit |

An unattributed conversion is a **first-class answer, not an error**: the
postback was received, understood and recorded, and simply has no click to hang
on. Phase 23 stores it either way — an unattributed conversion that is
invisible is indistinguishable from one that never arrived, and those need very
different responses.

Every outcome carries a human-readable `Reason`, in the same spirit as routing
explainability (§72): the postback log is where a disputed payout gets
re-argued.

## Tenant isolation (CLAUDE.md #5, §36-TENANCY)

`Conversion.OrganizationID` comes from the credential the postback
authenticated with — the API key, or the org-scoped postback URL — and **never
from the request body**. Without it, `AttributeConversion` returns
`ErrNoOrganization` rather than searching globally; the only alternative to
refusing is a cross-tenant leak that would behave like a working feature.

Enforcement lives in the **repository layer**: every `ClickResolver` method
takes `organizationID` and must filter on it. A filter that is part of the
query cannot be forgotten at one call site.

A click belonging to a *different* organization is reported as
`unknown_click`, identical to one that does not exist. Distinguishing them
would confirm the existence of another tenant's click id to anyone who can
guess one.

## Time to conversion

`Attribution.TimeToConversion` is the conversion timestamp minus the click's.
It is the number that separates a real traffic source from a cheating one:
conversions landing seconds after the click, at scale, are not people.

A **negative** value means the conversion predates its click — clock skew
against the network, or a replayed postback. It is reported, not clamped:
silently flooring it at zero would erase the one signal that says so. Skew is a
diagnostic, never grounds for refusing a click that matched.

## Where clicks are read from

`internal/attribution` is pure — no HTTP, no database driver, no clock of its
own — and reads clicks through the `ClickResolver` interface, exactly as
`internal/routing` reads configuration through `routingstore`.

```go
type ClickResolver interface {
    ByClickID(ctx context.Context, organizationID, clickID string) (Click, error)
    ByExternalClickID(ctx context.Context, organizationID, externalClickID string) ([]Click, error)
}
```

`ByExternalClickID` returns a **slice** on purpose. An interface that could
only return one click would have to pick a winner somewhere below this line,
out of sight of the "do not invent" rule.

**Today:** `MemoryResolver` — per-process, unbounded, gone on restart, and
labelled as such. The tracker currently hands events to `eventbuf.LogSink`,
which is explicitly not durable storage, so there is nothing to query yet.
Rather than pretend otherwise — by quietly adding a clicks table to Postgres
that §7 says does not belong there — the stand-in is honest and explicitly
replaceable.

**Landed in Phase 26:** `chstore.ClickResolver`, querying `click_events`
(§48's real analytical schema). `apps/tracker/main.go` wires it in place of
`MemoryResolver` — nothing in `internal/attribution` itself changed, exactly
as this package always promised. Two decisions specific to querying an
event-sourced table rather than a live index:

- **Eventual consistency**: a click reaches `click_events` only after
  `apps/tracker` enqueues it and `apps/worker` flushes it
  (`internal/eventqueue`, up to ~2s behind in the worst case). A conversion
  arriving within that window for a brand-new click can see
  `unknown_click` even though the click genuinely happened. Real postback
  delay dwarfs this window in practice; nothing compensates for it (no
  retry-after-delay), and none was added — see `chstore`'s own doc comment.
- **`stickyFlowKeepClickId` reuse** (§39-STICKY) means one `click_id` can
  legitimately appear on more than one `click_events` row across a
  returning visitor's journey. `ByClickID` resolves to the **earliest**
  occurrence — the original click that started the journey — not the most
  recent.

A resolver **failure** (a query against a live ClickHouse erroring) surfaces
as an error, never as `unattributed` — recording a blip as "no click found"
would permanently write off a real conversion, so the caller has to be able
to retry. This is distinct from `apps/tracker`'s own startup choice of
*which* resolver to wire up at all: ClickHouse being unreachable when the
tracker boots falls back to an empty `MemoryResolver` (logged clearly, not
silently) rather than refusing to start — the redirect path must never
depend on ClickHouse being up (CLAUDE.md #9's spirit), even though the
practical effect is every conversion reading `unknown_click` until the next
successful restart. Recorded here as a deliberate, known degradation, not
an oversight.

## Attribution window: decided in Phase 23 — there isn't one

There is no attribution **window** — no "conversions more than N days after
the click are refused." §44 doesn't specify one, and §45's own explicit
"never lose the conversion" stance (its Redis-unavailable fallback would
rather accept a wrong report than drop revenue) points the same direction: a
window that silently discards a late conversion is the kind of policy this
package refuses to invent on its own. `Attribution.TimeToConversion` is
already the observable an operator needs to build alerts on outliers without
FLOX unilaterally writing off revenue on their behalf. If a real window is
ever wanted, it belongs as an explicit, per-network, operator-configured
setting — not a default baked into `internal/attribution` or
`internal/conversion`.
