"use client"

import * as React from "react"
import {
  columnVisibilityFeature,
  columnOrderingFeature,
  columnFilteringFeature,
  columnResizingFeature,
  columnSizingFeature,
  globalFilteringFeature,
  createFilteredRowModel,
  createPaginatedRowModel,
  createSortedRowModel,
  filterFn_includesString,
  rowPaginationFeature,
  rowSelectionFeature,
  rowSortingFeature,
  sortFn_alphanumeric,
  sortFn_text,
  tableFeatures,
  useTable,
  type ColumnDef,
  type ColumnSizingState,
  type RowSelectionState,
} from "@tanstack/react-table"
import {
  ArrowDownIcon,
  ArrowUpIcon,
  ArrowUpDownIcon,
  ChevronLeftIcon,
  CircleHelpIcon,
  GripVerticalIcon,
  ChevronRightIcon,
  DownloadIcon,
  SearchIcon,
} from "lucide-react"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { EmptyState } from "@/components/ui/empty-state"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

export const dataTableFeatures = tableFeatures({
  rowSortingFeature,
  sortedRowModel: createSortedRowModel(),
  sortFns: { alphanumeric: sortFn_alphanumeric, text: sortFn_text },
  rowPaginationFeature,
  paginatedRowModel: createPaginatedRowModel(),
  columnFilteringFeature,
  globalFilteringFeature,
  filteredRowModel: createFilteredRowModel(),
  filterFns: { includesString: filterFn_includesString },
  columnVisibilityFeature,
  columnOrderingFeature,
  rowSelectionFeature,
  columnSizingFeature,
  columnResizingFeature,
})

const SELECT_COLUMN_ID = "__select__"

function buildSelectColumn<TData extends Record<string, unknown>>(
  selectAllLabel: string,
  selectRowLabel: string,
): ColumnDef<typeof dataTableFeatures, TData> {
  return {
    id: SELECT_COLUMN_ID,
    header: ({ table }) => (
      <Checkbox
        checked={table.getIsAllPageRowsSelected() ? true : table.getIsSomePageRowsSelected() ? "indeterminate" : false}
        onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
        aria-label={selectAllLabel}
      />
    ),
    cell: ({ row }) => (
      <Checkbox
        checked={row.getIsSelected()}
        onCheckedChange={(value) => row.toggleSelected(!!value)}
        onClick={(e) => e.stopPropagation()}
        aria-label={selectRowLabel}
      />
    ),
    enableSorting: false,
    enableHiding: false,
    enableResizing: false,
  }
}

/** RFC 4180-ish CSV cell quoting: wrap in quotes and escape embedded quotes
 * whenever the value contains a comma, quote, or newline — the three cases
 * a bare value would otherwise corrupt the file structure for. */
function csvCell(value: unknown): string {
  if (value === null || value === undefined) return ""
  const s = value instanceof Date ? value.toISOString() : String(value)
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}

/**
 * Client-side table: sort, paginate, search, column visibility. "Virtualize
 * large sets" (UX floor) is handled by pagination rather than DOM windowing
 * — scannable rows stay bounded per page instead of an unbounded live list.
 */
/** Optional column explanation (`columnDef.meta.hint`) shown as a tooltip on the header. */
function HeaderHint({ hint, children }: { hint?: string; children: React.ReactElement }) {
  if (!hint) return children
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent className="max-w-64">{hint}</TooltipContent>
    </Tooltip>
  )
}

