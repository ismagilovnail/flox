# CLAUDE.md — FLOX

> This file is read at the start of every session. Keep it lean.
> The full spec is `FLOX-master-prompt-v3.md` — read it before implementing any
> phase. This file is the operating manual + progress tracker, not the spec.

---

## PROJECT

**FLOX** — production-grade SaaS traffic tracking & routing platform.
Tagline: **Track. Route. Optimize.**

Full requirements: **`FLOX-master-prompt-v3.md`** (authoritative; sections
referenced below as §N).

---

## CURRENT STATE — UPDATE THIS EVERY PHASE

```
CURRENT PHASE : GEO/ASN PROVIDER — MAXMIND GEOLITE2 (user-scoped, post-Phase-34)
STATUS        : done — follow-up to the Click Log phase below, which
                surfaced that classifier.NoopGeoProvider/NoopASNProvider
                were still the only providers wired up (country/city/ASN
                always empty). Added apps/internal/classifier/maxmind: a
                real GeoProvider/ASNProvider implementation against local
                MaxMind GeoLite2 .mmdb files (github.com/oschwald/
                geoip2-golang, new dependency) — the interfaces
                themselves needed no changes (CLAUDE.md #11). New config
                GEOIP_CITY_DB_PATH/GEOIP_ASN_DB_PATH (.env.example,
                apps/internal/config), wired into apps/tracker/main.go
                replacing classifier.New(nil, nil, nil) — best-effort at
                startup exactly like the existing ClickHouse/Redis
                handling (CLAUDE.md #9): missing/bad path logs a warning
                and falls back to Noop, never blocks the redirect path.
                A per-IP lookup miss is likewise not an error — same
                empty result Noop returns. infra/docker-compose.dev.yml's
                tracker service bind-mounts infra/geoip/ (gitignored) at
                /geoip and points both env vars at it.
                MaxMind's databases need a free account + license key
                (EULA prohibits redistributing them) — no .mmdb ships in
                the repo, docs/geoip.md has the signup/download steps;
                that's the one manual step left, on the user.
                apps/internal/classifier/maxmind/testdata/ carries
                MaxMind's own public Apache-2.0 test fixtures instead,
                used for this session's verification.
                Verified: go build/vet/test -count=1 ./... clean,
                including fixture-backed unit tests (known IP ->
                country/region/city and IP -> ASN, plus the "not in
                database" no-error case). Real end-to-end pass against
                the actual dev tracker: bind-mounted the test fixtures,
                recreated the tracker container, hit a real tracking
                link via curl with a spoofed X-Forwarded-For through the
                real redirect path, confirmed the resulting click_events
                row in ClickHouse carried the real looked-up country/
                region/city/asn — not just an isolated unit test. Also
                verified graceful degradation: removed the fixtures,
                restarted, confirmed a clean warning log and normal
                startup, lookups falling back to empty. Fixtures removed
                from infra/geoip/ afterward — no real GeoLite2 data
                obtained or tested (no MaxMind account available).
LAST COMMIT   : (pending — this phase not yet committed)
UNCOMMITTED   : this phase's own work (see STATUS above) — apps/internal/
                classifier/maxmind/* (incl. testdata/*.mmdb),
                apps/internal/config/config.go, apps/tracker/main.go,
                apps/go.mod/go.sum, .env.example,
                infra/docker-compose.dev.yml, infra/geoip/README.md,
                .gitignore, docs/geoip.md, docs/click-log.md (one caveat
                line updated), this CHANGELOG entry. Same pre-existing
                unrelated uncommitted work noted in the superseded block
                below is still untouched in the tree.
NEXT          : confirm scope before starting. Real candidates: (a) the
                pre-existing list from the superseded block below, still
                open; (b) once the user has real GeoLite2 data in
                infra/geoip/, nothing further to build — it's a drop-in.

```

<details>
<summary>Superseded — Click Log (kept for history, see above for current state)</summary>

