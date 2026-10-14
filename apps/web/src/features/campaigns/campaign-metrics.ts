import type { Campaign } from "@/lib/api/campaigns";
import type { ReportRow } from "@/features/analytics/aggregate";
import type { MetricKey } from "@/features/analytics/registry";
import type { CustomMetric } from "@/lib/custom-metrics";

/** The registry metrics the campaigns table shows — the same set custom
 * metrics targeting it may use (§30.5), so a formula never asks for a
 * number the table does not have. */
export const CAMPAIGN_TABLE_METRICS = ["clicks", "conversions", "revenue", "cost", "profit", "roi", "emptySubsRate"] as const satisfies readonly MetricKey[];

export type CampaignTableMetric = (typeof CAMPAIGN_TABLE_METRICS)[number];
export type CampaignMetrics = Record<CampaignTableMetric, number | null>;

/** A campaign row with its period's numbers; `metrics` is undefined while
 * the report is loading or when it failed. */
export type CampaignRow = Campaign & { metrics?: CampaignMetrics };

/** A campaign with no report row had no events and no spend in the
 * period: zero traffic, and no cost — so profit and ROI are "—", never a
 * number computed against a made-up zero spend (CLAUDE.md #6). The empty
 * subs share of no clicks is "—" too. */
const NO_ACTIVITY: CampaignMetrics = { clicks: 0, conversions: 0, revenue: 0, cost: null, profit: null, roi: null, emptySubsRate: null };

/** Joins the report grouped by campaign (`/analytics/report`, dims
 * [campaign], unfiltered — so campaigns with spend but no clicks have a row)
 * onto the campaign list. `report` undefined: not loaded, rows carry no
 * metrics. */
export function withCampaignMetrics(campaigns: Campaign[], report: ReportRow[] | undefined): CampaignRow[] {
  if (!report) return campaigns;
  const byCampaign = new Map<string, CampaignMetrics>();
  for (const row of report) {
    const id = row.dims.campaign;
    if (!id) continue;
    const m = {} as CampaignMetrics;
    for (const key of CAMPAIGN_TABLE_METRICS) m[key] = row.metrics[key] ?? null;
    byCampaign.set(id, m);
  }
  return campaigns.map((c) => ({ ...c, metrics: byCampaign.get(c.id) ?? NO_ACTIVITY }));
}

/** The custom metrics published to the campaigns table and active. */
export function campaignTableCustomMetrics(all: CustomMetric[]): CustomMetric[] {
  return all.filter((m) => m.status === "published" && m.active && m.targets.includes("campaigns_table"));
}
