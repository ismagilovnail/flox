"use client";

import * as React from "react";
import ReactECharts from "echarts-for-react";
import { useTheme } from "next-themes";
import { useTranslation } from "react-i18next";

import { ChartCard } from "@/components/ui/chart-card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { chartBaseOption, chartBarOption, CHART_COLORS, type ChartMode } from "@/lib/chart-theme";
import type { ReportRow } from "@/features/analytics/aggregate";
import type { TimeseriesPoint } from "@/lib/api/analytics";
import { METRICS, metricLabel, formatMetric, type DimensionKey, type MetricKey } from "@/features/analytics/registry";

function MetricPicker({
  metrics,
  value,
  onChange,
}: {
  metrics: MetricKey[];
  value: MetricKey;
  onChange: (m: MetricKey) => void;
}) {
  const { t } = useTranslation("analytics");
  return (
    <Select value={value} onValueChange={(v) => onChange(v as MetricKey)}>
      <SelectTrigger size="sm" className="w-36">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {metrics.map((m) => (
          <SelectItem key={m} value={m}>
            {metricLabel(t, m)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export function ReportLineChart({
  points: series,
  metrics,
}: {
  /** One row per day, every metric included — the client picks which field
   * to plot, matching TimeseriesInput's own "no second request" contract. */
  points: TimeseriesPoint[];
  metrics: MetricKey[];
}) {
  const { t } = useTranslation("analytics");
  const { resolvedTheme } = useTheme();
  const mode: ChartMode = resolvedTheme === "light" ? "light" : "dark";
  const [metric, setMetric] = React.useState<MetricKey>(metrics[0] ?? "clicks");
  const activeMetric = metrics.includes(metric) ? metric : metrics[0];

  const points = React.useMemo(
    () =>
      [...series]
        .sort((a, b) => a.day.localeCompare(b.day))
        .map((p) => ({ date: p.day, value: p.metrics[activeMetric] })),
    [series, activeMetric],
  );
  const meta = METRICS.find((m) => m.key === activeMetric)!;

  const option = React.useMemo(() => {
    const base = chartBaseOption(mode);
    return {
      // NB: no `animation:` key here on purpose — see the identical note
      // in components/charts/line-metric-chart.tsx and the full
      // explanation in lib/chart-theme.ts.
      ...base,
      tooltip: { ...base.tooltip, valueFormatter: (v: number) => formatMetric(v, meta.format) },
      series: [
        {
          type: "line",
          data: points.map((p) => [p.date, p.value]),
          smooth: 0.2,
          symbol: "none",
          lineStyle: { color: CHART_COLORS.primary, width: 2 },
          areaStyle: { color: "oklch(0.68 0.16 255 / 0.15)" },
        },
      ],
    };
  }, [mode, points, meta]);

  return (
    <ChartCard
      title={t("charts.trend")}
      action={<MetricPicker metrics={metrics} value={activeMetric} onChange={setMetric} />}
    >
      {/* key={mode}: remount on theme change instead of letting zrender
          animate the transition — see line-metric-chart.tsx's comment on
          the interpolate1DArray crash this avoids. */}
      <ReactECharts key={mode} option={option} notMerge style={{ height: "100%", width: "100%" }} opts={{ renderer: "svg" }} />
    </ChartCard>
  );
}

export function ReportBarChart({
  rows,
  dimension,
  metrics,
}: {
  /** Every group for `dimension` (no top-N/sort baked in) — the client
   * sorts and slices to top-8 by whichever metric is currently selected,
   * the same behavior the removed client-side aggregateByDimension gave. */
  rows: ReportRow[];
  dimension: DimensionKey;
  metrics: MetricKey[];
}) {
  const { t } = useTranslation("analytics");
  const { resolvedTheme } = useTheme();
  const mode: ChartMode = resolvedTheme === "light" ? "light" : "dark";
  const [metric, setMetric] = React.useState<MetricKey>(metrics[0] ?? "clicks");
  const activeMetric = metrics.includes(metric) ? metric : metrics[0];

  const bars = React.useMemo(
    () =>
      rows
        .map((r) => ({ name: r.dims[dimension] ?? "", value: r.metrics[activeMetric] }))
        .sort((a, b) => (b.value ?? -Infinity) - (a.value ?? -Infinity))
        .slice(0, 8),
    [rows, dimension, activeMetric],
  );
  const meta = METRICS.find((m) => m.key === activeMetric)!;

  const option = React.useMemo(() => {
    const categories = [...bars].reverse().map((b) => b.name);
    const base = chartBarOption(mode, categories);
    return {
      ...base,
      tooltip: { ...base.tooltip, trigger: "item" as const, valueFormatter: (v: number) => formatMetric(v, meta.format) },
      series: [
        {
          type: "bar",
          data: [...bars].reverse().map((b) => b.value ?? 0),
          itemStyle: { color: CHART_COLORS.primary, borderRadius: [0, 3, 3, 0] },
          barMaxWidth: 22,
        },
      ],
    };
  }, [mode, bars, meta]);

  return (
    <ChartCard
      title={t("charts.breakdown")}
      action={<MetricPicker metrics={metrics} value={activeMetric} onChange={setMetric} />}
    >
      {/* key={mode}: remount on theme change instead of letting zrender
          animate the transition — see line-metric-chart.tsx's comment on
          the interpolate1DArray crash this avoids. */}
      <ReactECharts key={mode} option={option} notMerge style={{ height: "100%", width: "100%" }} opts={{ renderer: "svg" }} />
    </ChartCard>
  );
}