```
CURRENT PHASE : CLICK LOG (user-scoped, post-Phase-34)
STATUS        : done — user asked whether FLOX captures per-click IP/geo;
                shown that click_events always has (IP/UserAgent/geo/
                device on apps/internal/event.Event), they asked to see
                it in the UI. No such screen existed — analytics/the
                report builder only ever expose aggregates, never
                row-level click_events reads. Added a read-only Click Log
                page: one row per SOURCE_CLICK/SOURCE_FILTER visit,
                org-wide, newest first, paginated — mirrors
                apps/internal/postbacklogs' read side (same 90-day range
                cap, same 50/200 default/max page size). New backend:
                chstore.EventStore.ListClickLog/CountClickLog
                (chstore/click_log.go), new apps/internal/clicklogs
                package (model/service/handler), mounted at GET
                /click-log. No writes/replay — a click row isn't
                retriable like a postback attempt. New frontend:
                features/clicklogs/* (click-log-view.tsx, click-log-
                columns.tsx), /click-log page under "Traffic" in the nav
                (after Campaigns). Every click_events column is
                reachable via the table's Columns menu; only a practical
                default subset (time/type/campaign/click id/country/
                city/device/OS/browser/IP) is visible on first load —
                DataTable gained a generic initialColumnVisibility prop
                for this (apps/web/src/components/ui/data-table.tsx).
                Campaign ids resolve to names via the existing
                useCampaigns() list; stream-set/flow ids stay raw (no
                org-wide stream-set listing exists, and resolving those
                would mean an N+1 fetch per campaign).
                Deliberately unchanged, called out on the page and in
                docs/click-log.md: geo/ASN are still NoopGeoProvider/
                NoopASNProvider (render "—" for real traffic — a real
                provider is separately-scoped work); click_events still
                has no retention policy, IP/UA stored raw and
                indefinitely (a real compliance decision for the user to
                make explicitly, not invented here).
                Verified: go build/vet/test -count=1 ./... clean,
                including a real-ClickHouse integration test
                (chstore/click_log_test.go — list/count, pagination,
                newest-first, IP/geo/device/subs round-trip, cross-org
                isolation per CLAUDE.md #5). tsc --noEmit/eslint clean.
                Real browser pass against the actual reporting user's
                own account: real rows render (real internal IPs,
                resolved campaign names), Columns menu correctly toggles
                every hidden field, no console errors.
LAST COMMIT   : feat(analytics): read-only Click Log page (IP/geo/device
                per visit) — 36003de
UNCOMMITTED   : none of this phase's own work. NOTE: the working tree
                also carries a large amount of pre-existing, unrelated
                uncommitted work this phase deliberately left untouched
                and did not commit — a Sheet-to-full-page conversion
                across several features (domains/landings/networks/
                offers/pixels/postlanding/pwa/traffic-sources: *-form-
                sheet.tsx deleted, new *-form.tsx/*-detail-view.tsx/
                new-*-view.tsx added) plus stream-set filter-picker
                changes and a new relative-time.tsx component. Not this
                session's work to explain or commit — flag to the user
                before touching any of those files.
NEXT          : confirm scope before starting. Real candidates: (a) a
                real Geo/ASN provider (e.g. MaxMind GeoLite2) behind the
                existing classifier.GeoProvider/ASNProvider interfaces —
                no go.mod dependency, config plumbing, or docker-compose
                .mmdb volume exists yet for this; (b) the pre-existing
                list from the superseded block below, still open.

```

</details>

<details>
<summary>Superseded — Campaign edit page layout (kept for history, see above for current state)</summary>

```
CURRENT PHASE : CAMPAIGN EDIT PAGE LAYOUT (user-scoped, post-Phase-34)
STATUS        : done — user shared a reference tracker's campaign edit
                screenshot (compact settings + tracking-link block left,
                wide Stream Sets panel right) and asked for the same
                block layout. campaign-detail-view.tsx's Overview tab
                restructured into a lg:grid-cols-[360px_1fr] two-column
                layout: CampaignForm (moved out of Settings) + TrackingLinks
                on the left, StreamSetList on the right; KPI stats/chart
                stay full-width below. Settings tab now holds only the
                danger zone. Added a real name-search box to StreamSetList
                (drag-to-reorder disabled while filtered, to avoid
                reordering a partial view by index).
                Follow-up in the same session: user clarified they
                specifically wanted Keitaro-style inline row editing, not
                a Sheet. StreamSetFormSheet deleted outright; its exact
                form (same fields/validation/sub-components, nothing about
                the editing logic changed) now lives in stream-set-
                editor.tsx, rendered inline under a row when expanded
                (StreamSetRow owns its own expand/collapse state, chevron
                + name click toggle it). "New Stream Set" now creates
                immediately via createStreamSet — no dialog — defaulting
                the new set's flow to the org's first offer/network
                (the backend rejects an empty offerId; the old Sheet flow
                never hit this since client-side Zod validation always
                blocked that submission first). Button disabled when the
                org has no offers yet, alongside the pre-existing
                no-networks case. Also wired the pre-existing but unused
                useDeleteStreamSet hook into the row menu with a confirm
                dialog, matching the reference's trash icon.
                Not done, real gap if ever wanted: per-row Уники/
                Депозиты/Доход columns — no ClickHouse column ties an
                event to a stream_set_id, only campaign_id/flow_id, so
                those numbers would have to be fabricated.
                Fixed a stale streamSets.json string claiming the
                fallback URL lived "in Settings" (en+ru).
LAST COMMIT   : feat(web): campaign edit page as a two-column layout,
                inline stream-set editing — a92c9b5
UNCOMMITTED   : none.
NEXT          : confirm scope before starting — same real candidates as
                before (see superseded block below), plus, if ever
                wanted: a stream_set_id column on click/tracking/
                conversion events so per-set stats could be real.

```

</details>

