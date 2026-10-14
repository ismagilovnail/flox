"use client";

import * as React from "react";
import type { ColumnDef } from "@tanstack/react-table";
import type { DateRange } from "react-day-picker";
import { useTranslation } from "react-i18next";

import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { DateRangePicker } from "@/components/ui/date-range-picker";
import { ErrorState } from "@/components/ui/error-state";
import { LoadingState } from "@/components/ui/loading-state";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { StatCard } from "@/components/ui/stat-card";
import { Mono } from "@/components/ui/typography";
import { formatMetric } from "@/features/analytics/registry";
import { useCampaigns } from "@/hooks/use-campaigns";
import { useCustomMetrics } from "@/hooks/use-custom-metrics";
import { useOrganization } from "@/hooks/use-organization";
import { usePushStats } from "@/hooks/use-push";
import { usePwas } from "@/hooks/use-pwas";
import { useLocale } from "@/hooks/use-locale";
import { PUSH_STATS_GROUPS, type PushMetrics, type PushStatsGroup } from "@/lib/api/push";
import { countryOptions } from "@/lib/countries";
import type { CustomMetric } from "@/lib/custom-metrics";
import { formatInt, formatPercent } from "@/lib/format";
import { evaluateFormula } from "@/lib/formula-engine";
import { todayInZone } from "@/lib/timezones";

type StatsRow = { key: string; label: string; metrics: PushMetrics };

/** Published, active custom metrics shown on Push > Statistics (§30.5:
 * push formulas only, one data source per formula). */
export function pushStatsCustomMetrics(all: CustomMetric[]): CustomMetric[] {
  return all.filter((m) => m.status === "published" && m.active && m.targets.includes("push_stats"));
}

function ymd(d: Date) {
  return d.toISOString().slice(0, 10);
}

function num(n: number) {
  return <Mono className="tabular-nums">{formatInt(n)}</Mono>;
}

function ctr(v: number | null) {
  return v === null ? "—" : formatPercent(v);
}

/** Push metrics (docs/push.md): what the organization's broadcasts sent in a
 * range did, by day, broadcast, PWA, campaign or country. Waits for the
 * organization's timezone, which cuts the days. */
export function PushStatsPanel() {
  const { t } = useTranslation("push");
  const org = useOrganization();
  if (org.isPending) return <LoadingState label={t("stats.loading")} />;
  return <PushStatsContent timeZone={org.data?.timezone || "UTC"} />;
}

