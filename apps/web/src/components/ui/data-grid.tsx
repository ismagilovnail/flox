"use client"

import * as React from "react"
import { useTable, type ColumnDef } from "@tanstack/react-table"
import { useVirtualizer } from "@tanstack/react-virtual"
import { ChevronLeftIcon, ChevronRightIcon, ChevronsLeftIcon, DownloadIcon, Loader2Icon } from "lucide-react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { EmptyState } from "@/components/ui/empty-state"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import {
  ColumnsMenu,
  TableHeaderCell,
  dataTableFeatures,
  downloadCsv,
  exportableColumns,
  toCsv,
  useColumnState,
  useHeaderInteractions,
} from "@/components/ui/table-parts"
import {
  collectPages,
  hasNextPage,
  nextPage,
  pageRange,
  previousPage,
  withPageSize,
  type FetchPage,
  type GridPage,
} from "@/components/ui/data-grid-paging"

export type { GridPage, FetchPage }

const ROW_HEIGHT = 32
/** Rows kept rendered above and below the visible ones. */
const OVERSCAN = 10
/** What the viewport is assumed to be before it is measured (and in tests). */
const INITIAL_RECT = { width: 1200, height: 560 }

/**
 * A server-paged grid for event logs (§16 DataGrid; UX floor "virtualize
 * large sets"): the server holds the rows and their order, this shows one
 * page of them — the range and total from the server, a page size, rows
 * windowed so only the visible ones are in the DOM. Column definitions are
 * DataTable's (visibility, order, widths, remembered per browser).
 *
 * Deliberately no client-side search or sort: on one page of many they
 * would quietly describe that page alone. Filter with `toolbar` (a date
 * range…) and the API instead. Export walks every page of the current
 * filter (fetchPage), up to `exportCap` rows, and says when it stopped there.
 *
 * Keyboard: the grid is focusable; ↑/↓ move the active row, Home/End jump,
 * PageUp/PageDown move a screen, Enter activates (onRowActivate).
 */
