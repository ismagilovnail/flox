# Frontend i18n (§9-ish, first cross-cutting frontend infra phase)

`apps/web`'s internationalization foundation. Client-side, i18next-based,
zero routing changes — added ahead of the next large frontend domain per
direct instruction, migrating every currently-real (backend-wired)
screen: Campaigns, Traffic Sources, Networks, Offers, Stream Sets/
Filters/Flows, Routing Simulator, Cost, Conversions, Event Mappings, and
Postback Logs (+ the shared Postbacks page chrome and the app shell:
sidebar, topbar, breadcrumbs, command menu).

## Supported locales

- **`en`** (English) — the source/fallback locale. Always complete;
  every key exists here.
- **`ru`** (Russian) — the first additional locale. Must be complete for
  everything migrated in this phase; a genuinely missing Russian key
  falls back to the English string (i18next's `fallbackLng`), never a
  blank string or the raw key.

Adding a third locale means: add its `ISO`-code to `SUPPORTED_LOCALES` in
`apps/web/src/lib/i18n/config.ts`, add a full `locales/<code>/*.json` set
(one file per existing namespace — copy `locales/en/` as a starting
skeleton), import those files into `config.ts`'s `resources` object, and
add a display name to `LOCALE_NAMES` in
`apps/web/src/components/language-switcher.tsx`. Nothing else changes —
no routing, no build config.

## Why client-side i18next, not `next-intl`/routed locales

Inspection found no existing i18n library. `next-intl`'s idiomatic setup
puts the locale in the URL (`/[locale]/campaigns`) via middleware and
route restructuring — a real architectural change this phase was
explicitly asked to avoid ("do not redesign navigation/layout", "prefer
the smallest change"). `react-i18next` needs none of that: a client
provider (mirroring the existing `next-themes` `ThemeProvider` exactly)
plus a `useTranslation()` hook per component. Given the app is already
almost entirely `"use client"` feature components (TanStack Query +
Zustand throughout), this is the natural fit, not a workaround.

## Where translations live

```
apps/web/src/lib/i18n/
  config.ts              i18next.init, SUPPORTED_LOCALES, DEFAULT_LOCALE,
                          detectInitialLocale(), isSupportedLocale()
  config.test.ts          foundation tests (see "Testing" below)
  locales/en/*.json        one file per namespace, English (source)
  locales/ru/*.json        one file per namespace, Russian
```

One JSON file per **namespace**, one namespace per **domain/feature** —
not one giant dictionary (§3's explicit requirement). Current namespaces:
`common` (shared chrome: buttons, DataTable, loading/error defaults,
shared status words), `nav` (sidebar/breadcrumbs/command menu/topbar
search), `campaigns`, `cost` (the Campaigns detail page's Cost tab —
its own namespace despite living in `features/campaigns/`, since it was
named as its own migration priority), `trafficSources`, `networks`,
`offers`, `streamSets`, `routingSimulator`, `conversions`, `postbacks`
(covers Event Mappings, Postback Logs, and the Incoming/Outgoing tabs on
the same Postbacks page).

Resources are bundled statically (JSON imported directly into
`config.ts`, `resolveJsonModule` already enabled in `tsconfig.json`) —
not loaded over the network. The current string volume doesn't justify
async namespace loading's added complexity (loading states, suspense
boundaries); revisit if a much larger, later domain makes the bundle
size a real concern.

## Key convention

Dot-nested JSON, grouped by UI section within each namespace file, e.g.:

```json
{
  "list": { "title": "...", "emptyTitle": "...", "searchPlaceholder": "..." },
  "columns": { "name": "...", "status": "..." },
  "form": { "nameLabel": "...", "validation": { "nameMin": "..." } },
  "rowActions": { "edit": "...", "archiveConfirmTitle": "..." },
  "toast": { "created": "...", "updateError": "..." }
}
```

Call sites: `t("list.title", { ns: "campaigns" })`, or
`useTranslation(["campaigns", "common"])` once per component and then
bare `t("list.title")` for the first namespace in that array, `t("actions.cancel", { ns: "common" })`
for the rest.

**Shared vocabulary lives in `common`, not duplicated per namespace.**
Generic action words (`common.actions.save/cancel/delete/edit/add/retry/
duplicate/archive/pause/activate/copy/clear`) and generic entity status
words (`common.status.active/paused/archived/draft`) are defined once
and reused everywhere — check `common.json` before adding a same-meaning
key to a domain namespace.

## Adding a new user-facing string

1. Decide the namespace: which domain/feature owns this screen? (Shared
   chrome → `nav`/`common`.)
2. Check whether `common.actions.*`/`common.status.*` already covers it
   before adding a new key.
3. Add the key to `locales/en/<namespace>.json` with real English text.
4. Add the SAME key path to `locales/ru/<namespace>.json` with a real
   Russian translation — never leave it out "for later" or copy the
   English text as a placeholder.
5. Call `t("your.key.path", { ns: "<namespace>" })` (or via
   `useTranslation` array) at the call site. Never inline a literal
   English string in JSX for anything a user reads.
6. If the string embeds a variable, use `{{varName}}` interpolation —
   `t("form.titleEdit", { name: campaign.name })` — never string-concat.
7. If the string's phrasing changes with a count (a row count, a
   selection count, etc.), use i18next's plural-suffixed keys —
   `key_one`/`key_other` for English, `key_one`/`key_few`/`key_many`/
   `key_other` for Russian (CLDR: 1 → one, 2–4 → few, 5–20 → many, else
   the many/few pattern repeats) — and call `t("key", { count })`.
   i18next resolves the right suffix per the ACTIVE locale's real plural
   rules automatically (via `Intl.PluralRules`); see `common.json`'s
   `dataTable.selected_*`/`pagination_*` for a worked example, and
   `config.test.ts`'s pluralization tests for the expected output shape.

## Formatting: dates, numbers, currency, percentages

`apps/web/src/lib/format.ts` — `formatUsd(n, maximumFractionDigits, locale)`,
`formatInt(n, locale)`, `formatPercent(n, locale, maximumFractionDigits)`.
`locale` is the last argument and always optional, defaulting to `"en"`
— every call site that existed before this phase (dashboard, referral,
analytics — none of them in this phase's real-screen scope) keeps its
exact prior en-US-pinned behavior unchanged. Only call sites inside the
migrated real screens pass a live locale, via
`apps/web/src/hooks/use-locale.ts`'s `useLocale()` hook.

**A locale change never touches the underlying value.** These functions
only change digit grouping, decimal separator, and symbol placement
(`Intl.NumberFormat` under the hood) — the numeric value and the ISO
currency code passed in are identical regardless of `locale`. `ru`
renders `$14,655.87` as `14 655,87 $` (non-breaking-space thousands
separator, comma decimal, symbol after the value) — same 14655.87 USD,
different presentation.

Dates: `date-fns`'s `formatDistanceToNow`/`format` calls in migrated
list columns (e.g. "3 hours ago") were left as-is where the copy
convention doesn't obviously need localizing on its own merits — this
phase did not add `date-fns/locale`-based month/weekday name
localization, since none of the migrated screens render an actual
calendar date string (only relative "N time ago" captions, which read
fine in either language without a locale-specific format call). If a
future screen renders an absolute date (e.g. "March 5, 2026"), pass
`date-fns`'s own `locale` option (`import { ru } from "date-fns/locale"`)
keyed off `useLocale()`, the same way `formatUsd` takes one.

**Timezone semantics are untouched.** No formatting call in this phase
changed which timezone a timestamp is interpreted in — only how the
resulting local-time string is punctuated/worded.

## Statuses and domain/technical values — never translated at the source

CPA statuses (`CPA_HOLD`/`CPA_ACCEPT`/`CPA_REDEP`/`CPA_DECLINE`/
`CPA_TRASH`), postback results, campaign/network/source statuses, filter
operators/fields, routing decision reasons — every one of these stays
exactly the string the Go backend sends or expects, in API requests,
responses, and TanStack Query cache keys alike. What's translatable is
only the **display label** shown for a given value, via a
`X_I18N_KEY: Record<Value, string>` map co-located with that value's
type definition in `apps/web/src/lib/api/*.ts` (see
`SOURCE_TYPE_I18N_KEY`/`COST_INTEGRATION_I18N_KEY` in
`lib/api/traffic-sources.ts` for the pattern this phase established).
The Select/Badge component's `value`/underlying data field always stays
the raw untranslated string; only the rendered label text goes through
`t()`.

**Never translated, anywhere:** campaign/offer/network/traffic-source
names, click IDs, notes/free-text fields, raw network postback status
strings (e.g. `"ftd"`), URLs, macro tokens, regex patterns — any value a
user typed or the backend/a partner network generated.

## Backend

Untouched. Inspection found no backend requirement this phase couldn't
avoid — no database schema changes, no translated API responses, no
Accept-Language handling on `apps/api`. Every domain/enum value already
flowed through the API in English-coded form (`CPA_HOLD`, `"active"`,
`"IS_NOT"`, ...) before this phase and still does; only the browser's
own rendering of those values changed.

## Testing

`apps/web/src/lib/i18n/config.test.ts` (Vitest — newly added to the
project this phase; run via `npm run test` from `apps/web/`). Covers:
default locale, `detectInitialLocale`'s persisted-choice-wins/
unsupported-fallback/browser-language-detection behavior, `en ↔ ru`
switching, i18next's own fallback for a language with no resource
bundle at all, a genuinely-missing-key fallback to English (constructed
via `addResourceBundle`, not relying on an accidental real gap),
interpolation, English and Russian pluralization (including Russian's
`few`/`many` forms, not just `one`/`other`), and locale-aware
`formatUsd`/`formatInt`/`formatPercent` (including that the *value*
survives the locale change unchanged, only its presentation does).

## Accessibility

The language switcher (`apps/web/src/components/language-switcher.tsx`,
topbar, next to the theme toggle) is a standard Radix `Select` with a
real `aria-label` (`common.language.switcherLabel`) and full keyboard
navigation inherited from Radix — no custom key handling needed. Options
are language names ("English", "Русский"), never flags. `<html lang>`
is kept in sync with the active locale via `I18nProvider`
(`apps/web/src/components/i18n-provider.tsx`), updated in a `useEffect`
after mount — see that file's own comment for why (SSR/hydration:
matches the existing `next-themes`/`ThemeToggle` "start at a known
default, switch after mount" pattern already in this codebase, so
server and first-client-render markup always agree and React never logs
a hydration mismatch).

## What's deliberately deferred

- **Screens not yet backed by a real API**: Landings, PWA, Postlanding,
  Domains, Pixels, Reports, LTV/Cohorts, Push, Referral, Content
  Gallery, the standalone `/analytics` report builder, Team, Tags,
  Custom Metrics, Dashboard. All still fully mocked (see
  `docs/frontend-integration.md`); translating them now would mean
  translating content scheduled to be rebuilt, not preserved, once each
  gets its own real-backend phase.
- **A third locale.** The foundation supports adding one cheaply (see
  above), but none was requested this phase.
- **Server-side/SSR-perfect locale rendering.** The very first paint
  (server render and the first client render, before mount) is always
  English, matching `DEFAULT_LOCALE`; a returning Russian-locale user
  sees a brief flash of English before the post-mount effect switches
  the UI to their persisted choice. This is the same class of trade-off
  `next-themes` already makes for theme in this codebase (a "smallest
  change" client-only foundation, not a routed/cookie-based
  server-aware one) — a genuinely zero-flash SSR-locale-aware setup
  would need Next.js middleware/route restructuring, explicitly out of
  this phase's scope.
- **`date-fns` locale-aware absolute date formatting** — not needed yet
  since no migrated screen renders one (see "Formatting" above); the
  hook point (`useLocale()`) is already in place for whenever it is.
- **Translation management tooling** (a TMS, extraction linting,
  translator handoff workflow) — out of scope per the phase's own
  stated boundaries; this is a plain JSON-file foundation, not a SaaS
  integration.