function PushStatsContent({ timeZone }: { timeZone: string }) {
  const { t, i18n } = useTranslation("push");
  const locale = useLocale();
  const defaultTo = React.useMemo(() => todayInZone(timeZone), [timeZone]);
  const defaultFrom = React.useMemo(() => {
    const d = new Date(defaultTo);
    d.setUTCDate(d.getUTCDate() - 29);
    return d;
  }, [defaultTo]);
  const [range, setRange] = React.useState<DateRange | undefined>({ from: defaultFrom, to: defaultTo });
  const [groupBy, setGroupBy] = React.useState<PushStatsGroup>("day");
  const from = range?.from ?? defaultFrom;
  const to = range?.to ?? range?.from ?? defaultTo;

  const query = usePushStats({ from: ymd(from), to: ymd(to), timezone: timeZone, groupBy }, true);
  const pwas = usePwas().data?.pwas;
  const campaigns = useCampaigns().data?.campaigns;
  const customMetricsData = useCustomMetrics().data;
  const customMetrics = React.useMemo(() => pushStatsCustomMetrics(customMetricsData?.metrics ?? []), [customMetricsData]);
  const countries = React.useMemo(() => new Map(countryOptions(i18n.language).map((c) => [c.value, c.label])), [i18n.language]);

  const label = React.useCallback(
    (key: string, serverLabel?: string) => {
      // '' is the broadcasts sent before the breakdown existed.
      if (key === "") return t("stats.notSplit");
      switch (groupBy) {
        case "broadcast":
          return serverLabel || key;
        case "pwa":
          return pwas?.find((p) => p.id === key)?.name ?? key;
        case "campaign":
          return campaigns?.find((c) => c.id === key)?.name ?? key;
        case "country":
          return countries.get(key) ?? key;
        default:
          return key;
      }
    },
    [groupBy, pwas, campaigns, countries, t],
  );

  const rows: StatsRow[] = React.useMemo(
    () => (query.data?.rows ?? []).map((r) => ({ key: r.key, label: label(r.key, r.label), metrics: r.metrics })),
    [query.data, label],
  );

  const columns: ColumnDef<typeof dataTableFeatures, StatsRow>[] = [
    { accessorKey: "label", header: t(`stats.groups.${groupBy}`), cell: ({ getValue }) => <span className="font-medium">{getValue() as string}</span> },
    { id: "sent", accessorFn: (r) => r.metrics.push_sent, header: t("stats.columns.sent"), cell: ({ getValue }) => num(getValue() as number) },
    { id: "delivered", accessorFn: (r) => r.metrics.push_delivered, header: t("stats.columns.delivered"), cell: ({ getValue }) => num(getValue() as number) },
    { id: "failed", accessorFn: (r) => r.metrics.push_failed, header: t("stats.columns.failed"), cell: ({ getValue }) => num(getValue() as number) },
    {
      id: "unsubscribed",
      accessorFn: (r) => r.metrics.push_unsubscribed,
      header: t("stats.columns.unsubscribed"),
      cell: ({ getValue }) => num(getValue() as number),
    },
    { id: "opened", accessorFn: (r) => r.metrics.push_opened, header: t("stats.columns.opened"), cell: ({ getValue }) => num(getValue() as number) },
    {
      id: "ctr",
      accessorFn: (r) => r.metrics.push_ctr,
      header: t("stats.columns.ctr"),
      cell: ({ getValue }) => <Mono className="tabular-nums">{ctr(getValue() as number | null)}</Mono>,
    },
    ...customMetrics.map(
      (cm): ColumnDef<typeof dataTableFeatures, StatsRow> => ({
        id: `cm_${cm.id}`,
        header: cm.name,
        accessorFn: (r) => evaluateFormula(cm.formula, r.metrics),
        cell: ({ getValue }) => <Mono className="tabular-nums">{formatMetric(getValue() as number | null, cm.format)}</Mono>,
      }),
    ),
  ];

  const totals = query.data?.totals;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <DateRangePicker value={range} onChange={setRange} timezone={timeZone} />
        <Select value={groupBy} onValueChange={(v) => setGroupBy(v as PushStatsGroup)}>
          <SelectTrigger size="sm" className="w-44" aria-label={t("stats.groupBy")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {PUSH_STATS_GROUPS.map((g) => (
              <SelectItem key={g} value={g}>
                {t(`stats.groups.${g}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span className="text-xs text-muted-foreground">{t("stats.bySendDay")}</span>
      </div>

      {query.isPending ? (
        <LoadingState label={t("stats.loading")} />
      ) : query.isError ? (
        <ErrorState title={t("stats.loadError")} description={query.error.message} onRetry={() => query.refetch()} />
      ) : (
        <>
          {totals && (
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              <StatCard label={t("stats.columns.sent")} value={formatInt(totals.push_sent, locale)} />
              <StatCard label={t("stats.columns.delivered")} value={formatInt(totals.push_delivered, locale)} />
              <StatCard label={t("stats.columns.opened")} value={formatInt(totals.push_opened, locale)} />
              <StatCard label={t("stats.columns.ctr")} value={ctr(totals.push_ctr)} />
            </div>
          )}
          <DataTable
            columns={columns}
            data={rows}
            searchPlaceholder={t("stats.searchPlaceholder")}
            exportFilename={`push-stats-${groupBy}`}
            emptyTitle={t("stats.emptyTitle")}
            emptyDescription={t("stats.emptyDescription")}
            pageSize={50}
          />
        </>
      )}
    </div>
  );
}