function DataGrid<TData extends Record<string, unknown>>({
  columns,
  rows,
  total,
  page,
  onPageChange,
  pageSizes = [50, 100, 200],
  isFetching = false,
  getRowId,
  toolbar,
  emptyTitle,
  emptyDescription,
  exportFilename,
  fetchPage,
  exportCap = 10_000,
  initialColumnVisibility,
  enableColumnReorder = false,
  persistKey,
  onRowActivate,
  ariaLabel,
  className,
}: {
  columns: ColumnDef<typeof dataTableFeatures, TData>[]
  /** The current page's rows, in the server's order. */
  rows: TData[]
  /** Rows the server has for the current filter, across every page. */
  total: number
  page: GridPage
  onPageChange: (page: GridPage) => void
  pageSizes?: number[]
  /** A page is loading: the current rows stay, dimmed. */
  isFetching?: boolean
  getRowId: (row: TData) => string
  /** Filters (date range…) rendered left of the toolbar's buttons. */
  toolbar?: React.ReactNode
  emptyTitle?: string
  emptyDescription?: string
  exportFilename?: string
  /** Reads one page for export; omitted, the Export button is not shown. */
  fetchPage?: FetchPage<TData>
  exportCap?: number
  initialColumnVisibility?: Record<string, boolean>
  enableColumnReorder?: boolean
  persistKey?: string
  /** Enter (or a click) on a row. */
  onRowActivate?: (row: TData) => void
  ariaLabel: string
  className?: string
}) {
  const { t } = useTranslation("common")
  const columnState = useColumnState({ initialColumnVisibility, persistKey })
  const { columnVisibility, setColumnVisibility, columnOrder, setColumnOrder, columnSizing, setColumnSizing } = columnState

  const table = useTable({
    features: dataTableFeatures,
    columns,
    data: rows,
    state: { columnVisibility, columnSizing, columnOrder },
    onColumnVisibilityChange: setColumnVisibility,
    onColumnSizingChange: setColumnSizing,
    onColumnOrderChange: setColumnOrder,
    // The server pages and orders: the table neither.
    manualPagination: true,
    manualSorting: true,
    enableSorting: false,
    enableColumnResizing: true,
    columnResizeMode: "onChange",
    // A log can hold two rows alike in every field the owner keys on (two
    // attempts in the same millisecond): the position on the page keeps
    // React keys unique whatever getRowId returns.
    getRowId: (row, index) => `${getRowId(row)}#${index}`,
  })
  const interactions = useHeaderInteractions(table, setColumnOrder, setColumnSizing)
  const tableRows = table.getRowModel().rows
  const visibleColumnCount = table.getVisibleLeafColumns().length

  const scrollRef = React.useRef<HTMLDivElement>(null)
  // TanStack Virtual returns fresh functions each render, so the React
  // Compiler leaves this component unoptimized — expected, and harmless here.
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: tableRows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: OVERSCAN,
    initialRect: INITIAL_RECT,
  })
  const virtualRows = virtualizer.getVirtualItems()
  const padTop = virtualRows.length > 0 ? virtualRows[0].start : 0
  const padBottom = virtualRows.length > 0 ? virtualizer.getTotalSize() - virtualRows[virtualRows.length - 1].end : 0

  // The active row belongs to a page: a new page starts with none.
  const [active, setActive] = React.useState<{ offset: number; index: number }>({ offset: page.offset, index: -1 })
  const activeIndex = active.offset === page.offset && active.index < tableRows.length ? active.index : -1

  function goTo(page: GridPage) {
    if (scrollRef.current) scrollRef.current.scrollTop = 0
    onPageChange(page)
  }

  function moveActive(index: number) {
    if (tableRows.length === 0) return
    const clamped = Math.max(0, Math.min(tableRows.length - 1, index))
    setActive({ offset: page.offset, index: clamped })
    virtualizer.scrollToIndex(clamped, { align: "auto" })
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTableElement>) {
    const screen = Math.max(1, Math.floor((scrollRef.current?.clientHeight ?? INITIAL_RECT.height) / ROW_HEIGHT) - 1)
    const keys: Record<string, () => void> = {
      ArrowDown: () => moveActive(activeIndex + 1),
      ArrowUp: () => moveActive(activeIndex < 0 ? 0 : activeIndex - 1),
      PageDown: () => moveActive(activeIndex + screen),
      PageUp: () => moveActive(activeIndex - screen),
      Home: () => moveActive(0),
      End: () => moveActive(tableRows.length - 1),
      Enter: () => {
        if (activeIndex >= 0) onRowActivate?.(tableRows[activeIndex].original)
      },
    }
    const action = keys[e.key]
    if (!action) return
    e.preventDefault()
    action()
  }

  const [exporting, setExporting] = React.useState<{ done: number; total: number } | null>(null)
  async function exportCsv() {
    if (!fetchPage) return
    const cols = exportableColumns(table)
    setExporting({ done: 0, total })
    try {
      const result = await collectPages(fetchPage, {
        pageSize: Math.max(...pageSizes),
        cap: exportCap,
        onProgress: (done, total) => setExporting({ done, total }),
      })
      const body = result.rows.map((row, i) => cols.map((c) => c.accessorFn?.(row, i)))
      downloadCsv(exportFilename ?? "export", toCsv(cols.map((c) => c.columnDef.header as string), body))
      if (result.truncated) {
        toast.warning(t("dataGrid.exportTruncated", { count: result.rows.length, total: result.total }))
      }
    } catch (err) {
      toast.error(t("dataGrid.exportError"), { description: err instanceof Error ? err.message : String(err) })
    } finally {
      setExporting(null)
    }
  }

  const range = pageRange(page, tableRows.length, total)
  const fmt = (n: number) => n.toLocaleString()
  const activeRowId = activeIndex >= 0 ? `${persistKey ?? "grid"}-row-${activeIndex}` : undefined

  return (
    <div data-slot="data-grid" className={cn("flex flex-col gap-3", className)}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">{toolbar}</div>
        <div className="flex items-center gap-2">
          {fetchPage && (
            <Button variant="outline" size="sm" className="gap-1.5" disabled={total === 0 || exporting !== null} onClick={exportCsv}>
              {exporting ? <Loader2Icon className="size-3.5 animate-spin" /> : <DownloadIcon className="size-3.5" />}
              {exporting
                ? t("dataGrid.exporting", { done: fmt(exporting.done), total: fmt(Math.min(exporting.total, exportCap)) })
                : t("dataTable.exportCsv")}
            </Button>
          )}
          <ColumnsMenu table={table} />
        </div>
      </div>

      <div
        ref={scrollRef}
        className={cn(
          "relative max-h-[70vh] overflow-auto rounded-lg ring-1 ring-foreground/10 has-[table:focus-visible]:ring-2 has-[table:focus-visible]:ring-ring",
          isFetching && "opacity-60 transition-opacity",
        )}
      >
        <table
          role="grid"
          aria-label={ariaLabel}
          aria-busy={isFetching}
          aria-rowcount={total + 1}
          aria-activedescendant={activeRowId}
          tabIndex={0}
          onKeyDown={onKeyDown}
          className="w-full text-xs outline-none"
        >
          <thead className="sticky top-0 z-10 bg-background">
            {table.getHeaderGroups().map((headerGroup) => (
              <tr key={headerGroup.id} className="border-b border-border">
                {headerGroup.headers.map((header) => (
                  <TableHeaderCell
                    key={header.id}
                    table={table}
                    header={header}
                    columnSizing={columnSizing}
                    enableColumnReorder={enableColumnReorder}
                    interactions={interactions}
                    className="h-8 whitespace-nowrap"
                  />
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {tableRows.length === 0 ? (
              <tr>
                <td colSpan={visibleColumnCount} className="p-0">
                  {isFetching ? (
                    <div className="h-24" />
                  ) : (
                    <EmptyState title={emptyTitle ?? t("dataTable.emptyTitle")} description={emptyDescription} />
                  )}
                </td>
              </tr>
            ) : (
              <>
                {padTop > 0 && (
                  <tr aria-hidden style={{ height: padTop }}>
                    <td colSpan={visibleColumnCount} />
                  </tr>
                )}
                {virtualRows.map((vr) => {
                  const row = tableRows[vr.index]
                  const isActive = vr.index === activeIndex
                  return (
                    <tr
                      key={row.id}
                      id={`${persistKey ?? "grid"}-row-${vr.index}`}
                      data-index={vr.index}
                      aria-rowindex={page.offset + vr.index + 2}
                      aria-selected={isActive}
                      onClick={() => {
                        setActive({ offset: page.offset, index: vr.index })
                        onRowActivate?.(row.original)
                      }}
                      style={{ height: ROW_HEIGHT }}
                      className={cn(
                        "border-b border-border last:border-0 hover:bg-muted/40",
                        isActive && "bg-primary/10 hover:bg-primary/15",
                        onRowActivate && "cursor-pointer",
                      )}
                    >
                      {row.getVisibleCells().map((cell) => {
                        const resized = cell.column.id in columnSizing
                        return (
                          <td
                            key={cell.id}
                            className="overflow-hidden px-3 align-middle text-ellipsis whitespace-nowrap"
                            style={resized ? { width: cell.column.getSize(), maxWidth: cell.column.getSize() } : undefined}
                          >
                            <table.FlexRender cell={cell} />
                          </td>
                        )
                      })}
                    </tr>
                  )
                })}
                {padBottom > 0 && (
                  <tr aria-hidden style={{ height: padBottom }}>
                    <td colSpan={visibleColumnCount} />
                  </tr>
                )}
              </>
            )}
          </tbody>
        </table>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
        <span className="font-mono font-tabular" aria-live="polite">
          {total === 0 ? t("dataGrid.noRows") : t("dataGrid.range", { first: fmt(range.first), last: fmt(range.last), total: fmt(total) })}
        </span>
        <div className="flex items-center gap-2">
          <span>{t("dataGrid.rowsPerPage")}</span>
          <Select value={String(page.limit)} onValueChange={(v) => goTo(withPageSize(Number(v)))}>
            <SelectTrigger size="sm" className="h-7 w-20 font-mono" aria-label={t("dataGrid.rowsPerPage")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {pageSizes.map((size) => (
                <SelectItem key={size} value={String(size)}>
                  {size}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            variant="outline"
            size="icon-sm"
            aria-label={t("dataGrid.firstPageAria")}
            disabled={page.offset === 0 || isFetching}
            onClick={() => goTo({ ...page, offset: 0 })}
          >
            <ChevronsLeftIcon className="size-3.5" />
          </Button>
          <Button
            variant="outline"
            size="icon-sm"
            aria-label={t("dataTable.previousPageAria")}
            disabled={page.offset === 0 || isFetching}
            onClick={() => goTo(previousPage(page))}
          >
            <ChevronLeftIcon className="size-3.5" />
          </Button>
          <Button
            variant="outline"
            size="icon-sm"
            aria-label={t("dataTable.nextPageAria")}
            disabled={!hasNextPage(page, total) || isFetching}
            onClick={() => goTo(nextPage(page))}
          >
            <ChevronRightIcon className="size-3.5" />
          </Button>
        </div>
      </div>
    </div>
  )
}

export { DataGrid }
