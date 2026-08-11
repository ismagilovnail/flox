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
  | "landing"
  | "pwa"
  | "postlanding"
  | "offer"
  | "network";

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
  { key: "landing", label: "Landing" },
  { key: "pwa", label: "PWA" },
  { key: "postlanding", label: "Postlanding" },
  { key: "offer", label: "Offer" },
  { key: "network", label: "Network" },
];

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
  | "epc";

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
];

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
