"use client";

import * as React from "react";
import type { DateRange } from "react-day-picker";

import type { GridPage } from "@/components/ui/data-grid";
import { todayInZone } from "@/lib/timezones";

/** The logs' days are UTC days (their APIs cut [from, to] in UTC). */
const LOG_TIMEZONE = "UTC";
/** The APIs' own default window when none is given. */
const DEFAULT_DAYS = 7;

export function defaultLogRange(now: Date = new Date()): { from: Date; to: Date } {
  const to = todayInZone(LOG_TIMEZONE, now);
  const from = new Date(to);
  from.setUTCDate(from.getUTCDate() - (DEFAULT_DAYS - 1));
  return { from, to };
}

const ymd = (d: Date) => d.toISOString().slice(0, 10);

/**
 * The state of a server-paged log (click log, postback logs, conversions):
 * a date range, an exact click id, and the page. Changing a filter starts
 * over at the first page. `filter` is what the API takes besides paging;
 * `params` adds the page.
 */
export function useLogGrid(defaultLimit = 50) {
  const [range, setRangeState] = React.useState<{ from: Date; to: Date }>(() => defaultLogRange());
  const [clickId, setClickIdState] = React.useState("");
  const [page, setPage] = React.useState<GridPage>({ offset: 0, limit: defaultLimit });

  const setRange = (next: DateRange | undefined) => {
    if (!next?.from) return; // the picker cleared mid-selection: keep the last full range
    setRangeState({ from: next.from, to: next.to ?? next.from });
    setPage((p) => ({ ...p, offset: 0 }));
  };
  const setClickId = (next: string) => {
    setClickIdState(next.trim());
    setPage((p) => ({ ...p, offset: 0 }));
  };

  const filter = React.useMemo(
    () => ({ from: ymd(range.from), to: ymd(range.to), clickId: clickId || undefined }),
    [range, clickId],
  );
  const params = React.useMemo(() => ({ ...filter, limit: page.limit, offset: page.offset }), [filter, page]);
  return { range, setRange, clickId, setClickId, page, setPage, filter, params, timezone: LOG_TIMEZONE };
}
