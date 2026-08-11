"use client";

import * as React from "react";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { StatCard } from "@/components/ui/stat-card";
import { ReportControls, type ReportControlsState } from "@/features/analytics/report-controls";
import { ReportTable } from "@/features/analytics/report-table";
import { ReportLineChart, ReportBarChart } from "@/features/analytics/report-chart";
import { FunnelChart } from "@/features/analytics/funnel-chart";
import { aggregateReport } from "@/features/analytics/aggregate";
import { METRICS, formatMetric } from "@/features/analytics/registry";
import { generateAnalyticsSlices } from "@/lib/mock/analytics";

const SLICES = generateAnalyticsSlices();

function previousRange(range: ReportControlsState["dateRange"]) {
  if (!range?.from) return undefined;
  const to = range.to ?? range.from;
  const spanMs = to.getTime() - range.from.getTime();
  const prevTo = new Date(range.from.getTime() - 24 * 60 * 60 * 1000);
  const prevFrom = new Date(prevTo.getTime() - spanMs);
  return { from: prevFrom, to: prevTo };
}

export function AnalyticsView() {
  const lastDate = new Date("2026-08-11T00:00:00Z");
  const defaultFrom = new Date(lastDate);
  defaultFrom.setUTCDate(defaultFrom.getUTCDate() - 29);

  const [state, setState] = React.useState<ReportControlsState>({
    dateRange: { from: defaultFrom, to: lastDate },
    timezone: "UTC",
    dimensions: ["campaign"],
    metrics: ["clicks", "conversions", "revenue", "cvr"],
    filters: [],
    groupBy: "campaign",
    sort: { key: "revenue", dir: "desc" },
    compare: false,
  });

  function onChange(patch: Partial<ReportControlsState>) {
    setState((s) => ({ ...s, ...patch }));
  }

  const rows = React.useMemo(
    () =>
      aggregateReport({
        slices: SLICES,
        dateRange: state.dateRange,
        dimensions: state.dimensions,
        filters: state.filters,
        sort: state.sort,
      }),
    [state.dateRange, state.dimensions, state.filters, state.sort],
  );

  const compareRows = React.useMemo(() => {
    if (!state.compare) return null;
    const current = aggregateReport({
      slices: SLICES,
      dateRange: state.dateRange,
      dimensions: [],
      filters: state.filters,
      sort: state.sort,
    })[0];
    const previous = aggregateReport({
      slices: SLICES,
      dateRange: previousRange(state.dateRange),
      dimensions: [],
      filters: state.filters,
      sort: state.sort,
    })[0];
    return { current, previous };
  }, [state.compare, state.dateRange, state.filters, state.sort]);

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-semibold tracking-tight">Analytics</h1>

      <ReportControls state={state} onChange={onChange} />

      {compareRows && (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {state.metrics.map((m) => {
            const meta = METRICS.find((x) => x.key === m)!;
            const curr = compareRows.current.metrics[m];
            const prev = compareRows.previous.metrics[m];
            const delta = curr !== null && prev !== null && prev !== 0 ? ((curr - prev) / prev) * 100 : null;
            return (
              <StatCard
                key={m}
                label={meta.label}
                value={formatMetric(curr, meta.format)}
                {...(delta !== null
                  ? {
                      trend: (Math.abs(delta) < 0.5 ? "flat" : delta > 0 ? "up" : "down") as
                        | "up"
                        | "down"
                        | "flat",
                      delta: `${delta > 0 ? "+" : ""}${delta.toFixed(1)}%`,
                    }
                  : {})}
              />
            );
          })}
        </div>
      )}

      <Tabs defaultValue="table">
        <TabsList>
          <TabsTrigger value="table">Table</TabsTrigger>
          <TabsTrigger value="line">Line</TabsTrigger>
          <TabsTrigger value="bar">Bar</TabsTrigger>
          <TabsTrigger value="funnel">Funnel</TabsTrigger>
        </TabsList>
        <TabsContent value="table">
          <ReportTable rows={rows} dimensions={state.dimensions} metrics={state.metrics} />
        </TabsContent>
        <TabsContent value="line">
          <ReportLineChart
            slices={SLICES}
            dateRange={state.dateRange}
            filters={state.filters}
            metrics={state.metrics}
          />
        </TabsContent>
        <TabsContent value="bar">
          <ReportBarChart
            slices={SLICES}
            dateRange={state.dateRange}
            filters={state.filters}
            dimension={state.groupBy}
            metrics={state.metrics}
          />
        </TabsContent>
        <TabsContent value="funnel">
          <FunnelChart />
        </TabsContent>
      </Tabs>
    </div>
  );
}