<details>
<summary>Superseded — Domains + Tracking Links (kept for history, see above for current state)</summary>

```
CURRENT PHASE : DOMAINS + TRACKING LINKS (user-scoped, post-Phase-34)
STATUS        : done — apps/internal/domain + apps/internal/trackinglink
                (new packages) are the real CRUD the domains/tracking_links
                tables (migration 00007) never had — apps/tracker's hot
                path already read them for real, but nothing let an
                operator populate them, and apps/web's Domains page was
                pure mock data. User reported creating a campaign and
                finding no way to tell it was active and no link to send
                traffic to. Two root causes, both fixed:
                (1) campaign-row-actions.tsx's row menu had no Activate
                action for a draft campaign (backend already supported
                it — frontend-only gap, now fixed).
                (2) No tracking-link UI anywhere. New POST /domains +
                GET/PATCH/DELETE /domains/{id}; new POST /campaigns/
                {campaignId}/tracking-links + PATCH/DELETE .../{id},
                returning a computed url (http://{domain}/t/{slug})
                matching apps/tracker's own /t/{trackingID} route. No
                registrar/DNS/SSL automation (none exists in this repo) —
                status/ssl/registrar/dnsProvider are operator-set fields,
                same honest shape as campaign.Status; the mock's "Verify
                ownership"/"Issue SSL" quick actions were dropped, not
                carried forward as fake automation.
                apps/web: features/domains/* switched to real API calls,
                lib/mock/domains.ts + stores/domains.ts deleted outright.
                New features/campaigns/tracking-links.tsx at the top of a
                campaign's Overview tab (domain picker, slug input, live
                URL preview, copy/pause/delete on existing links) — fully
                localized (new campaigns.json trackingLinks keys, en+ru),
                unlike the Domains page itself which stays English
                (pre-existing, separately-scoped gap, not touched here).
                Full design + the two tenant-isolation checks that
                actually matter in trackinglink.Repository.Create
                (domain_id from the request body, campaign_id from the
                route — neither re-validated against org elsewhere the
                way a plain List/Get already is) written up in
                docs/domains-tracking-links.md.
                Verified: go build/vet/test -count=1 ./... clean,
                including both new packages' own repository tests against
                real Postgres (cross-org domain conflict, cross-org
                domain/campaign rejection on tracking-link create,
                duplicate-slug conflict). Real browser pass against the
                actual reporting user's own account and real campaign
                (not a synthetic test org): added a real domain, created a
                tracking link from the Overview tab, and confirmed with a
                direct curl against the real containerized tracker that it
                302-redirects to the campaign's configured offer — the
                real click path, not a mocked-looking success screen.
                tsc --noEmit/eslint clean.
LAST COMMIT   : feat(domains): real Domains + Tracking Links CRUD, fix
                draft-campaign activation — aa25993
UNCOMMITTED   : none.
NEXT          : confirm scope before starting. Real candidates, not
                ranked: (a) the six remaining frontend-integration gaps —
                Domains' own UI still isn't localized (separate from
                today's fix, which only added the backend + the campaign-
                side tracking-links UI), Settings, Tags, Custom Metrics,
                Report Presets, Referral, Content Gallery; (b) a "distinct
                values for dimension X" endpoint for the analytics report
                builder's filter-value dropdown (docs/analytics-report-
                api.md); (c) campaign_id/flow_id -> display name
                resolution in Top Campaigns/Top Flows; (d) the DataTable
                column-resize/export gap; (e) repository/handler-layer
                unit test coverage; (f) real registrar/DNS/SSL automation
                for Domains, if ever wanted — today's fix deliberately
                left status/ssl as operator-declared, not automated.

```

</details>

<details>
<summary>Superseded — Phase 35 (kept for history, see above for current state)</summary>