function DataTable<TData extends Record<string, unknown>>({
  columns,
  data,
  searchPlaceholder,
  emptyTitle,
  emptyDescription,
  pageSize = 20,
  className,
  filters,
  enableRowSelection = false,
  getRowId,
  bulkActions,
  initialColumnVisibility,
  exportFilename,
  enableColumnReorder = false,
  columnOrder: controlledColumnOrder,
  onColumnOrderChange,
  columnSizing: controlledColumnSizing,
  onColumnSizingChange,
  columnVisibility: controlledColumnVisibility,
  onColumnVisibilityChange,
}: {
  columns: ColumnDef<typeof dataTableFeatures, TData>[]
  data: TData[]
  searchPlaceholder?: string
  emptyTitle?: string
  emptyDescription?: string
  pageSize?: number
  className?: string
  /** Extra toolbar controls rendered between search and the Columns menu — e.g. a tag filter. */
  filters?: React.ReactNode
  /** Opt-in row selection + a checkbox column. Every existing call site keeps working unchanged when omitted. */
  enableRowSelection?: boolean
  /** Required when enableRowSelection is true — selection state is keyed by this, not row index. */
  getRowId?: (row: TData) => string
  /** Rendered in a toolbar that appears only while rows are selected. */
  bulkActions?: (ctx: { selectedRows: TData[]; clearSelection: () => void }) => React.ReactNode
  /** Columns hidden on first render (still toggleable via the Columns menu) — for a table with more columns than fit comfortably at once. Omitted means every column starts visible, as before. */
  initialColumnVisibility?: Record<string, boolean>
  /** Base filename (without .csv) for the Export CSV button. Falls back to a generic name when omitted — every existing call site keeps working unchanged. */
  exportFilename?: string
  /** Opt-in: header labels can be dragged to reorder columns. Omitted keeps the fixed order, as before. */
  enableColumnReorder?: boolean
  /** Controlled column order (column ids): lets the owner persist it, e.g. in a saved report preset. Omitted, the table keeps its own. */
  columnOrder?: string[]
  onColumnOrderChange?: (order: string[]) => void
  /** Controlled column widths in px by column id ({} = natural widths): lets the owner persist them, e.g. in a saved report preset. Omitted, the table keeps its own. */
  columnSizing?: Record<string, number>
  onColumnSizingChange?: (sizing: Record<string, number>) => void
  /** Controlled visibility (id -> shown); absent = the table keeps it itself. */
  columnVisibility?: Record<string, boolean>
  onColumnVisibilityChange?: (visibility: Record<string, boolean>) => void
}) {
  const { t } = useTranslation("common")
  const [sorting, setSorting] = React.useState<
    { id: string; desc: boolean }[]
  >([])
  const [globalFilter, setGlobalFilter] = React.useState("")
  const [internalColumnVisibility, setInternalColumnVisibility] = React.useState<
    Record<string, boolean>
  >(initialColumnVisibility ?? {})
  const columnVisibility = controlledColumnVisibility ?? internalColumnVisibility
  const setColumnVisibility = (
    next: Record<string, boolean> | ((old: Record<string, boolean>) => Record<string, boolean>),
  ) => {
    const resolved = typeof next === "function" ? next(columnVisibility) : next
    setInternalColumnVisibility(resolved)
    onColumnVisibilityChange?.(resolved)
  }
  const [pagination, setPagination] = React.useState({
    pageIndex: 0,
    pageSize,
  })
  const [internalColumnOrder, setInternalColumnOrder] = React.useState<string[]>([])
  const columnOrder = controlledColumnOrder ?? internalColumnOrder
  const setColumnOrder = (next: string[] | ((old: string[]) => string[])) => {
    const resolved = typeof next === "function" ? next(columnOrder) : next
    setInternalColumnOrder(resolved)
    onColumnOrderChange?.(resolved)
  }
  const [resizingColumnId, setResizingColumnId] = React.useState<string | null>(null)
  const [dragColumnId, setDragColumnId] = React.useState<string | null>(null)
  const [dropColumnId, setDropColumnId] = React.useState<string | null>(null)
  const [rowSelection, setRowSelection] = React.useState<RowSelectionState>({})
  // Empty by default means every column renders at its natural, auto-layout
  // width exactly as before this feature existed — a width only ever gets
  // pinned here once a user actually drags that column's own resize handle
  // (below), so tables nobody resizes look and behave identically to
  // before, with zero visual change across the app.
  const [internalColumnSizing, setInternalColumnSizing] = React.useState<ColumnSizingState>({})
  const columnSizing = controlledColumnSizing ?? internalColumnSizing
  const setColumnSizing = (next: ColumnSizingState | ((old: ColumnSizingState) => ColumnSizingState)) => {
    const resolved = typeof next === "function" ? next(columnSizing) : next
    setInternalColumnSizing(resolved)
    onColumnSizingChange?.(resolved)
  }

  const tableColumns = React.useMemo(
    () =>
      enableRowSelection
        ? [buildSelectColumn<TData>(t("dataTable.selectAllAria"), t("dataTable.selectRowAria")), ...columns]
        : columns,
    [enableRowSelection, columns, t],
  )

  const table = useTable({
    features: dataTableFeatures,
    columns: tableColumns,
    data,
    state: { sorting, globalFilter, columnVisibility, pagination, rowSelection, columnSizing, columnOrder },
    onSortingChange: setSorting,
    onGlobalFilterChange: setGlobalFilter,
    onColumnVisibilityChange: setColumnVisibility,
    onPaginationChange: setPagination,
    onRowSelectionChange: setRowSelection,
    onColumnSizingChange: setColumnSizing,
    onColumnOrderChange: setColumnOrder,
    enableColumnResizing: true,
    columnResizeMode: "onChange",
    getRowId: getRowId as ((row: TData) => string) | undefined,
    globalFilterFn: "includesString",
  })

  function moveColumn(fromId: string, toId: string) {
    if (fromId === toId) return
    const ids = table.getAllLeafColumns().map((c) => c.id)
    const from = ids.indexOf(fromId)
    const to = ids.indexOf(toId)
    if (from < 0 || to < 0) return
    ids.splice(to, 0, ids.splice(from, 1)[0])
    setColumnOrder(ids)
  }

  /** Own resize handler instead of header.getResizeHandler(): TanStack measures the
   * start size from the column def (150 by default), not the rendered width, so a
   * column that auto-layout made wider snapped to ~150px on the first pixel dragged. */
  function startColumnResize(
    e: React.MouseEvent<HTMLElement> | React.TouchEvent<HTMLElement>,
    columnId: string,
  ) {
    const th = e.currentTarget.parentElement
    if (!th) return
    const isTouch = "touches" in e
    const startX = isTouch ? e.touches[0].clientX : e.clientX
    const startWidth = th.getBoundingClientRect().width
    const resize = (clientX: number) =>
      setColumnSizing((prev) => ({ ...prev, [columnId]: Math.max(48, Math.round(startWidth + clientX - startX)) }))
    setResizingColumnId(columnId)
    if (isTouch) {
      const onMove = (ev: TouchEvent) => resize(ev.touches[0].clientX)
      const onEnd = () => {
        document.removeEventListener("touchmove", onMove)
        document.removeEventListener("touchend", onEnd)
        document.removeEventListener("touchcancel", onEnd)
        setResizingColumnId(null)
      }
      document.addEventListener("touchmove", onMove)
      document.addEventListener("touchend", onEnd)
      document.addEventListener("touchcancel", onEnd)
      return
    }
    e.preventDefault()
    const onMove = (ev: MouseEvent) => resize(ev.clientX)
    const onUp = () => {
      document.removeEventListener("mousemove", onMove)
      document.removeEventListener("mouseup", onUp)
      setResizingColumnId(null)
    }
    document.addEventListener("mousemove", onMove)
    document.addEventListener("mouseup", onUp)
  }

  const rows = table.getRowModel().rows
  const selectedRows = enableRowSelection ? table.getSelectedRowModel().rows.map((r) => r.original) : []

  // Every row matching the current search/sort, across every page — not
  // just the rows currently on screen (table.getRowModel(), post-
  // pagination) — since "export what I've filtered down to" is the
  // useful contract for a CSV button, not "export this one page." Value
  // extraction is accessor-based (cell.getValue()), the same mechanism
  // the search box above already filters with — a column with no
  // accessor (a pure computed/joined `cell` renderer and no
  // accessorKey/accessorFn) was already unsearchable for that same
  // reason and exports as an empty column here too, a pre-existing,
  // consistent limitation rather than a new one. Columns with no
  // meaningful string header (the row-actions column's icon buttons,
  // conventionally `header: ""`) are skipped outright, not exported as
  // an empty trailing column with nothing to label it.
  function exportCsv() {
    const exportColumns = table
      .getVisibleLeafColumns()
      .filter((c) => c.id !== SELECT_COLUMN_ID && typeof c.columnDef.header === "string" && c.columnDef.header !== "")
    const header = exportColumns.map((c) => csvCell(c.columnDef.header as string))
    const body = table.getSortedRowModel().rows.map((row) =>
      exportColumns.map((c) => csvCell(row.getValue(c.id))).join(","),
    )
    const csv = [header.join(","), ...body].join("\n")
    const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" })
    const url = URL.createObjectURL(blob)
    const a = document.createElement("a")
    a.href = url
    a.download = `${exportFilename ?? "export"}.csv`
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div data-slot="data-table" className={cn("flex flex-col gap-3", className)}>
      {enableRowSelection && selectedRows.length > 0 && (
        <div className="flex items-center gap-2 rounded-md border border-border bg-muted/40 px-3 py-2 text-sm">
          <span className="font-medium">{t("dataTable.selected", { count: selectedRows.length })}</span>
          {bulkActions?.({ selectedRows, clearSelection: () => table.setRowSelection({}) })}
          <Button
            variant="ghost"
            size="sm"
            className="ml-auto"
            onClick={() => table.setRowSelection({})}
          >
            {t("actions.clear")}
          </Button>
        </div>
      )}

      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative w-64">
            <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={globalFilter}
              onChange={(e) => setGlobalFilter(e.target.value)}
              placeholder={searchPlaceholder ?? t("dataTable.searchPlaceholder")}
              className="h-8 pl-8"
            />
          </div>
          {filters}
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            className="gap-1.5"
            disabled={rows.length === 0}
            onClick={exportCsv}
          >
            <DownloadIcon className="size-3.5" />
            {t("dataTable.exportCsv")}
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="sm">
                {t("dataTable.columns")}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {table
                .getAllLeafColumns()
                .filter((column) => column.getCanHide())
                .map((column) => (
                  <DropdownMenuCheckboxItem
                    key={column.id}
                    checked={column.getIsVisible()}
                    onCheckedChange={(value) => column.toggleVisibility(!!value)}
                    onSelect={(e) => e.preventDefault()}
                  >
                    {String(column.columnDef.header ?? column.id)}
                  </DropdownMenuCheckboxItem>
                ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <div className="overflow-x-auto rounded-lg ring-1 ring-foreground/10">
        <table className="w-full text-sm">
          <thead>
            {table.getHeaderGroups().map((headerGroup) => (
              <tr key={headerGroup.id} className="border-b border-border">
                {headerGroup.headers.map((header) => {
                  const sortDir = header.column.getIsSorted()
                  // A width is only ever pinned once this column's own
                  // entry exists in columnSizing (i.e. someone actually
                  // dragged its handle) — every other column keeps
                  // rendering at its natural, auto-layout width exactly
                  // as before this feature existed.
                  const resized = header.column.id in columnSizing
                  return (
                    <th
                      key={header.id}
                      className={cn(
                        "relative h-9 px-3 text-left align-middle text-xs font-medium text-muted-foreground",
                        enableColumnReorder && dropColumnId === header.column.id && dragColumnId !== header.column.id && "bg-primary/10",
                      )}
                      onDragOver={
                        enableColumnReorder && dragColumnId
                          ? (e) => {
                              e.preventDefault()
                              setDropColumnId(header.column.id)
                            }
                          : undefined
                      }
                      onDrop={
                        enableColumnReorder && dragColumnId
                          ? (e) => {
                              e.preventDefault()
                              moveColumn(dragColumnId, header.column.id)
                              setDragColumnId(null)
                              setDropColumnId(null)
                            }
                          : undefined
                      }
                      style={resized ? { width: header.getSize(), minWidth: header.getSize(), maxWidth: header.getSize() } : undefined}
                    >
                      {header.isPlaceholder ? null : header.column.getCanSort() ? (
                        <HeaderHint hint={(header.column.columnDef.meta as { hint?: string } | undefined)?.hint}>
                        <button
                          type="button"
                          onClick={header.column.getToggleSortingHandler()}
                          draggable={enableColumnReorder && header.column.id !== SELECT_COLUMN_ID}
                          onDragStart={(e) => {
                            e.dataTransfer.effectAllowed = "move"
                            setDragColumnId(header.column.id)
                          }}
                          onDragEnd={() => {
                            setDragColumnId(null)
                            setDropColumnId(null)
                          }}
                          title={
                            enableColumnReorder &&
                            header.column.id !== SELECT_COLUMN_ID &&
                            !(header.column.columnDef.meta as { hint?: string } | undefined)?.hint
                              ? t("dataTable.reorderColumnAria")
                              : undefined
                          }
                          className={cn(
                            "inline-flex max-w-full items-center gap-1 truncate hover:text-foreground",
                            enableColumnReorder && "cursor-grab active:cursor-grabbing",
                            dragColumnId === header.column.id && "opacity-50",
                          )}
                        >
                          {enableColumnReorder && header.column.id !== SELECT_COLUMN_ID && (
                            <GripVerticalIcon
                              className="-ml-1.5 size-3 shrink-0 opacity-40"
                              aria-hidden
                            />
                          )}
                          <table.FlexRender header={header} />
                          {(header.column.columnDef.meta as { hint?: string } | undefined)?.hint && (
                            <CircleHelpIcon className="size-3 shrink-0 opacity-60" />
                          )}
                          {sortDir === "asc" ? (
                            <ArrowUpIcon className="size-3 shrink-0" />
                          ) : sortDir === "desc" ? (
                            <ArrowDownIcon className="size-3 shrink-0" />
                          ) : (
                            <ArrowUpDownIcon className="size-3 shrink-0 opacity-40" />
                          )}
                        </button>
                        </HeaderHint>
                      ) : (
                        <table.FlexRender header={header} />
                      )}
                      {header.column.getCanResize() && (
                        <div
                          onMouseDown={(e) => startColumnResize(e, header.column.id)}
                          onTouchStart={(e) => startColumnResize(e, header.column.id)}
                          onClick={(e) => e.stopPropagation()}
                          role="separator"
                          aria-orientation="vertical"
                          aria-label={t("dataTable.resizeColumnAria")}
                          title={t("dataTable.resizeColumnAria")}
                          className={cn(
                            "group/resize absolute right-0 top-0 h-full w-1.5 -translate-x-1/2 cursor-col-resize touch-none select-none after:absolute after:inset-y-2 after:left-1/2 after:w-px after:-translate-x-1/2 after:bg-border after:content-[''] hover:bg-primary/50 hover:after:bg-transparent",
                            resizingColumnId === header.column.id && "bg-primary",
                          )}
                        />
                      )}
                    </th>
                  )
                })}
              </tr>
            ))}
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr>
                <td colSpan={table.getAllLeafColumns().length} className="p-0">
                  <EmptyState title={emptyTitle ?? t("dataTable.emptyTitle")} description={emptyDescription} />
                </td>
              </tr>
            ) : (
              rows.map((row) => (
                <tr
                  key={row.id}
                  className="border-b border-border last:border-0 hover:bg-muted/40"
                >
                  {row.getVisibleCells().map((cell) => {
                    const resized = cell.column.id in columnSizing
                    return (
                      <td
                        key={cell.id}
                        className={cn("h-10 px-3 align-middle", resized && "overflow-hidden text-ellipsis whitespace-nowrap")}
                        style={resized ? { width: cell.column.getSize(), maxWidth: cell.column.getSize() } : undefined}
                      >
                        <table.FlexRender cell={cell} />
                      </td>
                    )
                  })}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {rows.length > 0 && (
        <div className="flex items-center justify-between text-xs text-muted-foreground">
          <span className="font-mono font-tabular">
            {t("dataTable.pagination", {
              page: table.state.pagination.pageIndex + 1,
              total: Math.max(table.getPageCount(), 1),
              count: table.getRowCount(),
            })}
          </span>
          <div className="flex items-center gap-1">
            <Button
              variant="outline"
              size="icon-sm"
              aria-label={t("dataTable.previousPageAria")}
              disabled={!table.getCanPreviousPage()}
              onClick={() => table.previousPage()}
            >
              <ChevronLeftIcon className="size-3.5" />
            </Button>
            <Button
              variant="outline"
              size="icon-sm"
              aria-label={t("dataTable.nextPageAria")}
              disabled={!table.getCanNextPage()}
              onClick={() => table.nextPage()}
            >
              <ChevronRightIcon className="size-3.5" />
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}

export { DataTable }
