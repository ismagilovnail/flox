"use client";

import * as React from "react";
import ReactECharts from "echarts-for-react";
import { useTheme } from "next-themes";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Mono, Caption } from "@/components/ui/typography";
import { chartBaseOption, CHART_COLORS, type ChartMode } from "@/lib/chart-theme";
import { formatInt } from "@/lib/format";
import { generateFunnelMock } from "@/lib/mock/analytics";

const STEP_LABELS: Record<string, string> = {
  SOURCE_CLICK: "Source Click",
  LAND_VIEW: "Landing View",
  LAND_CLICK: "Landing Click",
  PWA_VIEW: "PWA View",
  PWA_INSTALL: "PWA Install",
  CPA_HOLD: "CPA Hold",
  CPA_ACCEPT: "CPA Accept",
  CPA_REDEP: "CPA Redeposit",
};

export function FunnelChart() {
  const { resolvedTheme } = useTheme();
  const mode: ChartMode = resolvedTheme === "light" ? "light" : "dark";

  const steps = React.useMemo(() => generateFunnelMock(), []);
  const first = steps[0].value;

  const option = React.useMemo(() => {
    const base = chartBaseOption(mode);
    return {
      textStyle: base.textStyle,
      animationDuration: base.animationDuration,
      tooltip: {
        ...base.tooltip,
        trigger: "item" as const,
        formatter: (p: { name: string; value: number }) =>
          `${p.name}<br/>${formatInt(p.value)} (${((p.value / first) * 100).toFixed(1)}% of ${STEP_LABELS.SOURCE_CLICK})`,
      },
      series: [
        {
          type: "funnel",
          left: "4%",
          width: "92%",
          top: 8,
          bottom: 8,
          min: 0,
          max: first,
          sort: "none" as const,
          gap: 3,
          label: {
            show: true,
            position: "inside" as const,
            color: "#fff",
            fontSize: 11,
            formatter: (p: { name: string }) => STEP_LABELS[p.name] ?? p.name,
          },
          itemStyle: { color: CHART_COLORS.primary, borderColor: "transparent" },
          data: steps.map((s, i) => ({
            name: s.step,
            value: s.value,
            itemStyle: { color: CHART_COLORS.primary.replace(/\)$/, ` / ${100 - i * 8}%)`) },
          })),
        },
      ],
    };
  }, [mode, steps, first]);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Conversion Funnel</CardTitle>
      </CardHeader>
      <CardContent className="grid grid-cols-1 gap-4 lg:grid-cols-[1.2fr_1fr]">
        <div className="h-96">
          {/* key={mode}: remount on theme change instead of letting zrender
              animate the transition — see line-metric-chart.tsx's comment
              on the interpolate1DArray crash this avoids. */}
          <ReactECharts key={mode} option={option} notMerge style={{ height: "100%", width: "100%" }} opts={{ renderer: "svg" }} />
        </div>
        <div className="flex flex-col gap-1.5 self-center">
          {steps.map((s, i) => {
            const prev = i > 0 ? steps[i - 1].value : s.value;
            const stepRate = i > 0 ? (s.value / prev) * 100 : 100;
            const totalRate = (s.value / first) * 100;
            return (
              <div
                key={s.step}
                className="flex items-center justify-between gap-3 rounded-md border border-border px-3 py-2"
              >
                <div className="flex flex-col">
                  <span className="text-sm font-medium text-foreground">
                    {STEP_LABELS[s.step]}
                  </span>
                  <Caption>{s.step}</Caption>
                </div>
                <div className="flex flex-col items-end">
                  <Mono className="text-sm font-semibold">{formatInt(s.value)}</Mono>
                  <Caption className="font-mono font-tabular">
                    {i > 0 && `${stepRate.toFixed(1)}% step · `}
                    {totalRate.toFixed(1)}% total
                  </Caption>
                </div>
              </div>
            );
          })}
        </div>
      </CardContent>
    </Card>
  );
}
