/**
 * The analytics API's contract types (apps/internal/analytics): the
 * dimensions a report groups by, the metrics it returns, and the rows the
 * client maps its responses to. The catalog of labels and formats built on
 * them is `@/lib/analytics-registry`.
 */

/** Mirrors chstore.ReportDimension (+ "day"). */
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
  | "telegram_channel"
  | "buyer"
  | "day";

/** Push metrics — a separate data source (§30.5): one formula never mixes
 * them with the rest. The custom-metric catalog's ids, the same on Push >
 * Statistics. */
export type PushMetricKey = "push_sent" | "push_delivered" | "push_opened" | "push_unsubscribed" | "push_ctr";

/** Mirrors analytics.MetricValues' JSON keys. */
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
  | "tgStarts"
  | "tgJoins"
  | "registrations"
  | "purchases"
  | "repurchases"
  | "declines"
  | "trash"
  | "clickToInstall"
  | "installToReg"
  | "regToPurchase"
  | "costPerInstall"
  | "costPerReg"
  | "costPerPurchase"
  | "avgOrderValue"
  | "blocked"
  | "bots"
  | "blockedRate"
  | "botRate"
  | "emptySubs"
  | "emptySubsRate"
  | PushMetricKey;

export type MetricFormat = "int" | "currency" | "percent" | "ratio";

export type FilterCondition = { dimension: DimensionKey; value: string };

/** One row of a report, timeseries or dashboard response, as
 * `@/lib/api/analytics` maps it. */
export type ReportRow = {
  key: string;
  dims: Partial<Record<DimensionKey, string>>;
  metrics: Record<MetricKey, number | null>;
  /** Dated by click: the row's conversions with no attributed click, which
   * keep their own date (absent otherwise). */
  datedByConversion?: number;
};
