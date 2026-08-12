/**
 * Report Presets (§27.5) — a saved, reusable {columns, metrics, grouping,
 * period, timezone}. `period` is stored relative ("last N days") wherever
 * possible rather than a frozen from/to pair, so applying a preset next
 * month still means "the last 30 days," not a stale historical window —
 * only a custom range that doesn't match a known relative window falls
 * back to fixed dates.
 */

import type { DimensionKey, MetricKey } from "@/features/analytics/registry";

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
  name: string;
  dimensions: DimensionKey[];
  metrics: MetricKey[];
  groupBy: DimensionKey;
  period: ReportPeriod;
  timezone: string;
  /** System default — visible to everyone, can't be renamed or deleted. */
  isDefault: boolean;
  createdAt: string;
  updatedAt: string;
};

export const REPORT_PRESETS: ReportPreset[] = [
  {
    id: "preset_default_overview",
    name: "Campaign Overview (default)",
    dimensions: ["campaign"],
    metrics: ["clicks", "conversions", "revenue", "cvr"],
    groupBy: "campaign",
    period: { type: "relative", days: 30 },
    timezone: "UTC",
    isDefault: true,
    createdAt: "2026-01-20T00:00:00Z",
    updatedAt: "2026-01-20T00:00:00Z",
  },
  {
    id: "preset_source_performance",
    name: "Source Performance — 7d",
    dimensions: ["source", "country"],
    metrics: ["clicks", "revenue", "roi", "cpa"],
    groupBy: "source",
    period: { type: "relative", days: 7 },
    timezone: "UTC",
    isDefault: false,
    createdAt: "2026-05-10T00:00:00Z",
    updatedAt: "2026-05-10T00:00:00Z",
  },
];
