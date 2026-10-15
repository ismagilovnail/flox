/**
 * The report catalog: every dimension and metric the analytics API answers,
 * their labels and formats — shared by the report builder, the dashboard,
 * custom metrics and report presets. The types are `@/types/analytics`.
 * apps/internal/custommetrics' registry_sync_test.go reads METRICS from this
 * file to keep the Go catalog in step.
 */
import type { TFunction } from "i18next";

import { formatMoney, formatInt } from "@/lib/format";
import type { DimensionKey, MetricFormat, MetricKey, PushMetricKey } from "@/types/analytics";


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

/** What the report builder's dimension picker offers: every dimension
 * chstore.ReportDimension answers, plus "day" (per-day rows, bucketed in the
 * report's timezone). */
export const REPORT_DIMENSIONS = DIMENSIONS;

/** What the filter picker offers: like REPORT_DIMENSIONS without "day" — a day
 * is a bucket of event time, not a value an event carries. */
export const FILTER_DIMENSIONS = REPORT_DIMENSIONS.filter((d) => d.key !== "day");

/** `.label` above stays English (relied on as-is by the not-yet-localized
 * Custom Metrics feature) — the report builder's own screens call these
 * instead, which go through the `analytics` namespace. */
export function dimensionLabel(t: TFunction, key: DimensionKey): string {
  return t(`dimensions.${key}`, { ns: "analytics" });
}


/** Push metrics — a separate data source (§30.5): one formula never mixes
 * them with the rest. They keep the custom-metric catalog's ids, the same
 * on Push > Statistics, and are filled only when the report is grouped and
 * filtered by day, campaign, country or PWA (else "—"). */
export const PUSH_METRIC_KEYS = [
  "push_sent",
  "push_delivered",
  "push_opened",
  "push_unsubscribed",
  "push_ctr",
] as const satisfies readonly PushMetricKey[];

// Every PushMetricKey is listed above (a new one added to the type alone
// fails to compile here).
const allPushMetricsListed: Exclude<PushMetricKey, (typeof PUSH_METRIC_KEYS)[number]> extends never ? true : false = true;
void allPushMetricsListed;


export function isPushMetric(key: string): key is PushMetricKey {
  return (PUSH_METRIC_KEYS as readonly string[]).includes(key);
}


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
  { key: "declines", label: "Declines", format: "int" },
  { key: "trash", label: "Trash", format: "int" },
  { key: "clickToInstall", label: "Click → install", format: "percent" },
  { key: "installToReg", label: "Install → reg.", format: "percent" },
  { key: "regToPurchase", label: "Reg. → purchase", format: "percent" },
  { key: "costPerInstall", label: "Cost per install", format: "currency" },
  { key: "costPerReg", label: "Cost per registration", format: "currency" },
  { key: "costPerPurchase", label: "Cost per purchase", format: "currency" },
  { key: "blocked", label: "Blocked", format: "int" },
  { key: "bots", label: "Bots", format: "int" },
  { key: "blockedRate", label: "Blocked rate", format: "percent" },
  { key: "botRate", label: "Bot rate", format: "percent" },
  { key: "emptySubs", label: "Empty subs", format: "int" },
  { key: "emptySubsRate", label: "Empty subs %", format: "percent" },
  { key: "push_sent", label: "Push sent", format: "int" },
  { key: "push_delivered", label: "Push delivered", format: "int" },
  { key: "push_opened", label: "Push opened", format: "int" },
  { key: "push_unsubscribed", label: "Push unsubscribed", format: "int" },
  { key: "push_ctr", label: "Push CTR", format: "percent" },
];

export function metricLabel(t: TFunction, key: MetricKey): string {
  return t(`metrics.${key}`, { ns: "analytics" });
}


/** `currency` is the reporting currency the value is in (useReportingCurrency). */
export function formatMetric(value: number | null, format: MetricFormat, currency = "USD"): string {
  if (value === null) return "—";
  switch (format) {
    case "int":
      return formatInt(value);
    case "currency":
      return formatMoney(value, currency, 2);
    case "percent":
      return `${value.toFixed(2)}%`;
    case "ratio":
      return `${value.toFixed(2)}x`;
  }
}
