import type { TFunction } from "i18next";

import type { MetricCategory, ShowInTarget } from "@/lib/custom-metrics-registry";

const CATEGORY_KEY: Record<MetricCategory, string> = {
  "Traffic / Performance": "traffic",
  "CPA Funnel": "cpaFunnel",
  Push: "push",
  Fraud: "fraud",
  LTV: "ltv",
};

/** A catalog metric's name in the interface language: the custom-metrics
 * catalog's own translation, else the analytics registry's (the traffic
 * metrics are the same ones), else the registry's English label. */
export function metricLabel(t: TFunction, metric: { id: string; label: string }): string {
  return t(`customMetrics:catalog.${metric.id}`, {
    defaultValue: t(`analytics:metrics.${metric.id}`, { defaultValue: metric.label }),
  });
}

export function categoryLabel(t: TFunction, category: MetricCategory): string {
  return t(`customMetrics:categories.${CATEGORY_KEY[category]}`);
}

export function targetLabel(t: TFunction, target: ShowInTarget): string {
  return t(`customMetrics:targets.${target}`);
}
