/**
 * Report Presets (§27.5) — a saved, reusable {columns, metrics, grouping,
 * period, timezone}. Real backend: apps/internal/reportpresets. `period` is
 * stored relative ("last N days") wherever possible rather than a frozen
 * from/to pair, so applying a preset next month still means "the last 30
 * days," not a stale historical window — only a custom range that doesn't
 * match a known relative window falls back to fixed dates.
 *
 * Moved out of lib/mock/ — this is real, permanent vocabulary (the type
 * shape and the period<->date-range conversions are used against real API
 * responses now), not disposable mock data. Same move already made for
 * lib/tags.ts, lib/custom-metrics.ts, lib/organization.ts.
 */

import type { DimensionKey, MetricKey, FilterCondition } from "@/types/analytics";
import type { DateBasis } from "@/lib/api/analytics";

export type ReportPeriod = { type: "relative"; days: number } | { type: "custom"; from: string; to: string };

export const RELATIVE_PERIODS: { days: number; label: string }[] = [
  { days: 1, label: "Today" },
  { days: 7, label: "Last 7 days" },
  { days: 30, label: "Last 30 days" },
  { days: 90, label: "Last 90 days" },
];

export function periodLabel(period: ReportPeriod): string {
  if (period.type === "relative") {
    return RELATIVE_PERIODS.find((p) => p.days === period.days)?.label ?? `Last ${period.days} days`;
  }
  return `${period.from} → ${period.to}`;
}

/** Re-anchors a relative period to `today` — the whole point of storing it relative. */
export function periodToDateRange(period: ReportPeriod, today: Date): { from: Date; to: Date } {
  if (period.type === "custom") {
    return { from: new Date(`${period.from}T00:00:00Z`), to: new Date(`${period.to}T00:00:00Z`) };
  }
  const to = new Date(today);
  const from = new Date(today);
  from.setUTCDate(from.getUTCDate() - (period.days - 1));
  return { from, to };
}

/** Inverse of periodToDateRange — used when saving the current report state as a
 * preset: if the current range matches a known "last N days ending today" window,
 * store it relative; otherwise fall back to a fixed custom range. */
export function dateRangeToPeriod(range: { from?: Date; to?: Date } | undefined, today: Date): ReportPeriod {
  const from = range?.from;
  const to = range?.to ?? from;
  if (!from || !to) return { type: "relative", days: 30 };

  const isToday = to.toDateString() === today.toDateString();
  if (isToday) {
    const days = Math.round((today.getTime() - from.getTime()) / (1000 * 60 * 60 * 24)) + 1;
    if (RELATIVE_PERIODS.some((p) => p.days === days)) {
      return { type: "relative", days };
    }
  }
  return { type: "custom", from: from.toISOString().slice(0, 10), to: to.toISOString().slice(0, 10) };
}

export type ReportPreset = {
  id: string;
  organizationId?: string;
  name: string;
  dimensions: DimensionKey[];
  metrics: MetricKey[];
  /** Saved "dimension = value" conditions, AND-combined; applying the preset sets them. */
  filters: FilterCondition[];
  /** Dragged table column order (ids); [] is the natural order. */
  columnOrder: string[];
  /** Dragged table column widths in px by column id; {} is natural widths. */
  columnWidths: Record<string, number>;
  /** Ids of the columns hidden in the table; [] shows every column. */
  hiddenColumns: string[];
  groupBy: DimensionKey;
  period: ReportPeriod;
  timezone: string;
  /** How the report dates conversions (tech audit AN5). */
  dateBasis: DateBasis;
  /** System default — visible to everyone, can't be renamed or deleted.
   * Real backend lazily seeds this on an org's first GET /report-presets —
   * see apps/internal/reportpresets/repository.go's EnsureDefault. */
  isDefault: boolean;
  createdByUserId?: string | null;
  createdAt: string;
  updatedAt: string;
};

export type ReportPresetInput = {
  name: string;
  dimensions: DimensionKey[];
  metrics: MetricKey[];
  filters: FilterCondition[];
  columnOrder: string[];
  columnWidths: Record<string, number>;
  hiddenColumns: string[];
  groupBy: DimensionKey;
  period: ReportPeriod;
  timezone: string;
  dateBasis: DateBasis;
};
