"use client";

import * as React from "react";
import type { DateRange } from "react-day-picker";

import { DateRangePicker } from "@/components/ui/date-range-picker";
import { LineMetricChart } from "@/components/charts/line-metric-chart";
import { CHART_COLORS } from "@/lib/chart-theme";
import type { DashboardMock } from "@/lib/mock/dashboard";
import { KpiGrid } from "@/features/dashboard/kpi-grid";
import { TopTable } from "@/features/dashboard/top-tables";
import { aggregate } from "@/features/dashboard/metrics";

const usd = (n: number) =>
  n.toLocaleString(undefined, { style: "currency", currency: "USD", maximumFractionDigits: 0 });

function toDate(iso: string) {
  return new Date(`${iso}T00:00:00Z`);
}

export function DashboardView({ mock }: { mock: DashboardMock }) {
  const { daily } = mock;
  const lastDate = toDate(daily[daily.length - 1].date);
  const defaultFrom = new Date(lastDate);
  defaultFrom.setUTCDate(defaultFrom.getUTCDate() - 29);

  const [range, setRange] = React.useState<DateRange | undefined>({
    from: defaultFrom,
    to: lastDate,
  });

  const from = range?.from ?? defaultFrom;
  const to = range?.to ?? range?.from ?? lastDate;

  const current = daily.filter((p) => {
    const d = toDate(p.date);
    return d >= from && d <= to;
  });
  const spanDays = Math.max(current.length, 1);
  const prevTo = new Date(from);
  prevTo.setUTCDate(prevTo.getUTCDate() - 1);
  const prevFrom = new Date(prevTo);
  prevFrom.setUTCDate(prevFrom.getUTCDate() - (spanDays - 1));
  const previous = daily.filter((p) => {
    const d = toDate(p.date);
    return d >= prevFrom && d <= prevTo;
  });

  const currentMetrics = aggregate(current);
  const previousMetrics = aggregate(previous.length > 0 ? previous : current);

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold tracking-tight">Overview</h1>
        <DateRangePicker value={range} onChange={setRange} />
      </div>

      <KpiGrid current={currentMetrics} previous={previousMetrics} />

      <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
        <LineMetricChart
          title="Revenue"
          points={current.map((p) => ({ date: p.date, value: p.revenue }))}
          color={CHART_COLORS.success}
          valueFormatter={usd}
        />
        <LineMetricChart
          title="Spend"
          points={current.map((p) => ({ date: p.date, value: p.spend }))}
          color={CHART_COLORS.danger}
          valueFormatter={usd}
        />
        <LineMetricChart
          title="Profit"
          points={current.map((p) => ({ date: p.date, value: p.profit }))}
          color={CHART_COLORS.primary}
          valueFormatter={usd}
        />
        <LineMetricChart
          title="Conversions"
          points={current.map((p) => ({ date: p.date, value: p.conversions }))}
          color={CHART_COLORS.warning}
          valueFormatter={(v) => v.toLocaleString()}
        />
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <TopTable title="Top Campaigns" nameHeader="Campaign" rows={mock.topCampaigns} />
        <TopTable title="Top Offers" nameHeader="Offer" rows={mock.topOffers} />
        <TopTable title="Top Countries" nameHeader="Country" rows={mock.topCountries} />
        <TopTable title="Top Flows" nameHeader="Flow" rows={mock.topFlows} />
      </div>
    </div>
  );
}
