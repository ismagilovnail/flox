"use client";

import * as React from "react";

import { DataGrid } from "@/components/ui/data-grid";
import { ErrorState } from "@/components/ui/error-state";
import { LoadingState } from "@/components/ui/loading-state";
import { LogGridToolbar } from "@/components/shared/log-grid-toolbar";
import type { useLogGrid } from "@/hooks/use-log-grid";

type GridProps<TData extends Record<string, unknown>> = Omit<
  React.ComponentProps<typeof DataGrid<TData>>,
  "rows" | "total" | "page" | "onPageChange" | "isFetching" | "toolbar" | "fetchPage"
>;

/**
 * A server-paged log page body: its filters (always shown, so a range the
 * API refuses can be changed back), then loading, the error, or the grid.
 * `fetchPage` reads one page with the current filter, for the grid's page
 * export.
 */
export function LogGrid<TData extends Record<string, unknown>>({
  grid,
  query,
  select,
  fetchPage,
  loadingLabel,
  errorTitle,
  errorMessage,
  ...gridProps
}: GridProps<TData> & {
  grid: ReturnType<typeof useLogGrid>;
  query: {
    isPending: boolean;
    isError: boolean;
    isFetching: boolean;
    error: Error | null;
    refetch: () => unknown;
    data?: unknown;
  };
  /** The page's rows and total from the query's data. */
  select: () => { rows: TData[]; total: number } | undefined;
  fetchPage: (filter: ReturnType<typeof useLogGrid>["filter"], offset: number, limit: number) => Promise<{ rows: TData[]; total: number }>;
  loadingLabel: string;
  errorTitle: string;
  errorMessage: (err: Error) => string;
}) {
  const toolbar = (
    <LogGridToolbar
      range={grid.range}
      onRangeChange={grid.setRange}
      clickId={grid.clickId}
      onClickIdChange={grid.setClickId}
      timezone={grid.timezone}
    />
  );
  const { filter } = grid;
  const pageReader = React.useCallback((offset: number, limit: number) => fetchPage(filter, offset, limit), [fetchPage, filter]);
  const page = select();

  if (query.isPending || query.isError || !page) {
    return (
      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center gap-2">{toolbar}</div>
        {query.isError ? (
          <ErrorState title={errorTitle} description={query.error ? errorMessage(query.error) : undefined} onRetry={() => query.refetch()} />
        ) : (
          <LoadingState label={loadingLabel} />
        )}
      </div>
    );
  }

  return (
    <DataGrid<TData>
      {...gridProps}
      rows={page.rows}
      total={page.total}
      page={grid.page}
      onPageChange={grid.setPage}
      isFetching={query.isFetching}
      toolbar={toolbar}
      fetchPage={pageReader}
    />
  );
}