```
CURRENT PHASE : PHASE 35 — REAL MULTI-DIMENSION /ANALYTICS BACKEND
STATUS        : done — apps/internal/analytics/report.go +
                apps/internal/chstore/report.go (Report/Timeseries/
                Breakdown/Funnel/Dashboard) replace the mock data the
                report builder (features/analytics/*) and /overview
                dashboard ran on since their Phase 14/14.5 frontend-first
                build. 11 of the report builder's 16 named dimensions are
                backed (5 — landing/pwa/postlanding/offer/network — have
                no column anywhere in the event pipeline yet, confirmed
                as later work, not faked). Cost joins only at campaign
                granularity or coarser (cost_entries is campaign+day
                grained in Postgres); every other dimension combination
                correctly returns null cost/profit/roi/roas/cpc/cpa,
                never a fabricated 0 (CLAUDE.md #6). New HTTP surface:
                POST /analytics/{report,timeseries,breakdown,funnel,
                dashboard}, JSON body. Frontend: hooks/use-analytics-
                report.ts + lib/api/analytics.ts additions replace
                aggregate.ts's client-side aggregateReport/
                aggregateTimeSeries/aggregateByDimension (deleted) and
                lib/mock/{analytics,dashboard}.ts's generators (deleted,
                lib/mock/dashboard.ts removed outright) — ReportTable's
                existing ReportRow/custom-metric-formula code needed no
                changes since API responses map onto that exact shape.
                No Top Offers card on /overview (offer isn't a backed
                dimension — dropped, not faked). Full design, dimension
                whitelist, and known-gaps writeup in
                docs/analytics-report-api.md; CHANGELOG's own Phase 35
                entry has the fix/verification detail.
                This phase also committed three other pieces of already-
                finished work found sitting uncommitted in the tree (see
                CHANGELOG for each): containerizing the dev stack (api/
                tracker/worker/web now run as containers with air hot-
                reload, dropping the last host-network-shaped thing in
                the repo), gating login behind email verification (real
                SMTP mailer, new /auth/verify-email + /auth/resend-
                verification), and closing the recurring ECharts/zrender
                animation crash for good (animation: false as the master
                kill switch) plus russifying /overview and the shared
                date-range picker, plus a small unrelated i18n pass on
                the analytics report builder screens.
                Found and fixed two real bugs: (1) apps/api/main.go
                didn't compile — analytics.NewService's second
                (CostSource) parameter was added in an interrupted
                earlier session but the call site was never updated to
                pass costSvc. (2) The already-written dashboard mapping
                this phase inherited had KpiGrid showing a fabricated $0
                for Spend/Profit instead of "—" whenever an org has zero
                cost_entries rows — PeriodMetrics.spend/.profit are now
                nullable end to end, rendering "—" like the ROI/CPA tiles
                already did.
                Verified: gofmt/go vet/go build/go test -count=1 ./...
                clean repo-wide, including chstore's report tests run
                against a REAL ClickHouse container (not mocked) —
                dimension grouping/filtering/by-day/cross-tenant
                isolation all pass. Full containerized dev stack brought
                up; signed up a real test org; curl'd all five new
                endpoints (empty-state, then non-empty after inserting
                real ClickHouse rows) — correct aggregation and null-cost
                cascade. Real browser pass (Claude-in-Chrome): /overview
                and all four /analytics report-builder tabs render real
                data with no console errors for both an empty org and the
                seeded one; a published custom metric correctly evaluates
                to "—" against a null-cost row. tsc --noEmit/eslint clean
                on apps/web.
LAST COMMIT   : feat(analytics): real multi-dimension /analytics backend
                (Phase 35) — cfe3390, preceded by 6aa98df feat(auth): gate
                login behind email verification and b3306ff feat(infra):
                containerize the dev stack, drop host networking
UNCOMMITTED   : none — this phase's commit(s) include everything that was
                previously sitting uncommitted (containerization, email
                verification, the zrender fix, dashboard/analytics i18n)
                alongside Phase 35's own work.
NEXT          : confirm scope before starting. Real candidates, not
                ranked: (a) the six remaining frontend-integration gaps —
                Domains, Settings, Tags, Custom Metrics, Report Presets,
                Referral, Content Gallery (Analytics/Overview's own gap
                is now closed by this phase); (b) a "distinct values for
                dimension X" endpoint for the report builder's filter-
                value dropdown (currently still a fixed mock pool, see
                docs/analytics-report-api.md); (c) campaign_id/flow_id ->
                display name resolution in Top Campaigns/Top Flows; (d)
                the DataTable column-resize/export gap; (e) repository/
                handler-layer unit test coverage.

```

</details>

<details>
<summary>Superseded — Phase 34 audit (kept for history, see above for current state)</summary>

