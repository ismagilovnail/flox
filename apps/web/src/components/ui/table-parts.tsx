"use client"

/**
 * What DataTable (client-side lists) and DataGrid (server-paged logs) share:
 * column state that can be controlled or remembered per browser, the header
 * cell (sort, hint, drag-to-reorder, resize handle), the Columns menu and
 * CSV export. Each table keeps its own body and pagination.
 */

import * as React from "react"
import {
  columnFilteringFeature,
  columnOrderingFeature,
  columnResizingFeature,
  columnSizingFeature,
  columnVisibilityFeature,
  createFilteredRowModel,
  createPaginatedRowModel,
  createSortedRowModel,
  filterFn_includesString,
  globalFilteringFeature,
  rowPaginationFeature,
  rowSelectionFeature,
  rowSortingFeature,
  sortFn_alphanumeric,
  sortFn_text,
  tableFeatures,
  type Header,
  type ReactTable,
  type RowData,
} from "@tanstack/react-table"
import { ArrowDownIcon, ArrowUpIcon, ArrowUpDownIcon, CircleHelpIcon, GripVerticalIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"

/** The feature set of DataTable and DataGrid: one, so a column definition
 * (`ColumnDef<typeof dataTableFeatures, Row>`) works in either. */
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

/** A table built with dataTableFeatures, as useTable returns it. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type FloxTable<TData extends RowData> = ReactTable<typeof dataTableFeatures, TData, any>

/** The row-selection checkbox column's id: never reordered or exported. */
export const SELECT_COLUMN_ID = "__select__"

/** RFC 4180-ish CSV cell quoting: wrap in quotes and escape embedded quotes
 * whenever the value contains a comma, quote, or newline — the three cases
 * a bare value would otherwise corrupt the file structure for. */
export function csvCell(value: unknown): string {
  if (value === null || value === undefined) return ""
  const s = value instanceof Date ? value.toISOString() : String(value)
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}

/** The CSV text of a header row and value rows. */
export function toCsv(header: string[], rows: unknown[][]): string {
  return [header.map(csvCell).join(","), ...rows.map((r) => r.map(csvCell).join(","))].join("\n")
}

export function downloadCsv(filename: string, csv: string) {
  const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" })
  const url = URL.createObjectURL(blob)
  const a = document.createElement("a")
  a.href = url
  a.download = `${filename}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

/** Columns worth a CSV column: visible, with a text header (the row-actions
 * column's `header: ""` and the selection checkbox are left out). */
export function exportableColumns<TData extends RowData>(table: FloxTable<TData>) {
  return table
    .getVisibleLeafColumns()
    .filter((c) => c.id !== SELECT_COLUMN_ID && typeof c.columnDef.header === "string" && c.columnDef.header !== "")
}

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

type PersistedTable = {
  visibility?: Record<string, boolean>
  order?: string[]
  sizing?: Record<string, number>
}

const persistedStorageKey = (key: string) => `flox.table.${key}`

function readPersisted(key: string): PersistedTable {
  try {
    const raw = window.localStorage.getItem(persistedStorageKey(key))
    return raw ? (JSON.parse(raw) as PersistedTable) : {}
  } catch {
    return {}
  }
}

const EMPTY_PERSISTED: PersistedTable = {}
const subscribePersisted = (onChange: () => void) => {
  window.addEventListener("storage", onChange)
  return () => window.removeEventListener("storage", onChange)
}

/** The stored choices for `key`. Empty on the server and on the first client
 * render (so hydration agrees), then what this browser remembers. */
function usePersistedTable(key: string | undefined): PersistedTable {
  const raw = React.useSyncExternalStore(
    subscribePersisted,
    () => {
      if (!key) return null
      try {
        return window.localStorage.getItem(persistedStorageKey(key))
      } catch {
        return null
      }
    },
    () => null,
  )
  return React.useMemo(() => {
    if (!raw) return EMPTY_PERSISTED
    try {
      return JSON.parse(raw) as PersistedTable
    } catch {
      return EMPTY_PERSISTED
    }
  }, [raw])
}

/** Merges `patch` into what is stored for `key`; storage errors (private window) are ignored. */
function writePersisted(key: string, patch: PersistedTable) {
  try {
    window.localStorage.setItem(persistedStorageKey(key), JSON.stringify({ ...readPersisted(key), ...patch }))
  } catch {
    // the choice just won't survive a reload
  }
}

type Updater<T> = T | ((old: T) => T)

export type ColumnStateOptions = {
  initialColumnVisibility?: Record<string, boolean>
  columnOrder?: string[]
  onColumnOrderChange?: (order: string[]) => void
  columnSizing?: Record<string, number>
  onColumnSizingChange?: (sizing: Record<string, number>) => void
  columnVisibility?: Record<string, boolean>
  onColumnVisibilityChange?: (visibility: Record<string, boolean>) => void
  persistKey?: string
}

/** Column visibility, order and widths: controlled by the owner where it
 * passes them, else the table's own, remembered under persistKey. */
export function useColumnState(o: ColumnStateOptions) {
  const persisted = usePersistedTable(o.persistKey)
  const [internalVisibility, setInternalVisibility] = React.useState<Record<string, boolean>>(o.initialColumnVisibility ?? {})
  const [internalOrder, setInternalOrder] = React.useState<string[]>([])
  // Empty by default means every column renders at its natural, auto-layout
  // width — a width only gets pinned once a user drags that column's handle.
  const [internalSizing, setInternalSizing] = React.useState<Record<string, number>>({})

  const columnVisibility = o.columnVisibility ?? persisted.visibility ?? internalVisibility
  const columnOrder = o.columnOrder ?? persisted.order ?? internalOrder
  const columnSizing = o.columnSizing ?? persisted.sizing ?? internalSizing

  const setColumnVisibility = (next: Updater<Record<string, boolean>>) => {
    const resolved = typeof next === "function" ? next(columnVisibility) : next
    setInternalVisibility(resolved)
    o.onColumnVisibilityChange?.(resolved)
    if (o.persistKey && !o.columnVisibility) writePersisted(o.persistKey, { visibility: resolved })
  }
  const setColumnOrder = (next: Updater<string[]>) => {
    const resolved = typeof next === "function" ? next(columnOrder) : next
    setInternalOrder(resolved)
    o.onColumnOrderChange?.(resolved)
    if (o.persistKey && !o.columnOrder) writePersisted(o.persistKey, { order: resolved })
  }
  const setColumnSizing = (next: Updater<Record<string, number>>) => {
    const resolved = typeof next === "function" ? next(columnSizing) : next
    setInternalSizing(resolved)
    o.onColumnSizingChange?.(resolved)
    if (o.persistKey && !o.columnSizing) writePersisted(o.persistKey, { sizing: resolved })
  }
  return { columnVisibility, setColumnVisibility, columnOrder, setColumnOrder, columnSizing, setColumnSizing }
}

/** Header drag-to-reorder and resize state, shared by every header cell of one table. */
export function useHeaderInteractions<TData extends RowData>(table: FloxTable<TData>, setColumnOrder: (order: string[]) => void, setColumnSizing: (next: Updater<Record<string, number>>) => void) {
  const [resizingColumnId, setResizingColumnId] = React.useState<string | null>(null)
  const [dragColumnId, setDragColumnId] = React.useState<string | null>(null)
  const [dropColumnId, setDropColumnId] = React.useState<string | null>(null)

  function moveColumn(fromId: string, toId: string) {
    // The selection checkbox column stays first: nothing is dropped onto it.
    if (fromId === toId || toId === SELECT_COLUMN_ID) return
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
  function startColumnResize(e: React.MouseEvent<HTMLElement> | React.TouchEvent<HTMLElement>, columnId: string) {
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

  return { resizingColumnId, dragColumnId, setDragColumnId, dropColumnId, setDropColumnId, moveColumn, startColumnResize }
}

export type HeaderInteractions = ReturnType<typeof useHeaderInteractions>

/** One header cell: the label (a sort toggle when the column sorts), its
 * hint, drag-to-reorder when enabled, and the resize handle. A width is
 * pinned only once the column's own entry exists in columnSizing. */
export function TableHeaderCell<TData extends RowData>({
  table,
  header,
  columnSizing,
  enableColumnReorder,
  interactions: ix,
  className,
}: {
  table: FloxTable<TData>
  header: Header<typeof dataTableFeatures, TData, unknown>
  columnSizing: Record<string, number>
  enableColumnReorder: boolean
  interactions: HeaderInteractions
  className?: string
}) {
  const { t } = useTranslation("common")
  const column = header.column
  const sortDir = column.getIsSorted()
  const resized = column.id in columnSizing
  const hint = (column.columnDef.meta as { hint?: string } | undefined)?.hint
  const reorderable = enableColumnReorder && column.id !== SELECT_COLUMN_ID
  const canSort = column.getCanSort()

  return (
    <th
      className={cn(
        "relative h-9 px-3 text-left align-middle text-xs font-medium text-muted-foreground",
        enableColumnReorder && ix.dropColumnId === column.id && ix.dragColumnId !== column.id && "bg-primary/10",
        className,
      )}
      onDragOver={
        enableColumnReorder && ix.dragColumnId
          ? (e) => {
              e.preventDefault()
              ix.setDropColumnId(column.id)
            }
          : undefined
      }
      onDrop={
        enableColumnReorder && ix.dragColumnId
          ? (e) => {
              e.preventDefault()
              ix.moveColumn(ix.dragColumnId!, column.id)
              ix.setDragColumnId(null)
              ix.setDropColumnId(null)
            }
          : undefined
      }
      style={resized ? { width: header.getSize(), minWidth: header.getSize(), maxWidth: header.getSize() } : undefined}
    >
      {header.isPlaceholder ? null : canSort || reorderable || hint ? (
        <HeaderHint hint={hint}>
          <button
            type="button"
            onClick={canSort ? column.getToggleSortingHandler() : undefined}
            draggable={reorderable}
            onDragStart={(e) => {
              e.dataTransfer.effectAllowed = "move"
              ix.setDragColumnId(column.id)
            }}
            onDragEnd={() => {
              ix.setDragColumnId(null)
              ix.setDropColumnId(null)
            }}
            title={reorderable && !hint ? t("dataTable.reorderColumnAria") : undefined}
            className={cn(
              "inline-flex max-w-full items-center gap-1 truncate",
              canSort && "hover:text-foreground",
              reorderable && "cursor-grab active:cursor-grabbing",
              ix.dragColumnId === column.id && "opacity-50",
            )}
          >
            {reorderable && <GripVerticalIcon className="-ml-1.5 size-3 shrink-0 opacity-40" aria-hidden />}
            <table.FlexRender header={header} />
            {hint && <CircleHelpIcon className="size-3 shrink-0 opacity-60" />}
            {canSort &&
              (sortDir === "asc" ? (
                <ArrowUpIcon className="size-3 shrink-0" />
              ) : sortDir === "desc" ? (
                <ArrowDownIcon className="size-3 shrink-0" />
              ) : (
                <ArrowUpDownIcon className="size-3 shrink-0 opacity-40" />
              ))}
          </button>
        </HeaderHint>
      ) : (
        <table.FlexRender header={header} />
      )}
      {column.getCanResize() && (
        <div
          onMouseDown={(e) => ix.startColumnResize(e, column.id)}
          onTouchStart={(e) => ix.startColumnResize(e, column.id)}
          onClick={(e) => e.stopPropagation()}
          role="separator"
          aria-orientation="vertical"
          aria-label={t("dataTable.resizeColumnAria")}
          title={t("dataTable.resizeColumnAria")}
          className={cn(
            "group/resize absolute right-0 top-0 h-full w-1.5 -translate-x-1/2 cursor-col-resize touch-none select-none after:absolute after:inset-y-2 after:left-1/2 after:w-px after:-translate-x-1/2 after:bg-border after:content-[''] hover:bg-primary/50 hover:after:bg-transparent",
            ix.resizingColumnId === column.id && "bg-primary",
          )}
        />
      )}
    </th>
  )
}

/** The Columns menu: toggles every hideable column. */
export function ColumnsMenu<TData extends RowData>({ table }: { table: FloxTable<TData> }) {
  const { t } = useTranslation("common")
  return (
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
  )
}
