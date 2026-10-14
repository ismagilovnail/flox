import type { TFunction } from "i18next";

import { formatInt, formatPercent } from "@/lib/format";
import type { Locale } from "@/lib/i18n/config";
import type { FlowStageCounts } from "@/lib/api/analytics";

/** Days the flow builder's node summaries cover, today included. */
export const FLOW_SUMMARY_DAYS = 7;

export type FlowNodeSummaries = {
  landing: string;
  pwa: string;
  postlanding: string;
  destination: string;
};

/** One line per §25 node from a flow's stage counts. A rate over zero
 * views is "—", never 0%. */
export function flowNodeSummaries(counts: FlowStageCounts, t: TFunction, locale: Locale): FlowNodeSummaries {
  const n = (type: string) => counts[type] ?? 0;
  const int = (v: number) => formatInt(v, locale);
  const rate = (part: number, whole: number) => (whole > 0 ? formatPercent((part / whole) * 100, locale) : "—");

  return {
    landing: t("flowSummary.viewsClicks", {
      views: int(n("LAND_VIEW")),
      clicks: int(n("LAND_CLICK")),
      rate: rate(n("LAND_CLICK"), n("LAND_VIEW")),
    }),
    pwa: t("flowSummary.viewsInstalls", {
      views: int(n("PWA_VIEW")),
      installs: int(n("PWA_INSTALL")),
      rate: rate(n("PWA_INSTALL"), n("PWA_VIEW")),
    }),
    postlanding: t("flowSummary.viewsClicks", {
      views: int(n("POSTLANDING_VIEW")),
      clicks: int(n("POSTLANDING_CLICK")),
      rate: rate(n("POSTLANDING_CLICK"), n("POSTLANDING_VIEW")),
    }),
    destination: t("flowSummary.clicksConversions", {
      clicks: int(n("SOURCE_CLICK")),
      conversions: int(n("CPA_ACCEPT")),
      redeps: int(n("CPA_REDEP")),
    }),
  };
}