```
CURRENT PHASE : PHASE 34 — FINAL AUDIT
STATUS        : done — §62's full checklist audited (UX/UI/accessibility/
                security/tenant isolation/performance/database/API/
                routing/attribution/analytics/LTV/testing/observability/
                documentation), plus the repo-wide TODO/FIXME/mock/dummy/
                temporary/hardcoded/console.log/debugger sweep. Zero
                TODO/FIXME/console.log/debugger found in source.
                Headline finding (not fixed this phase — real product-
                surface work, not an audit-phase action): seven whole
                frontend feature domains were built frontend-first
                (Phases 14/14.5-14.9) against apps/web/src/lib/mock/*.ts
                and never taken through a backend-integration phase —
                Domains, Settings (organization/API-keys/integrations
                panels), Tags, Custom Metrics, Report Presets, Referral,
                Content Gallery. Confirmed by absence: no matching
                apps/internal/* Go package, no apps/web/src/lib/api/*.ts
                client, no route in apps/api/main.go for any of the
                seven; their zustand stores import straight from lib/
                mock/. This is wider than the "missing tracking-URL
                generation API" gap Phase 32 flagged (that was only the
                Domains slice of it) — CURRENT STATE previously
                undersold the true scope. Concretely, CLAUDE.md
                non-negotiable #12 (custom-metric formula safety) is
                unenforceable today: no formula-evaluation engine exists
                anywhere, frontend or backend. Same root cause also
                explains the standalone Analytics report builder
                (`features/analytics/*`, dimension/funnel report UI) and
                the Overview dashboard (`(app)/overview`) still reading
                `lib/mock/analytics.ts`/`lib/mock/dashboard.ts`: the real
                `/analytics` backend only ever shipped two fixed
                campaign-scoped endpoints (daily clicks, daily revenue —
                genuinely wired via `hooks/use-campaign-analytics.ts`),
                never the flexible multi-dimension/funnel query capability
                that page's UI needs. Legitimately deferred, not a wiring
                bug — noted here so it doesn't get mis-filed as one later.
                Three real bugs were found and fixed (not just documented):
                (1) `campaign-cost-entries.tsx`'s delete action had zero
                confirmation step, violating CLAUDE.md's destructive-
                action rule — every sibling row-actions file (network,
                traffic source, postlanding) correctly gates delete behind
                a Dialog, this one didn't; now it does, with matching
                cost.json i18n keys (en+ru). (2) A stale doc comment in
                `apps/internal/routing/weighted.go` claimed the frontend
                had "a temporary simulator mock" mirroring the Go pick
                logic — no such file has existed for a while; the routing
                simulator has genuinely been a thin client over
                `/routing/simulate` all along (invariant #1 upheld in
                practice, the comment was just wrong). (3)
                `use-current-member.ts`'s comment claimed "no auth session
                yet (that's Phase 28)" — false, real `/auth/me` has
                existed since Phase 28; the actual reason this hook still
                reads the mock team roster (Custom Metrics/Referral/
                Content Gallery have no backend of their own yet, and
                switching identity source ahead of each feature's own
                integration phase would break their mock "is this mine"
                checks) is now stated accurately instead.
                Also removed as dead code (zero importers, confirmed by
                grep before deletion): `stores/{campaigns,offers,
                event-mappings}.ts` and `lib/mock/{offers,event-mappings,
                conversions}.ts` — all six pre-date the real API clients
                that replaced them and were never cleaned up.
                `lib/mock/campaigns.ts` was NOT deleted — `tag-
                assignments.ts` (the live, still-deferred Tags mock)
                still imports it for realistic fixture data.
                Everything else audited clean: routing single-source-of-
                truth (routing simulator only calls /routing/simulate, no
                TS reimplementation), deterministic weighted pick +
                sticky-cookie-is-truth (#4, §38), tenant isolation (all 19
                route groups in apps/api/main.go sit behind
                tenantMiddleware; repository-layer spot checks across 13+
                domains all filter on organization_id; the only two
                queries missing an explicit org_id filter are in
                auth/repository.go's AcceptInvite, which is correctly
                scoped upstream by a verified invite token instead),
                CSRF/secrets/postback-dedup (#3 — event_ref enforced in
                code, not just convention), no hardcoded secrets, .env
                correctly gitignored, tracker hot path (#9 — Handler.track
                never blocks on persistence or analytics; eventbuf.
                Writer.Enqueue is a non-blocking channel send), DB indexes
                on tenant-scoped hot tables, ClickHouse sort keys lead
                with organization_id, LTV/cohort currency handling (#7 —
                FX rate looked up at the event's OccurredAt, never
                time.Now()), OpenTelemetry tracing genuinely wired (real
                OTLP exporter + otelhttp instrumentation on all 3
                binaries, not just imported), no sensitive data (passwords/
                tokens/secrets) in any log call site, infra/alerts.yml's 8
                rules cross-checked against actual metric names (all
                resolve), go build/vet/test -count=1 ./... and tsc/eslint
                clean.
                Fixed: apps/web/src/app/globals.css gained the one global
                `@media (prefers-reduced-motion: reduce)` rule (§66) —
                previously absent entirely, so no component respected
                the OS setting. docs/deployment.md gained a Connection
                pool sizing section documenting that none of the four Go
                binaries set explicit Postgres/Redis pool limits
                (pgxpool.New/redis.Options both ride library defaults) —
                not a bug (Phase 31's real load test measured p95
                1.0-1.5ms against the 50ms budget with defaults as-is),
                but a previously-undocumented production tuning knob;
                documented the DATABASE_URL?pool_max_conns= path.
                Known gaps documented, not fixed this phase (real scoped
                work, not audit-phase actions): (a) the seven-domain
                frontend-integration gap above; (b) the standalone
                Analytics report builder + Overview dashboard's missing
                multi-dimension backend, above; (c) the shared DataTable
                component (`components/ui/data-table.tsx`) never got
                column resizing or CSV/export wired despite both being in
                CLAUDE.md's own UX-floor table rules (§64) — sorting,
                pagination, column visibility, search/filter are all
                real and used consistently, resizing/export are not, and
                this is repo-wide (every table), not per-table; (d)
                repository/handler-layer Go packages (network, offer,
                pixel, campaign, streamset, cost, and ~18 others — see
                `go test -cover` output) sit at 0.0% unit-test coverage —
                DB-backed logic is exercised only via the Phase 32 e2e
                scenario and manual dev-stack verification. Core
                computation packages are well covered (routing 81.7%,
                ltv 67.0%, attribution/event/macro/metrics 100%).
                Verified: gofmt -l/go vet/go build/go test -count=1
                ./... clean repo-wide; tsc --noEmit/eslint clean on
                apps/web.
LAST COMMIT   : fix(audit): close Phase 34 findings — cost-entry confirm dialog, dead code, stale comments
UNCOMMITTED   : between-phases infra work (user-scoped, see CHANGELOG's
                "[Between phases] — containerize the dev stack, drop host
                networking"): infra/docker-compose.dev.yml now runs
                api/tracker/worker/web themselves as containers (each
                Go service's Dockerfile gained a `dev` target — air
                hot-reload over a bind-mounted apps/ tree — web reuses
                its existing `deps` stage with `npm run dev`), not just
                the data stores. Removed the last host-network-shaped
                thing in the repo: prometheus's `extra_hosts:
                host-gateway` + `host.docker.internal` scrape targets,
                now `api:8080`/`tracker:8081`/`worker:8082` by compose
                service name. Root Makefile rewritten around `docker
                compose` instead of host `go run`/`npm run dev` + PID
                files. Verified end-to-end: full stack up, all 9
                containers healthy, all 4 app containers'
                `NetworkMode` is the compose bridge network (not
                `host`), hot reload confirmed live.
                Second, later batch of uncommitted work, same session
                (see CHANGELOG's "[Between phases] — gate login behind
                email verification"): registration used to auto-login
                immediately with no proof of email ownership — user-
                reported as an incomplete registration flow. Signup no
                longer creates a session; it emails a real verification
                link instead (new apps/internal/mailer, a real net/smtp
                client — the SMTP_URL/mailhog infra from the first batch
                above was staged exactly for this), and Login now 403s
                unverified accounts with a distinct email_not_verified
                code. New migration 00022 (users.email_verified_at +
                verification token columns, existing rows grandfathered
                verified). New POST /auth/verify-email (auto-logs in,
                same AcceptInvite-style token consumption) and POST
                /auth/resend-verification (silent/anti-enumeration).
                AcceptInvite also marks accounts verified (clicking an
                invite link is equally strong proof of inbox access) —
                without this, invited teammates would be permanently
                locked out by Login's new check. apps/web: new "check
                your email" post-signup panel, new /verify-email page,
                login's new resend-on-unverified action. 8 new Go tests;
                existing 21-test suite needed zero changes (signupOrg
                helper's signature/return shape unchanged, now routes
                through a spy Mailer + the real verify step internally).
                Found and fixed two real bugs along the way: a stuck-
                loading React bug in the new verify-email page (useQuery
                fixed what useMutation-fired-from-an-effect couldn't —
                full writeup in docs/auth.md) and leftover host-run
                api/tracker/worker/web processes (from before the first
                batch's containerization) blocking the new containers'
                ports. Full curl + real-browser verification, all
                documented in docs/auth.md. Not yet committed — waiting
                on the user re: how to handle the other unrelated
                uncommitted work already in the tree (analytics-report
                i18n) before commit.
NEXT          : confirm scope before starting. Real candidates, not
                ranked: (a) the seven-domain frontend-integration gap
                above (Domains, Settings, Tags, Custom Metrics, Report
                Presets, Referral, Content Gallery) — likely several
                phases' worth, probably slice-by-slice like the earlier
                domain-integration phases (Traffic Sources, Networks/
                Offers, Stream Sets) rather than one giant phase; (b) a
                real multi-dimension /analytics backend for the report
                builder + dashboard, replacing their mock data; (c) the
                DataTable column-resize/export gap; (d) repository/
                handler-layer unit test coverage. Per §9's fixed phase
                order, Phase 34 was the last listed phase — anything past
                it is v3-scope/hardening work the user should scope
                explicitly, not an assumed default.

```

</details>

> At the end of every phase: update the four lines above, add a CHANGELOG entry,
> commit, then STOP and wait. Do not roll into the next phase automatically.

---

## THE ONE RULE

**Work strictly one phase at a time.** Never build ahead. Never generate large
code before validating the current phase. Every phase follows:

```
Inspect → Plan → Implement → Run → Test → Fix → Review → Document → STOP
```

Only advance when the current phase meets its Definition of Done (§79).
Phase order is fixed — see §9. Do not reorder or skip.

---

## PHASE PROTOCOL (per §2)

Open each phase with:

```
PHASE X — NAME — GOAL
What will be built
Files created/changed
Dependencies
Acceptance criteria
```

Close each phase with:

```
COMPLETED / TESTS / KNOWN ISSUES / FILES CHANGED / NEXT PHASE
```

Run typecheck, lint, unit tests, and build where applicable before reporting
complete. Never skip validation. Do not ask permission to continue unless a real
architectural blocker exists — but never make silent destructive changes.

---

## STACK (do not swap without asking)

**Frontend:** Next.js · React · TypeScript · Tailwind · shadcn/ui · Radix ·
TanStack Query/Table · React Hook Form · Zod · Zustand · Lucide · date-fns ·
Apache ECharts.

**Backend:** Go · chi · pgx · sqlc (where sensible) · goose migrations ·
slog/zerolog · OpenTelemetry.

**Data:** PostgreSQL (control plane) · ClickHouse (high-volume events + analytics)
· Redis (cache, rate limits, sticky CACHE only, postback dedup) · S3-compatible
object storage.

**Architecture:** modular monolith. `apps/tracker` and `apps/worker` are separate
binaries inside the same Go module, sharing `internal/routing` and
`internal/classifier`. No microservices up front. No unnecessary frameworks.

---

## REPO LAYOUT

```
apps/
  web/       Next.js frontend (the only non-Go app)
  api/       Go control-plane API
  tracker/   Go hot-path click/redirect service
  worker/    Go async event/postback processor
packages/
  ui/ config/ types/
docs/        architecture, routing, event-model, ltv, metrics, etc. (§76)
infra/       docker-compose, deployment
FLOX-master-prompt-v3.md   ← full spec
CLAUDE.md                  ← this file
```

Frontend internal structure: `app/ components/ features/ hooks/ lib/ stores/
types/ schemas/`. Domain code lives in `features/`. All server calls go through
`src/lib/api` — never scatter `fetch()` in components (§32, §70).

Go structure per module: `handler → service → repository → domain`. Handlers are
thin; business logic never lives in handlers or in React components (§34, §71).

---

## NON-NEGOTIABLE INVARIANTS

Read the referenced section before touching related code.

1. **Single source of truth for routing (§6-SHARED, §26).** Routing/filter/sticky
   /metric decision logic exists ONCE (in Go). The routing simulator is a thin UI
   over the `/routing/simulate` contract — NOT a second TS implementation. Both
   sides pass one shared conformance fixture.

2. **Full event model (§43).** ~20 event types incl. CPA statuses
   HOLD/ACCEPT/REDEP/DECLINE/TRASH as an enum. Design the ClickHouse schema for
   all of them from day one — adding types later is a live-data migration. Never
   collapse conversions into one "conversion" type.

3. **Postback dedup key = (click_id, status, event_ref)** (§45) — NOT click_id
   alone, and NOT (click_id, status). `event_ref` = network txn id for CPA_REDEP
   (the only repeatable status), empty string for every other status even if a
   txn id was sent. **Status never goes back to CPA_HOLD** — nightly partner
   replays re-send it after approval and would take revenue out of a closed
   report (§45 STATUS PROGRESSION). Long Redis TTL + durable DB unique
   constraint. Store original currency + USD value at event time.
   `acceptDuplicates` override per network — it does not bypass the
   progression rule.

4. **Sticky = cookie is truth, Redis is cache only (§39-STICKY).** Cookie
   `sf_{campaignId}` = `setId:flowId[:clickId]`. Redis-only sticky is forbidden
   (silently corrupts A/B tests).

5. **Tenant isolation (§36-TENANCY).** Every tenant-scoped table has
   `organization_id`; every query filters on it; enforcement is in the repository
   layer. `organization_id` comes from the session/API key, never the request
   body. ClickHouse sort keys lead with it. Cross-tenant isolation test is part of
   DoD for API/data phases.

6. **Cost or it doesn't exist (§27-COST).** Profit/ROI/ROAS need spend. Manual
   cost entry first; FB/TikTok import later. No cost for a slice → show ROI as
   "—", never compute against zero.

7. **Currency at event time (§50-FX).** Normalize to USD using the rate on the
   event date, never the current rate. Store both original and USD.

8. **Regex = RE2 only (§5, §22).** Go stdlib `regexp`. Validate user patterns at
   save time, never on the hot path. No PCRE libs.

9. **Redirect hot path stays fast (§41, §56).** tracking p50 < 20ms / p95 < 50ms
   (excl. third-party latency). Persist events async (buffered batch → queue).
   Never run analytics queries or block on outbound partner calls in the redirect.

10. **WebView bounce ≠ moderator cloaking (§73).** Bouncing in-app WebView
    (FB/IG/TikTok/Telegram) to the external browser so the PWA install prompt can
    fire is a REQUIRED, provider-neutral capability. Vendor-specific moderator
    detection is FORBIDDEN. Keep them separate.

11. **Providers behind interfaces (§74–75).** Geo, ASN, bot, cost, conversion,
    pixel, registrar, DNS, FX — all swappable. No vendor lock-in in core logic.

12. **Custom metrics build on the registry, safely (§30.5, §50).** User formulas
    reference registry metrics by stable id; division-by-zero yields empty, never
    an error; one formula = one data source (no mixing push with regular); LTV not
    allowed in formulas; team-private.

---

## EVENT MODEL QUICK REF (authoritative list in §43)

```
SOURCE_CLICK, SOURCE_FILTER
LAND_VIEW, LAND_CLICK, POSTLANDING_VIEW, POSTLANDING_CLICK
PWA_VIEW, PWA_OPEN, PWA_INSTALL, IOS_INSTALL
NOTIFICATION_REQUEST/SUBSCRIBE/DECLINE/UNSUBSCRIBE/CLICK
TG_JOIN, TG_START
CPA_HOLD, CPA_ACCEPT, CPA_REDEP, CPA_DECLINE, CPA_TRASH
```

Canonical funnel: `SOURCE_CLICK → LAND_VIEW → LAND_CLICK → PWA_VIEW →
PWA_INSTALL → CPA_HOLD → CPA_ACCEPT → CPA_REDEP`. Chain linked by one click_id.

---

## ROUTING QUICK REF (full detail §21, §39)

```
Campaign → Stream Set (priority, first match wins, AND/OR filters, pixels)
        → Flow (weighted, pickWeighted) → destination
No set matches → campaign fallback / safe destination.
```

`pickWeighted` is **deterministic by visit key** (§38): an unseeded FNV-1a hash
of a stable property of the visit, never an RNG. Same visit → same flow on every
replica and after every restart. Independent of sticky — the cookie is still the
truth for a returning visitor (#4); the hash covers what happens before a cookie
exists. Candidates are filtered *before* the draw, so weights mean what the
operator typed.

Decisions must be explainable: why matched / why not / why this flow / why
fallback / sticky applied from where (§72).

---

## BUILD ORDER (frontend-first, §9 / §83)

Design & UI on mock contracts first (phases 2–15), then Go backend & engines
(16+), then integration (27), then LTV (26.5), auth/RBAC (28), hardening.
"Frontend-first" means UI/UX first on mock CONTRACTS — it does NOT license a
parallel routing engine in TS.

New in v3 (secondary, after core): Tags (14.5), Custom Metrics (14.6), Report
Presets + directory stats (14.7), Referral (14.8), Content Gallery (14.9).

Core workflows to prioritize (§84): Create Campaign → Stream Set → Filters →
Flow → Simulate Routing → Analytics → Track Conversion (HOLD→ACCEPT→REDEP) →
Postback → LTV.

---

## UX FLOOR (every screen, §63–68)

loading / empty / error / success states on everything. Destructive actions
confirm. Forms validate with inline errors + disabled submit + success feedback.
Tables: sort, paginate, column visibility/resize, search, filter, export;
virtualize large sets. Dark-first, dense, tabular numerals, meaningful status
colors — a trading-terminal control plane, not a generic admin dashboard.
WCAG 2.2 AA. Animation sparing (150–250ms ease-out).

---

## DEFINITION OF DONE (§79)

```
✓ implemented        ✓ no TS errors      ✓ no Go build errors
✓ lint passes        ✓ tests pass        ✓ build passes
✓ UX reviewed        ✓ responsive        ✓ loading/empty/error states
✓ tenant isolation verified (API/data phases)
✓ docs updated       ✓ CHANGELOG entry   ✓ CLAUDE.md CURRENT STATE updated
✓ logical commit (§78)
```

---

## NEVER (§80)

Build all phases at once · replace repo without inspection · duplicate business
logic (esp. a 2nd routing impl in TS) · hardcode secrets · commit `.env` · skip
tests/validation · ignore TS/Go errors · fake APIs that look real · hide errors ·
dedup on click_id alone or on (click_id, status) alone · record a postback that
moves a conversion back to CPA_HOLD · treat missing cost as zero · Redis-only sticky ·
truncate the event model · leak data across orgs · embed vendor moderator
detection · divide-by-zero into an error in custom metrics · mix push+regular
metrics in one formula · reimplement tags per entity · treat empty FB subs as
"unknown campaign".

Mocks are allowed only in frontend-first phases and must implement the same
contract as the real backend, and be explicitly replaceable.

---

## COMMITS (§78)

Logical, per-phase commits. Examples:
`feat(ui): design system` · `feat(routing): filter builder` ·
`feat(routing): sticky engine` · `feat(ltv): cohort engine` ·
`feat(api): campaign endpoints` · `feat(tracker): tracking endpoint`.
No single giant commit.

---

## SESSION START CHECKLIST

1. Read CURRENT STATE above.
2. Read the relevant §sections in `FLOX-master-prompt-v3.md` for this phase.
3. Confirm the phase plan (§2) before writing code.
4. Implement → validate → document → update CURRENT STATE → commit → STOP.
