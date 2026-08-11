/**
 * ECharts renders to canvas, which supports oklch() natively in evergreen
 * browsers — reuse the exact design tokens (globals.css) instead of a
 * second hardcoded hex palette that would drift from the design system.
 */
export type ChartMode = "dark" | "light";

const TOKENS = {
  dark: {
    text: "oklch(0.63 0.012 260)",
    axisLine: "oklch(1 0 0 / 8%)",
    splitLine: "oklch(1 0 0 / 6%)",
    tooltipBg: "oklch(0.19 0.006 260)",
    tooltipBorder: "oklch(1 0 0 / 8%)",
    tooltipText: "oklch(0.95 0.002 260)",
  },
  light: {
    text: "oklch(0.48 0.012 260)",
    axisLine: "oklch(0.9 0.004 260)",
    splitLine: "oklch(0.93 0.004 260)",
    tooltipBg: "oklch(1 0 0)",
    tooltipBorder: "oklch(0.9 0.004 260)",
    tooltipText: "oklch(0.18 0.006 260)",
  },
} as const;

export const CHART_COLORS = {
  primary: "oklch(0.68 0.16 255)",
  success: "oklch(0.72 0.16 145)",
  warning: "oklch(0.78 0.15 85)",
  danger: "oklch(0.65 0.2 25)",
};

/** Canvas text can't resolve CSS custom properties, so these mirror the
 * resolved stacks rather than referencing var(--font-sans/--font-mono). */
const FONT_SANS = "ui-sans-serif, system-ui, sans-serif";
const FONT_MONO = "ui-monospace, SFMono-Regular, Menlo, monospace";

export function chartBaseOption(mode: ChartMode) {
  const t = TOKENS[mode];
  return {
    textStyle: { fontFamily: FONT_SANS, color: t.text },
    animationDuration: 250,
    animationEasing: "cubicOut" as const,
    grid: { left: 8, right: 8, top: 24, bottom: 8, containLabel: true },
    xAxis: {
      type: "time" as const,
      axisLine: { lineStyle: { color: t.axisLine } },
      axisTick: { show: false },
      axisLabel: { color: t.text, fontSize: 11 },
      splitLine: { show: false },
    },
    yAxis: {
      type: "value" as const,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: t.text, fontSize: 11 },
      splitLine: { lineStyle: { color: t.splitLine } },
    },
    tooltip: {
      trigger: "axis" as const,
      backgroundColor: t.tooltipBg,
      borderColor: t.tooltipBorder,
      textStyle: { color: t.tooltipText, fontFamily: FONT_MONO, fontSize: 12 },
      confine: true,
    },
  };
}

/** Horizontal bar variant: category axis on y (reads long dimension labels better than rotated x labels). */
export function chartBarOption(mode: ChartMode, categories: string[]) {
  const t = TOKENS[mode];
  const base = chartBaseOption(mode);
  return {
    ...base,
    grid: { left: 8, right: 16, top: 8, bottom: 8, containLabel: true },
    xAxis: {
      type: "value" as const,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: t.text, fontSize: 11 },
      splitLine: { lineStyle: { color: t.splitLine } },
    },
    yAxis: {
      type: "category" as const,
      data: categories,
      inverse: true,
      axisLine: { lineStyle: { color: t.axisLine } },
      axisTick: { show: false },
      axisLabel: { color: t.text, fontSize: 11, width: 110, overflow: "truncate" as const },
      splitLine: { show: false },
    },
  };
}
