"use client";

import * as React from "react";
import { useSearchParams } from "next/navigation";
import { useTranslation } from "react-i18next";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { StatCard } from "@/components/ui/stat-card";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { ReportControls, type ReportControlsState } from "@/features/analytics/report-controls";
import { ReportPresetBar } from "@/features/analytics/report-preset-bar";
import { ReportTable } from "@/features/analytics/report-table";
import { ReportLineChart, ReportBarChart } from "@/features/analytics/report-chart";
import { FunnelChart } from "@/features/analytics/funnel-chart";
import type { FilterCondition } from "@/features/analytics/aggregate";
import { useReportingCurrency } from "@/hooks/use-organization";
import { METRICS, metricLabel, formatMetric, type DimensionKey } from "@/features/analytics/registry";
import { useReport, useTimeseries, useFunnelReport } from "@/hooks/use-analytics-report";
import { CostTimezoneNotice } from "@/features/analytics/cost-timezone-notice";
import { DateBasisNotice } from "@/features/analytics/date-basis";
import { useOrganization } from "@/hooks/use-organization";
import { todayInZone } from "@/lib/timezones";
import { EMPTY_CUSTOM_METRICS, useCustomMetrics } from "@/hooks/use-custom-metrics";
import { useApiErrorMessage } from "@/hooks/use-api-error-message";

/** Dimensions "View statistics" (§27.5) is allowed to deep-link a filter for —
 * an allowlist so an arbitrary/malformed URL query can't inject a bogus filter. */
const VIEW_STATS_DIMENSIONS: DimensionKey[] = ["campaign", "offer", "source", "stream_set", "flow", "landing", "pwa", "postlanding", "network"];

function previousRange(range: ReportControlsState["dateRange"]) {
  if (!range?.from) return undefined;
  const to = range.to ?? range.from;
  const spanMs = to.getTime() - range.from.getTime();
  const prevTo = new Date(range.from.getTime() - 24 * 60 * 60 * 1000);
  const prevFrom = new Date(prevTo.getTime() - spanMs);
  return { from: prevFrom, to: prevTo };
}

/** The organization's timezone (Settings) is the report's default; the picker in
 * the controls stays a per-report override. Waits for it so the first fetch isn't
 * a UTC one that gets thrown away; a failed settings load falls back to UTC. */
export function AnalyticsView() {
  const { t } = useTranslation("analytics");
  const org = useOrganization();
  if (org.isPending) return <LoadingState label={t("view.loading")} />;
  return <AnalyticsContent orgTimezone={org.data?.timezone || "UTC"} />;
}

