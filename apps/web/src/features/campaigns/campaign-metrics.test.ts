import { describe, expect, it } from "vitest";

import type { Campaign } from "@/lib/api/campaigns";
import type { ReportRow } from "@/features/analytics/aggregate";
import { SURFACE_AVAILABLE_METRIC_IDS } from "@/lib/custom-metrics-registry";
import { evaluateFormula } from "@/lib/formula-engine";
import type { CustomMetric } from "@/lib/custom-metrics";
import {
  CAMPAIGN_TABLE_METRICS,
  campaignTableCustomMetrics,
  withCampaignMetrics,
} from "@/features/campaigns/campaign-metrics";

const campaign = (id: string) => ({ id, name: id }) as Campaign;

function row(campaignId: string, metrics: Partial<ReportRow["metrics"]>): ReportRow {
  return { key: campaignId, dims: { campaign: campaignId }, metrics: metrics as ReportRow["metrics"] };
}

describe("withCampaignMetrics", () => {
  it("leaves rows without metrics while the report is not loaded", () => {
    expect(withCampaignMetrics([campaign("a")], undefined)[0].metrics).toBeUndefined();
  });

  it("joins report rows by campaign id", () => {
    const [a] = withCampaignMetrics(
      [campaign("a")],
      [row("a", { clicks: 120, conversions: 4, revenue: 200, cost: 50, profit: 150, roi: 300, ctr: 1 })],
    );
    expect(a.metrics).toEqual({ clicks: 120, conversions: 4, revenue: 200, cost: 50, profit: 150, roi: 300 });
  });

  it("keeps a missing cost as null, never zero", () => {
    const [a] = withCampaignMetrics([campaign("a")], [row("a", { clicks: 10, conversions: 1, revenue: 5, cost: null, profit: null, roi: null })]);
    expect(a.metrics?.cost).toBeNull();
    expect(a.metrics?.roi).toBeNull();
  });

  it("gives a campaign with no row zero traffic and no cost", () => {
    const [b] = withCampaignMetrics([campaign("b")], [row("a", { clicks: 1 })]);
    expect(b.metrics).toEqual({ clicks: 0, conversions: 0, revenue: 0, cost: null, profit: null, roi: null });
  });
});

describe("campaign table custom metrics", () => {
  it("shows the same metrics custom metrics targeting the table may use", () => {
    const surface = SURFACE_AVAILABLE_METRIC_IDS.campaigns_table;
    expect([...surface].sort()).toEqual([...CAMPAIGN_TABLE_METRICS].sort());
  });

  it("takes published, active metrics targeting the campaigns table", () => {
    const base = { status: "published", active: true, targets: ["campaigns_table"] } as unknown as CustomMetric;
    const list = [
      { ...base, id: "1" },
      { ...base, id: "2", status: "draft" },
      { ...base, id: "3", active: false },
      { ...base, id: "4", targets: ["report_builder"] },
    ] as CustomMetric[];
    expect(campaignTableCustomMetrics(list).map((m) => m.id)).toEqual(["1"]);
  });

  it("evaluates a formula on a row, empty without spend", () => {
    const [a, b] = withCampaignMetrics(
      [campaign("a"), campaign("b")],
      [row("a", { clicks: 100, conversions: 2, revenue: 80, cost: 40, profit: 40, roi: 100 })],
    );
    expect(evaluateFormula("{revenue} / {cost}", a.metrics!)).toBe(2);
    expect(evaluateFormula("{revenue} / {cost}", b.metrics!)).toBeNull();
  });
});
