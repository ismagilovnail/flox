# Public API reference (audit API-1)

The API an integration calls with a key from *Settings › API keys*, described
as OpenAPI 3.1.

| Where | What |
|---|---|
| `apps/internal/openapi/openapi.yaml` | The spec, written by hand, embedded in the API binary. |
| `GET /v1/openapi.json`, `GET /v1/openapi.yaml` | The spec, no session or key needed. `servers` is `API_PUBLIC_URL` + `/v1`, or the relative `/v1` when that is unset. |
| `/api-reference` (apps/web) | The reference page, public (`proxy.ts` PUBLIC_PATHS), rendered from the served JSON: operations by tag, search, parameters, request and response fields (expandable, enums inline), a curl example per operation. Linked from the API keys panel. |

## What is in it

Campaigns, stream sets and flows, the routing simulator, tracking links,
domains (read), spend (cost entries, CSV import, daily spend), offers,
networks (incl. secret rotation and diagnostics), traffic sources,
conversions and a click's timeline, the click log, the postback log, and
the reports (`/analytics/report`, `/timeseries`, `/breakdown`) — 61
operations.

Left out on purpose, each with its reason, in `notInPublicSpec`
(`apps/api/openapi_test.go`): sign-in and account security, team,
organization settings, API keys themselves, connected accounts, domain
management (DNS verification happens in the app), content (landings, PWA,
postlandings, gallery, pixels), push, tags, event mappings, the app's own
screens (alerts, notifications, custom metrics, presets, referral,
onboarding) and the UI-shaped report endpoints (dashboard, funnel,
flow-summary, dimension values, LTV). Content, push, tags, event mappings,
funnel and LTV are candidates for the next version.

Each operation carries `x-flox-scope`: `read` (every GET, the simulator and
the reports) or `write`. A key's scope tiers are those of
`auth.apiKeyScopeAllows`; a `write` key cannot reach team or security
settings, neither of which is in the spec.

The incoming postback is the tracker's, not the API's: the spec points to
each network's `incomingPostbackUrl` (docs/postback-delivery.md).

## How it stays true

The spec is hand-written; three tests hold it to the code.

1. **`internal/openapi` `TestSpecIsWellFormed`** (no database): every
   operation has a unique `operationId`, a scope, one tag, a summary, one
   2xx and a default response; every path parameter is declared; every
   schema uses only the keywords `openapi.Validator` implements and every
   `$ref` resolves. An unsupported keyword is an error, so the spec never
   states a rule nothing checks. (It caught unquoted commas in YAML flow
   maps, which silently split a description into stray keys.)
2. **`apps/api` `TestOpenAPIMatchesTheRouter`**: walks the real chi router.
   Every documented operation is a route; every route is documented or in
   `notInPublicSpec`; no entry there is stale or overlaps the spec. A new
   route fails until someone decides.
3. **`apps/api` `TestOpenAPIOperationsAnswerAsDocumented`** (Postgres +
   ClickHouse): creates a `write` and a `read` key and calls **every**
   operation of the spec through the real router — creating a source, a
   network, an offer, a campaign, stream sets with a nested filter tree, a
   tracking link and spend, inserting clicks, a conversion and postback
   attempts into ClickHouse for the reports and logs. Each request body is
   validated against its request schema, each answer against the schema of
   the status it returned, which must be documented. Response schemas are
   strict: `additionalProperties: false` and every field required unless Go
   omits it when empty — so adding, renaming or dropping a field of a Go
   response type fails the test until the spec says so. Then every write
   operation is called with the read key and must answer 403 in the error
   envelope; 401, 404 and 422 are checked too. An operation the test never
   calls fails it.

A mutation check (removing `notes`, `uniqueClicks`, renaming
`filterReason` in the spec) failed 15 calls across campaigns, reports and
the click log, as it should.

### Changing the API

- New field in a response type → add it to the schema (required unless
  `omitempty`).
- New route → add it to `openapi.yaml` with a call in the conformance test,
  or to `notInPublicSpec` with a reason.
- Field order on the page is required first, then by name: the served JSON
  comes from a Go map, so the YAML's order is not kept.
