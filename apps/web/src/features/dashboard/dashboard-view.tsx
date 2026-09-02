"use client";

import * as React from "react";
import type { DateRange } from "react-day-picker";
import { useTranslation } from "react-i18next";

import { DateRangePicker } from "@/components/ui/date-range-picker";
import { LineMetricChart } from "@/components/charts/line-metric-chart";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { CHART_COLORS } from "@/lib/chart-theme";
import { formatUsd, formatInt } from "@/lib/format";
import { KpiGrid } from "@/features/dashboard/kpi-grid";
import { TopTable } from "@/features/dashboard/top-tables";
import { metricsFromApi, performanceRowFromApi } from "@/features/dashboard/metrics";
import { useDashboardReport } from "@/hooks/use-analytics-report";

function daysBetween(from: Date, to: Date) {
  return Math.max(Math.round((to.getTime() - from.getTime()) / 86_400_000) + 1, 1);
}

export function DashboardView() {
  const { t } = useTranslation("dashboard");

  const defaultTo = React.useMemo(() => new Date(), []);
  const defaultFrom = React.useMemo(() => {
    const d = new Date(defaultTo);
    d.setUTCDate(d.getUTCDate() - 29);
    return d;
  }, [defaultTo]);

  const [range, setRange] = React.useState<DateRange | undefined>({ from: defaultFrom, to: defaultTo });

  const from = range?.from ?? defaultFrom;
  const to = range?.to ?? range?.from ?? defaultTo;

  const spanDays = daysBetween(from, to);
  const prevTo = new Date(from);
  prevTo.setUTCDate(prevTo.getUTCDate() - 1);
  const prevFrom = new Date(prevTo);
  prevFrom.setUTCDate(prevFrom.getUTCDate() - (spanDays - 1));

  const currentQuery = useDashboardReport(from, to);
  const previousQuery = useDashboardReport(prevFrom, prevTo);

  if (currentQuery.isPending) {
    return <LoadingState label={t("loading")} />;
  }
  if (currentQuery.isError) {
    return (
      <ErrorState
        title={t("loadError")}
        description={currentQuery.error.message}
        onRetry={() => currentQuery.refetch()}
      />
    );
  }

  const data = currentQuery.data;
  const currentMetrics = metricsFromApi(data.totals);
  // Falls back to the current period when the comparison window hasn't
  // resolved yet (still loading, or errored) — renders as "no change"
  // rather than a second loading/error state for a secondary comparison.
  const previousMetrics = metricsFromApi(previousQuery.data?.totals ?? data.totals);

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold tracking-tight">{t("title")}</h1>
        <DateRangePicker value={range} onChange={setRange} />
      </div>

      <KpiGrid current={currentMetrics} previous={previousMetrics} />

      <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
        <LineMetricChart
          title={t("charts.revenue")}
          points={data.daily.map((p) => ({ date: p.day, value: p.metrics.revenue ?? 0 }))}
          color={CHART_COLORS.success}
          valueFormatter={formatUsd}
        />
        <LineMetricChart
          title={t("charts.spend")}
          points={data.daily.map((p) => ({ date: p.day, value: p.metrics.cost ?? 0 }))}
          color={CHART_COLORS.danger}
          valueFormatter={formatUsd}
        />
        <LineMetricChart
          title={t("charts.profit")}
          points={data.daily.map((p) => ({ date: p.day, value: p.metrics.profit ?? 0 }))}
          color={CHART_COLORS.primary}
          valueFormatter={formatUsd}
        />
        <LineMetricChart
          title={t("charts.conversions")}
          points={data.daily.map((p) => ({ date: p.day, value: p.metrics.conversions ?? 0 }))}
          color={CHART_COLORS.warning}
          valueFormatter={formatInt}
        />
      </div>

      {/* No Top Offers card — "offer" isn't a backed report dimension yet
          (DashboardResult's own doc comment); dropped rather than rendered
          against fake data. */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <TopTable entity="campaigns" rows={data.topCampaigns.map((r) => performanceRowFromApi(r, "campaign"))} />
        <TopTable entity="countries" rows={data.topCountries.map((r) => performanceRowFromApi(r, "country"))} />
        <TopTable entity="flows" rows={data.topFlows.map((r) => performanceRowFromApi(r, "flow"))} />
      </div>
    </div>
  );
}
