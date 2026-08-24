"use client"

import * as React from "react"
import {
  columnVisibilityFeature,
  columnFilteringFeature,
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
  type RowSelectionState,
} from "@tanstack/react-table"
import {
  ArrowDownIcon,
  ArrowUpIcon,
  ArrowUpDownIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  SearchIcon,
} from "lucide-react"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
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
  rowSelectionFeature,
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
  }
}

/**
 * Client-side table: sort, paginate, search, column visibility. "Virtualize
 * large sets" (UX floor) is handled by pagination rather than DOM windowing
 * — scannable rows stay bounded per page instead of an unbounded live list.
 */
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
}) {
  const { t } = useTranslation("common")
  const [sorting, setSorting] = React.useState<
    { id: string; desc: boolean }[]
  >([])
  const [globalFilter, setGlobalFilter] = React.useState("")
  const [columnVisibility, setColumnVisibility] = React.useState<
    Record<string, boolean>
  >({})
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
    state: { sorting, globalFilter, columnVisibility, pagination, rowSelection },
    onSortingChange: setSorting,
    onGlobalFilterChange: setGlobalFilter,
    onColumnVisibilityChange: setColumnVisibility,
    onPaginationChange: setPagination,
    onRowSelectionChange: setRowSelection,
    getRowId: getRowId as ((row: TData) => string) | undefined,
    globalFilterFn: "includesString",
  })

  const rows = table.getRowModel().rows
  const selectedRows = enableRowSelection ? table.getSelectedRowModel().rows.map((r) => r.original) : []

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

      <div className="overflow-x-auto rounded-lg ring-1 ring-foreground/10">
        <table className="w-full text-sm">
          <thead>
            {table.getHeaderGroups().map((headerGroup) => (
              <tr key={headerGroup.id} className="border-b border-border">
                {headerGroup.headers.map((header) => {
                  const sortDir = header.column.getIsSorted()
                  return (
                    <th
                      key={header.id}
                      className="h-9 px-3 text-left align-middle text-xs font-medium text-muted-foreground"
                    >
                      {header.isPlaceholder ? null : header.column.getCanSort() ? (
                        <button
                          type="button"
                          onClick={header.column.getToggleSortingHandler()}
                          className="inline-flex items-center gap-1 hover:text-foreground"
                        >
                          <table.FlexRender header={header} />
                          {sortDir === "asc" ? (
                            <ArrowUpIcon className="size-3" />
                          ) : sortDir === "desc" ? (
                            <ArrowDownIcon className="size-3" />
                          ) : (
                            <ArrowUpDownIcon className="size-3 opacity-40" />
                          )}
                        </button>
                      ) : (
                        <table.FlexRender header={header} />
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
                  {row.getVisibleCells().map((cell) => (
                    <td key={cell.id} className="h-10 px-3 align-middle">
                      <table.FlexRender cell={cell} />
                    </td>
                  ))}
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
