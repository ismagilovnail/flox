"use client";

import * as React from "react";
import type { DateRange } from "react-day-picker";
import { useTranslation } from "react-i18next";

import { DateRangePicker } from "@/components/ui/date-range-picker";
import { LineMetricChart } from "@/components/charts/line-metric-chart";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { formatUsd, formatInt } from "@/lib/format";
import { KpiGrid } from "@/features/dashboard/kpi-grid";
import { TopTable } from "@/features/dashboard/top-tables";
import { metricsFromApi, performanceRowFromApi } from "@/features/dashboard/metrics";
import { useDimensionValueLabel } from "@/features/analytics/use-dimension-value-label";
import { useDashboardReport } from "@/hooks/use-analytics-report";
import { CostTimezoneNotice } from "@/features/analytics/cost-timezone-notice";
import { DateBasisNotice, DateBasisSelect } from "@/features/analytics/date-basis";
import type { DateBasis } from "@/lib/api/analytics";
import { useOrganization } from "@/hooks/use-organization";
import { useLocale } from "@/hooks/use-locale";
import { useApiErrorMessage } from "@/hooks/use-api-error-message";
import { todayInZone } from "@/lib/timezones";

function daysBetween(from: Date, to: Date) {
  return Math.max(Math.round((to.getTime() - from.getTime()) / 86_400_000) + 1, 1);
}

/** Waits for the organization's timezone: "today" and every day boundary on the
 * overview are cut in it, and a first fetch in UTC would be thrown away. A failed
 * settings load falls back to UTC rather than blocking the page. */
export function DashboardView() {
  const { t } = useTranslation("dashboard");
  const org = useOrganization();
  if (org.isPending) return <LoadingState label={t("loading")} />;
  return <DashboardContent timezone={org.data?.timezone || "UTC"} />;
}

function DashboardContent({ timezone }: { timezone: string }) {
  const { t } = useTranslation("dashboard");
  const errorMessage = useApiErrorMessage();
  const locale = useLocale();
  // Range days are UTC-midnight Dates holding the organization's calendar day.
  const formatDay = (d: Date) => new Intl.DateTimeFormat(locale, { day: "numeric", month: "short", timeZone: "UTC" }).format(d);
  const valueLabel = useDimensionValueLabel();

  const defaultTo = React.useMemo(() => todayInZone(timezone), [timezone]);
  const defaultFrom = React.useMemo(() => {
    const d = new Date(defaultTo);
    d.setUTCDate(d.getUTCDate() - 29);
    return d;
  }, [defaultTo]);

  const [range, setRange] = React.useState<DateRange | undefined>({ from: defaultFrom, to: defaultTo });
  const [dateBasis, setDateBasis] = React.useState<DateBasis>("conversion");

  const from = range?.from ?? defaultFrom;
  const to = range?.to ?? range?.from ?? defaultTo;

  const spanDays = daysBetween(from, to);
  const prevTo = new Date(from);
  prevTo.setUTCDate(prevTo.getUTCDate() - 1);
  const prevFrom = new Date(prevTo);
  prevFrom.setUTCDate(prevFrom.getUTCDate() - (spanDays - 1));

  const currentQuery = useDashboardReport(from, to, timezone, dateBasis);
  const previousQuery = useDashboardReport(prevFrom, prevTo, timezone, dateBasis);

  if (currentQuery.isPending) {
    return <LoadingState label={t("loading")} />;
  }
  if (currentQuery.isError) {
    return (
      <ErrorState
        title={t("loadError")}
        description={errorMessage(currentQuery.error)}
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
        <div className="flex flex-wrap items-center gap-2">
          <DateBasisSelect value={dateBasis} onValueChange={setDateBasis} />
          <DateRangePicker value={range} onChange={setRange} timezone={timezone} />
        </div>
      </div>

      <CostTimezoneNotice from={from} to={to} timezone={timezone} />
      <DateBasisNotice dateBasis={dateBasis} datedByConversion={data.datedByConversion} />

      <div className="flex flex-col gap-1.5">
        <KpiGrid current={currentMetrics} previous={previousMetrics} />
        <p className="text-xs text-muted-foreground">
          {t("comparedWith", { from: formatDay(prevFrom), to: formatDay(prevTo) })}
        </p>
      </div>

      <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
        <LineMetricChart
          title={t("charts.revenue")}
          points={data.daily.map((p) => ({ date: p.day, value: p.metrics.revenue ?? 0 }))}
          color="success"
          valueFormatter={formatUsd}
        />
        <LineMetricChart
          title={t("charts.spend")}
          points={data.daily.map((p) => ({ date: p.day, value: p.metrics.cost ?? 0 }))}
          color="danger"
          valueFormatter={formatUsd}
        />
        <LineMetricChart
          title={t("charts.profit")}
          points={data.daily.map((p) => ({ date: p.day, value: p.metrics.profit ?? 0 }))}
          color="primary"
          valueFormatter={formatUsd}
        />
        <LineMetricChart
          title={t("charts.conversions")}
          points={data.daily.map((p) => ({ date: p.day, value: p.metrics.conversions ?? 0 }))}
          color="warning"
          valueFormatter={formatInt}
        />
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <TopTable entity="campaigns" rows={data.topCampaigns.map((r) => performanceRowFromApi(r, "campaign", valueLabel))} />
        <TopTable entity="countries" rows={data.topCountries.map((r) => performanceRowFromApi(r, "country", valueLabel))} />
        <TopTable entity="flows" rows={data.topFlows.map((r) => performanceRowFromApi(r, "flow", valueLabel))} />
        <TopTable entity="offers" rows={data.topOffers.map((r) => performanceRowFromApi(r, "offer", valueLabel))} />
      </div>
    </div>
  );
}
