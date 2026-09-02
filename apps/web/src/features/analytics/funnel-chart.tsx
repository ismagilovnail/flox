"use client";

import * as React from "react";
import ReactECharts from "echarts-for-react";
import { useTheme } from "next-themes";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Mono, Caption } from "@/components/ui/typography";
import { chartBaseOption, CHART_COLORS, type ChartMode } from "@/lib/chart-theme";
import { formatInt } from "@/lib/format";
import type { FunnelStepData } from "@/lib/api/analytics";

function stepLabel(t: TFunction, step: string): string {
  return t(`funnelSteps.${step}`, { ns: "analytics", defaultValue: step });
}

export function FunnelChart({ steps }: { steps: FunnelStepData[] }) {
  const { t } = useTranslation("analytics");
  const { resolvedTheme } = useTheme();
  const mode: ChartMode = resolvedTheme === "light" ? "light" : "dark";

  const first = steps[0]?.value || 1;

  const option = React.useMemo(() => {
    const base = chartBaseOption(mode);
    return {
      textStyle: base.textStyle,
      // Threaded explicitly (this option is hand-built, not spread from
      // base) — `animation` especially: without it this chart would be
      // the one place animation stayed on. See lib/chart-theme.ts.
      animation: base.animation,
      animationDuration: base.animationDuration,
      animationDurationUpdate: base.animationDurationUpdate,
      tooltip: {
        ...base.tooltip,
        trigger: "item" as const,
        formatter: (p: { name: string; value: number }) =>
          `${p.name}<br/>${formatInt(p.value)} (${((p.value / first) * 100).toFixed(1)}% of ${stepLabel(t, "SOURCE_CLICK")})`,
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
            formatter: (p: { name: string }) => stepLabel(t, p.name),
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
  }, [mode, steps, first, t]);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("charts.funnelTitle")}</CardTitle>
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
            const stepRate = i > 0 ? (prev > 0 ? (s.value / prev) * 100 : 0) : 100;
            const totalRate = (s.value / first) * 100;
            return (
              <div
                key={s.step}
                className="flex items-center justify-between gap-3 rounded-md border border-border px-3 py-2"
              >
                <div className="flex flex-col">
                  <span className="text-sm font-medium text-foreground">
                    {stepLabel(t, s.step)}
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
