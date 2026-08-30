"use client";

import * as React from "react";
import ReactECharts from "echarts-for-react";
import { useTheme } from "next-themes";

import { ChartCard } from "@/components/ui/chart-card";
import { chartBaseOption, type ChartMode } from "@/lib/chart-theme";
import { formatInt } from "@/lib/format";

/** color is a plain "oklch(L C H)" token — inserts an alpha channel for gradient stops. */
function withAlpha(color: string, alpha: number) {
  return color.replace(/\)$/, ` / ${alpha})`);
}

export function LineMetricChart({
  title,
  points,
  color,
  valueFormatter = formatInt,
  action,
}: {
  title: string;
  points: { date: string; value: number }[];
  color: string;
  valueFormatter?: (value: number) => string;
  action?: React.ReactNode;
}) {
  const { resolvedTheme } = useTheme();
  const mode: ChartMode = resolvedTheme === "light" ? "light" : "dark";

  const option = React.useMemo(() => {
    const base = chartBaseOption(mode);
    return {
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
          symbol: "none",
          lineStyle: { color, width: 2 },
          areaStyle: {
            color: {
              type: "linear",
              x: 0,
              y: 0,
              x2: 0,
              y2: 1,
              colorStops: [
                { offset: 0, color: withAlpha(color, 0.25) },
                { offset: 1, color: withAlpha(color, 0) },
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
