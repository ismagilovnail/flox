import type { TFunction } from "i18next";

import { formatUsd, formatInt } from "@/lib/format";

export type DimensionKey =
  | "campaign"
  | "source"
  | "country"
  | "region"
  | "city"
  | "device"
  | "platform"
  | "os"
  | "browser"
  | "language"
  | "flow"
  | "stream_set"
  | "landing"
  | "pwa"
  | "postlanding"
  | "offer"
  | "network"
  | "day";

/** Mirrors maxReportDims in apps/internal/analytics/report.go — the API rejects more. */
export const MAX_REPORT_DIMENSIONS = 4;

export const DIMENSIONS: { key: DimensionKey; label: string }[] = [
  { key: "campaign", label: "Campaign" },
  { key: "source", label: "Source" },
  { key: "country", label: "Country" },
  { key: "region", label: "Region" },
  { key: "city", label: "City" },
  { key: "device", label: "Device" },
  { key: "platform", label: "Platform" },
  { key: "os", label: "OS" },
  { key: "browser", label: "Browser" },
  { key: "language", label: "Language" },
  { key: "flow", label: "Flow" },
  { key: "stream_set", label: "Stream Set" },
  { key: "landing", label: "Landing" },
  { key: "pwa", label: "PWA" },
  { key: "postlanding", label: "Postlanding" },
  { key: "offer", label: "Offer" },
  { key: "network", label: "Network" },
  { key: "day", label: "Day" },
];

/** Dimensions /analytics/report can't answer yet (no event column — see
 * chstore.ReportDimension): picking one 400s the whole report, so the report
 * builder never offers them. `registry.ts` keeps the full list for Custom Metrics. */
const UNBACKED_DIMENSIONS: DimensionKey[] = ["postlanding", "network"];

/** What the report builder's dimension picker offers: every backed dimension,
 * plus "day" (per-day rows, bucketed in the report's timezone). */
export const REPORT_DIMENSIONS = DIMENSIONS.filter((d) => !UNBACKED_DIMENSIONS.includes(d.key));

/** What the filter picker offers: like REPORT_DIMENSIONS without "day" — a day
 * is a bucket of event time, not a value an event carries. */
export const FILTER_DIMENSIONS = REPORT_DIMENSIONS.filter((d) => d.key !== "day");

/** `.label` above stays English (relied on as-is by the not-yet-localized
 * Custom Metrics feature) — the report builder's own screens call these
 * instead, which go through the `analytics` namespace. */
export function dimensionLabel(t: TFunction, key: DimensionKey): string {
  return t(`dimensions.${key}`, { ns: "analytics" });
}

export type MetricKey =
  | "clicks"
  | "uniqueClicks"
  | "conversions"
  | "revenue"
  | "cost"
  | "profit"
  | "roi"
  | "roas"
  | "ctr"
  | "cvr"
  | "cpc"
  | "cpa"
  | "epc"
  | "pwaInstalls"
  | "registrations"
  | "purchases"
  | "repurchases"
  | "clickToInstall"
  | "installToReg"
  | "regToPurchase"
  | "costPerInstall"
  | "costPerReg"
  | "costPerPurchase";

export type MetricFormat = "int" | "currency" | "percent" | "ratio";

/** Formulas match §50 (METRICS REGISTRY) — never recompute these ad hoc elsewhere. */
export const METRICS: { key: MetricKey; label: string; format: MetricFormat }[] = [
  { key: "clicks", label: "Clicks", format: "int" },
  { key: "uniqueClicks", label: "Unique Clicks", format: "int" },
  { key: "conversions", label: "Conversions", format: "int" },
  { key: "revenue", label: "Revenue", format: "currency" },
  { key: "cost", label: "Cost", format: "currency" },
  { key: "profit", label: "Profit", format: "currency" },
  { key: "roi", label: "ROI", format: "percent" },
  { key: "roas", label: "ROAS", format: "ratio" },
  { key: "ctr", label: "CTR", format: "percent" },
  { key: "cvr", label: "CVR", format: "percent" },
  { key: "cpc", label: "CPC", format: "currency" },
  { key: "cpa", label: "CPA", format: "currency" },
  { key: "epc", label: "EPC", format: "currency" },
  { key: "pwaInstalls", label: "PWA installs", format: "int" },
  { key: "registrations", label: "Registrations", format: "int" },
  { key: "purchases", label: "Purchases", format: "int" },
  { key: "repurchases", label: "Repurchases", format: "int" },
  { key: "clickToInstall", label: "Click → install", format: "percent" },
  { key: "installToReg", label: "Install → reg.", format: "percent" },
  { key: "regToPurchase", label: "Reg. → purchase", format: "percent" },
  { key: "costPerInstall", label: "Cost per install", format: "currency" },
  { key: "costPerReg", label: "Cost per registration", format: "currency" },
  { key: "costPerPurchase", label: "Cost per purchase", format: "currency" },
];

export function metricLabel(t: TFunction, key: MetricKey): string {
  return t(`metrics.${key}`, { ns: "analytics" });
}

export const TIMEZONES = [
  "UTC",
  "America/New_York",
  "America/Los_Angeles",
  "Europe/London",
  "Europe/Berlin",
  "Asia/Dubai",
  "Asia/Singapore",
];

export function formatMetric(value: number | null, format: MetricFormat): string {
  if (value === null) return "—";
  switch (format) {
    case "int":
      return formatInt(value);
    case "currency":
      return formatUsd(value, 2);
    case "percent":
      return `${value.toFixed(2)}%`;
    case "ratio":
      return `${value.toFixed(2)}x`;
  }
}