function AnalyticsContent({ orgTimezone }: { orgTimezone: string }) {
  const { t } = useTranslation("analytics");
  const currency = useReportingCurrency();
  const errorMessage = useApiErrorMessage();
  const searchParams = useSearchParams();
  const allCustomMetrics = useCustomMetrics().data?.metrics ?? EMPTY_CUSTOM_METRICS;
  const reportBuilderMetrics = React.useMemo(
    () => allCustomMetrics.filter((m) => m.status === "published" && m.active && m.targets.includes("report_builder")),
    [allCustomMetrics],
  );

  const defaultTo = React.useMemo(() => todayInZone(orgTimezone), [orgTimezone]);
  const defaultFrom = React.useMemo(() => {
    const d = new Date(defaultTo);
    d.setUTCDate(d.getUTCDate() - 6);
    return d;
  }, [defaultTo]);

  const [state, setState] = React.useState<ReportControlsState>(() => {
    const dim = searchParams.get("dim");
    const val = searchParams.get("val");
    const filters: FilterCondition[] =
      dim && val && VIEW_STATS_DIMENSIONS.includes(dim as DimensionKey)
        ? [{ dimension: dim as DimensionKey, value: val }]
        : [];
    return {
      dateRange: { from: defaultFrom, to: defaultTo },
      timezone: orgTimezone,
      dateBasis: "conversion",
      dimensions: ["campaign"],
      // The PWA → registration → purchase funnel, each step's rate and cost,
      // then money (marketer audit #12: these were only reachable through
      // the metric picker). Saved presets and an unsaved draft keep their own.
      metrics: [
        "clicks",
        "pwaInstalls",
        "clickToInstall",
        "registrations",
        "installToReg",
        "purchases",
        "regToPurchase",
        "repurchases",
        "cost",
        "costPerInstall",
        "costPerReg",
        "costPerPurchase",
        "revenue",
        "profit",
        "roi",
        "epc",
      ],
      filters,
      columnOrder: [],
      columnWidths: {},
      hiddenColumns: [],
      groupBy: "campaign",
      sort: { key: "revenue", dir: "desc" },
      compare: false,
    };
  });

  const initialTab = searchParams.get("tab") === "line" ? "line" : "table";

  function onChange(patch: Partial<ReportControlsState>) {
    setState((s) => ({ ...s, ...patch }));
  }

  const from = state.dateRange?.from ?? defaultFrom;
  const to = state.dateRange?.to ?? state.dateRange?.from ?? defaultTo;

  const { timezone, dateBasis } = state;
  const tableQuery = useReport({ dims: state.dimensions, filters: state.filters, from, to, sort: state.sort, timezone, dateBasis });
  const timeseriesQuery = useTimeseries({ filters: state.filters, from, to, timezone, dateBasis });
  const barQuery = useReport({
    dims: [state.groupBy],
    filters: state.filters,
    from,
    to,
    sort: { key: "revenue", dir: "desc" },
    timezone,
    dateBasis,
  });
  const funnelQuery = useFunnelReport(from, to, state.filters, state.timezone);

  const prevRange = previousRange(state.dateRange);
  const compareCurrentQuery = useReport(
    { dims: [], filters: state.filters, from, to, sort: state.sort, timezone, dateBasis },
    { enabled: state.compare },
  );
  const comparePreviousQuery = useReport(
    {
      dims: [],
      filters: state.filters,
      from: prevRange?.from ?? from,
      to: prevRange?.to ?? to,
      sort: state.sort,
      timezone,
      dateBasis,
    },
    { enabled: state.compare && !!prevRange },
  );

  const compareRows =
    state.compare && compareCurrentQuery.data && comparePreviousQuery.data
      ? { current: compareCurrentQuery.data[0], previous: comparePreviousQuery.data[0] }
      : null;

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-semibold tracking-tight">{t("view.title")}</h1>

      <ReportPresetBar state={state} onApply={onChange} />
      <ReportControls state={state} onChange={onChange} />
      <CostTimezoneNotice from={from} to={to} timezone={state.timezone} />
      <DateBasisNotice
        dateBasis={dateBasis}
        datedByConversion={(tableQuery.data ?? []).reduce((n, r) => n + (r.datedByConversion ?? 0), 0)}
      />

      {compareRows && (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {state.metrics.map((m) => {
            const meta = METRICS.find((x) => x.key === m)!;
            const curr = compareRows.current?.metrics[m] ?? null;
            const prev = compareRows.previous?.metrics[m] ?? null;
            const delta = curr !== null && prev !== null && prev !== 0 ? ((curr - prev) / prev) * 100 : null;
            return (
              <StatCard
                key={m}
                label={metricLabel(t, m)}
                value={formatMetric(curr, meta.format, currency)}
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

      <Tabs defaultValue={initialTab}>
        <TabsList>
          <TabsTrigger value="table">{t("tabs.table")}</TabsTrigger>
          <TabsTrigger value="line">{t("tabs.line")}</TabsTrigger>
          <TabsTrigger value="bar">{t("tabs.bar")}</TabsTrigger>
          <TabsTrigger value="funnel">{t("tabs.funnel")}</TabsTrigger>
        </TabsList>
        <TabsContent value="table">
          {tableQuery.isPending ? (
            <LoadingState label={t("view.loading")} />
          ) : tableQuery.isError ? (
            <ErrorState
              title={t("view.loadError")}
              description={errorMessage(tableQuery.error)}
              onRetry={() => tableQuery.refetch()}
            />
          ) : (
            <ReportTable
              rows={tableQuery.data}
              dimensions={state.dimensions}
              metrics={state.metrics}
              customMetrics={reportBuilderMetrics}
              columnOrder={state.columnOrder}
              onColumnOrderChange={(columnOrder) => onChange({ columnOrder })}
              columnSizing={state.columnWidths}
              onColumnSizingChange={(columnWidths) => onChange({ columnWidths })}
              hiddenColumns={state.hiddenColumns}
              onHiddenColumnsChange={(hiddenColumns) => onChange({ hiddenColumns })}
            />
          )}
        </TabsContent>
        <TabsContent value="line">
          {timeseriesQuery.isPending ? (
            <LoadingState label={t("view.loading")} />
          ) : timeseriesQuery.isError ? (
            <ErrorState
              title={t("view.loadError")}
              description={errorMessage(timeseriesQuery.error)}
              onRetry={() => timeseriesQuery.refetch()}
            />
          ) : (
            <ReportLineChart points={timeseriesQuery.data} metrics={state.metrics} />
          )}
        </TabsContent>
        <TabsContent value="bar">
          {barQuery.isPending ? (
            <LoadingState label={t("view.loading")} />
          ) : barQuery.isError ? (
            <ErrorState
              title={t("view.loadError")}
              description={errorMessage(barQuery.error)}
              onRetry={() => barQuery.refetch()}
            />
          ) : (
            <ReportBarChart rows={barQuery.data} dimension={state.groupBy} metrics={state.metrics} />
          )}
        </TabsContent>
        <TabsContent value="funnel">
          {funnelQuery.isPending ? (
            <LoadingState label={t("view.loading")} />
          ) : funnelQuery.isError ? (
            <ErrorState
              title={t("view.loadError")}
              description={errorMessage(funnelQuery.error)}
              onRetry={() => funnelQuery.refetch()}
            />
          ) : (
            <FunnelChart steps={funnelQuery.data} />
          )}
        </TabsContent>
      </Tabs>
    </div>
  );
}
