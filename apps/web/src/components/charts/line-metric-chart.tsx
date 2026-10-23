"use client";

import * as React from "react";
import ReactECharts from "echarts-for-react";
import { useTheme } from "next-themes";

import { ChartCard } from "@/components/ui/chart-card";
import { chartBaseOption, CHART_RGB, rgba, type ChartMode } from "@/lib/chart-theme";
import { formatInt } from "@/lib/format";

export function LineMetricChart({
  title,
  points,
  color,
  valueFormatter = formatInt,
  action,
}: {
  title: string;
  points: { date: string; value: number }[];
  /** A CHART_RGB key, not an oklch token: zrender can't parse oklch, and animates a hovered series' colour (see CHART_RGB). */
  color: keyof typeof CHART_RGB;
  valueFormatter?: (value: number) => string;
  action?: React.ReactNode;
}) {
  const { resolvedTheme } = useTheme();
  const mode: ChartMode = resolvedTheme === "light" ? "light" : "dark";

  const option = React.useMemo(() => {
    const base = chartBaseOption(mode);
    return {
      // NB: do not add an `animation:` key here. This used to carry
      // `animation: points.length > 1` as a narrow guard against the
      // zrender interpolate1DArray crash; once chart-theme.ts turned
      // animation off wholesale (see its comment), that line was the
      // thing turning it back ON for every chart with 2+ points, which
      // is every real one — it kept the crash alive.
      ...base,
      tooltip: {
        ...base.tooltip,
        valueFormatter: (v: number) => valueFormatter(v),
      },
      series: [
        {
          type: "line",
          data: points.map((p) => [p.date, p.value]),
          smooth: 0.2,
          // A single point draws no line: show it as a dot (BUG-7).
          symbol: points.length > 1 ? "none" : "circle",
          symbolSize: 6,
          lineStyle: { color: rgba(CHART_RGB[color], 1), width: 2 },
          itemStyle: { color: rgba(CHART_RGB[color], 1) },
          // No hover animation: it re-tints the line and gradient towards a
          // state colour zrender can't compute, which made them blink.
          emphasis: { disabled: true },
          areaStyle: {
            color: {
              type: "linear",
              x: 0,
              y: 0,
              x2: 0,
              y2: 1,
              colorStops: [
                { offset: 0, color: rgba(CHART_RGB[color], 0.25) },
                { offset: 1, color: rgba(CHART_RGB[color], 0) },
              ],
            },
          },
        },
      ],
    };
  }, [mode, points, color, valueFormatter]);

  return (
    <ChartCard title={title} action={action}>
      {/* key={mode}: without it, toggling theme calls setOption on the
          existing zrender instance, which animates from the old option to
          the new one and throws "Cannot read properties of undefined
          (reading 'length')" inside zrender's interpolate1DArray mid-
          transition (reproduced 2026-08-30). Remounting sidesteps the
          animated-transition code path entirely. */}
      <ReactECharts
        key={mode}
        option={option}
        notMerge
        style={{ height: "100%", width: "100%" }}
        opts={{ renderer: "svg" }}
      />
    </ChartCard>
  );
}
