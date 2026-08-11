"use client";

import * as React from "react";
import ReactECharts from "echarts-for-react";
import { useTheme } from "next-themes";
import type { DateRange } from "react-day-picker";

import { ChartCard } from "@/components/ui/chart-card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { chartBaseOption, chartBarOption, CHART_COLORS, type ChartMode } from "@/lib/chart-theme";
import { aggregateByDimension, aggregateTimeSeries, type FilterCondition } from "@/features/analytics/aggregate";
import { METRICS, formatMetric, type DimensionKey, type MetricKey } from "@/features/analytics/registry";
import type { RawSlice } from "@/lib/mock/analytics";

function MetricPicker({
  metrics,
  value,
  onChange,
}: {
  metrics: MetricKey[];
  value: MetricKey;
  onChange: (m: MetricKey) => void;
}) {
  return (
    <Select value={value} onValueChange={(v) => onChange(v as MetricKey)}>
      <SelectTrigger size="sm" className="w-36">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {metrics.map((m) => (
          <SelectItem key={m} value={m}>
            {METRICS.find((x) => x.key === m)?.label ?? m}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export function ReportLineChart({
  slices,
  dateRange,
  filters,
  metrics,
}: {
  slices: RawSlice[];
  dateRange?: DateRange;
  filters: FilterCondition[];
  metrics: MetricKey[];
}) {
  const { resolvedTheme } = useTheme();
  const mode: ChartMode = resolvedTheme === "light" ? "light" : "dark";
  const [metric, setMetric] = React.useState<MetricKey>(metrics[0] ?? "clicks");
  const activeMetric = metrics.includes(metric) ? metric : metrics[0];

  const points = React.useMemo(
    () => aggregateTimeSeries({ slices, dateRange, filters, metric: activeMetric }),
    [slices, dateRange, filters, activeMetric],
  );
  const meta = METRICS.find((m) => m.key === activeMetric)!;

  const option = React.useMemo(() => {
    const base = chartBaseOption(mode);
    return {
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
      title="Trend"
      action={<MetricPicker metrics={metrics} value={activeMetric} onChange={setMetric} />}
    >
      <ReactECharts option={option} notMerge style={{ height: "100%", width: "100%" }} opts={{ renderer: "svg" }} />
    </ChartCard>
  );
}

export function ReportBarChart({
  slices,
  dateRange,
  filters,
  dimension,
  metrics,
}: {
  slices: RawSlice[];
  dateRange?: DateRange;
  filters: FilterCondition[];
  dimension: DimensionKey;
  metrics: MetricKey[];
}) {
  const { resolvedTheme } = useTheme();
  const mode: ChartMode = resolvedTheme === "light" ? "light" : "dark";
  const [metric, setMetric] = React.useState<MetricKey>(metrics[0] ?? "clicks");
  const activeMetric = metrics.includes(metric) ? metric : metrics[0];

  const bars = React.useMemo(
    () => aggregateByDimension({ slices, dateRange, filters, dimension, metric: activeMetric }),
    [slices, dateRange, filters, dimension, activeMetric],
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
      title="Breakdown"
      action={<MetricPicker metrics={metrics} value={activeMetric} onChange={setMetric} />}
    >
      <ReactECharts option={option} notMerge style={{ height: "100%", width: "100%" }} opts={{ renderer: "svg" }} />
    </ChartCard>
  );
}
