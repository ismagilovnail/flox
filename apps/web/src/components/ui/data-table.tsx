"use client"

import * as React from "react"
import { useTable, type ColumnDef, type RowSelectionState } from "@tanstack/react-table"
import { ChevronLeftIcon, ChevronRightIcon, DownloadIcon, SearchIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { EmptyState } from "@/components/ui/empty-state"
import {
  ColumnsMenu,
  SELECT_COLUMN_ID,
  dataTableFeatures,
  TableHeaderCell,
  downloadCsv,
  exportableColumns,
  toCsv,
  useColumnState,
  useHeaderInteractions,
} from "@/components/ui/table-parts"


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

/**
 * Client-side table: sort, paginate, search, column visibility. For a list
 * the server pages (a log of events), use DataGrid instead: paging here
 * would only ever see the rows of the first request.
 */
function DataTable<TData extends Record<string, unknown>>({
  columns,
  data,
  searchPlaceholder,
  enableSearch = true,
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
  persistKey,
}: {
  columns: ColumnDef<typeof dataTableFeatures, TData>[]
  data: TData[]
  searchPlaceholder?: string
  /** Set false to drop the search box (a short list where searching adds nothing). Every other table keeps it. */
  enableSearch?: boolean
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
  /** Opt-in: remembers the table's own column visibility, order and widths in this browser under this key (ignored for the parts the owner controls). */
  persistKey?: string
}) {
  const { t } = useTranslation("common")
  const [sorting, setSorting] = React.useState<
    { id: string; desc: boolean }[]
  >([])
  const [globalFilter, setGlobalFilter] = React.useState("")
  const { columnVisibility, setColumnVisibility, columnOrder, setColumnOrder, columnSizing, setColumnSizing } = useColumnState({
    initialColumnVisibility,
    columnOrder: controlledColumnOrder,
    onColumnOrderChange,
    columnSizing: controlledColumnSizing,
    onColumnSizingChange,
    columnVisibility: controlledColumnVisibility,
    onColumnVisibilityChange,
    persistKey,
  })
  const [pagination, setPagination] = React.useState({
    pageIndex: 0,
    pageSize,
  })
  const [rowSelection, setRowSelection] = React.useState<RowSelectionState>({})

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

  const interactions = useHeaderInteractions(table, setColumnOrder, setColumnSizing)

  const rows = table.getRowModel().rows
  const selectedRows = enableRowSelection ? table.getSelectedRowModel().rows.map((r) => r.original) : []

  // Every row matching the current search/sort, across every page — not
  // just the rows on screen — since "export what I've filtered down to" is
  // the useful contract for a CSV button. Values come from the accessors
  // (the same the search filters with): a column with only a `cell`
  // renderer exports empty, as it is unsearchable.
  function exportCsv() {
    const cols = exportableColumns(table)
    const body = table.getSortedRowModel().rows.map((row) => cols.map((c) => row.getValue(c.id)))
    downloadCsv(exportFilename ?? "export", toCsv(cols.map((c) => c.columnDef.header as string), body))
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
          {enableSearch && (
            <div className="relative w-64">
              <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={globalFilter}
                onChange={(e) => setGlobalFilter(e.target.value)}
                placeholder={searchPlaceholder ?? t("dataTable.searchPlaceholder")}
                className="h-8 pl-8"
              />
            </div>
          )}
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
          <ColumnsMenu table={table} />
        </div>
      </div>

      <div className="overflow-x-auto rounded-lg ring-1 ring-foreground/10">
        <table className="w-full text-sm">
          <thead>
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
                  />
                ))}
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

// dataTableFeatures lives with the shared parts (DataGrid uses it too);
// column files import it from here.
export { DataTable, dataTableFeatures }
