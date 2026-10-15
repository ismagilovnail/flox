# DataGrid — server-paged logs

§16 lists DataGrid next to DataTable; the UX floor (§63–68) asks tables to
"virtualize large sets". `components/ui/data-grid.tsx` is the table for
lists the **server** pages: the click log, postback logs and conversions.

## Why not DataTable

DataTable sorts, searches and pages on the client, over whatever `data` it
is given. The three logs handed it their API's first page (50 rows, the
default `limit`) and paged that client-side: past 50 rows nothing was
reachable, and search and CSV covered those 50 alone while looking like
they covered everything. Their APIs already took `from/to/limit/offset`
and returned `total`; the UI ignored them.

## What it does

- **Server paging.** The owner holds the page (`{offset, limit}`), the grid
  shows the server's range and total ("1–200 of 460"), a page size (50 /
  100 / 200, the APIs' cap), first/previous/next. The hooks keep the last
  page on screen while the next loads (`placeholderData: keepPreviousData`).
- **Windowed rows.** `@tanstack/react-virtual` (pinned 3.14.13): only the
  rows in view (+10) are in the DOM — 40 of 200 in the dev check. Dense
  32px rows, sticky header.
- **Columns** are DataTable's: the same `ColumnDef<typeof dataTableFeatures>`,
  Columns menu, drag to reorder, resize, remembered under `persistKey`.
  The shared parts live in `components/ui/table-parts.tsx` (DataTable uses
  them too).
- **No client search or sort.** On one page of many they would describe
  that page alone. Instead the toolbar (`components/shared/log-grid-toolbar.tsx`)
  has a date range (UTC days, the APIs' window; default the last 7) and an
  **exact click ID** filter, applied on Enter/blur — `clickId` on
  `GET /click-log`, `/postback-logs`, `/conversions` (`chstore.logWindow`:
  `AND click_id = ?` after the organization filter).
- **Export** walks every page of the current filter (`collectPages`, 200
  per request) up to 10,000 rows, with progress on the button; past that a
  warning says how many of how many were exported.
- **Keyboard.** The table is `role="grid"`, focusable: ↑/↓, PageUp/PageDown,
  Home/End move the active row (`aria-activedescendant`), Enter (or a
  click) calls `onRowActivate` — Conversions opens the click's timeline.
- Row keys are `getRowId(row)#index`: two log rows can be alike in every
  field (two postback attempts in the same millisecond).

`components/shared/log-grid.tsx` wraps it for a log page: the filters stay
visible on loading and on error (a range over the APIs' 90 days is refused
— the user has to be able to change it back).

## Tests

`data-grid.test.tsx`: page arithmetic, `collectPages` (all pages, the cap,
a short page), only the rows in view rendered, next/previous/last page,
keyboard and Enter, duplicate ids, export across pages with the cap and
CSV quoting — mutation-checked (render every row, next always enabled,
cap ignored, ArrowDown broken, keys without the index). Go:
`chstore.TestLogsFilterByClickID` (each log: the click, an unknown id,
another organization's click id → none; count agrees).

Live (2026-10-15): 450 synthetic clicks in the demo organization → "1–50
of 460", 200 per page, page 2 "201–400", End to the last row, the click ID
filter → one row, export → 460 rows from three requests; conversions and
postback logs on the grid with no console errors. The synthetic rows were
removed afterwards (and the geo rollup rebuilt for those days).
