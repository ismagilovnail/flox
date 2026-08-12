import type { MetricFormat } from "@/features/analytics/registry";
import type { ShowInTarget } from "@/lib/mock/custom-metrics-registry";

export type CustomMetricStatus = "draft" | "published";

export type CustomMetric = {
  id: string;
  name: string;
  group: string;
  formula: string;
  format: MetricFormat;
  status: CustomMetricStatus;
  active: boolean;
  targets: ShowInTarget[];
  createdByMemberId: string;
  createdAt: string;
  updatedAt: string;
};

export const CUSTOM_METRICS: CustomMetric[] = [
  {
    id: "cm_margin_per_click",
    name: "Margin per Click",
    group: "Profitability",
    formula: "({revenue} - {cost}) / {clicks}",
    format: "currency",
    status: "published",
    active: true,
    targets: ["report_builder", "campaigns_table"],
    createdByMemberId: "mem_owner",
    createdAt: "2026-06-01T00:00:00Z",
    updatedAt: "2026-06-01T00:00:00Z",
  },
  {
    // The spec's own §30.5 example formula, kept as a Draft: {bots}/{click_all}
    // are catalog-only (no tracker/bot-classifier exists yet — Phase 20), so
    // this is exactly what "drafts are invisible in reports until published"
    // is for — defined, valid, not yet backed by live data anywhere.
    id: "cm_bot_share",
    name: "Bot Share",
    group: "Fraud",
    formula: "{bots} / ({bots} + {click_all})",
    format: "percent",
    status: "draft",
    active: false,
    targets: [],
    createdByMemberId: "mem_owner",
    createdAt: "2026-07-15T00:00:00Z",
    updatedAt: "2026-07-15T00:00:00Z",
  },
];
