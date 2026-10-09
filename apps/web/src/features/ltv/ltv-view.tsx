"use client";

import * as React from "react";
import type { ColumnDef } from "@tanstack/react-table";
import type { DateRange } from "react-day-picker";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { DateRangePicker } from "@/components/ui/date-range-picker";
import { ErrorState } from "@/components/ui/error-state";
import { LoadingState } from "@/components/ui/loading-state";
import { SearchableSelect } from "@/components/ui/searchable-select";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Caption } from "@/components/ui/typography";
import { useCampaigns } from "@/hooks/use-campaigns";
import { useLocale } from "@/hooks/use-locale";
import { useCohorts } from "@/hooks/use-ltv";
import { useNetworks } from "@/hooks/use-networks";
import { useApiErrorMessage } from "@/hooks/use-api-error-message";
import { LTV_WINDOWS, type Cohort, type CohortPeriod, type CohortType, type LtvWindow } from "@/lib/api/ltv";
import { countryOptions } from "@/lib/countries";
import { formatInt, formatPercent, formatUsd } from "@/lib/format";
import type { Locale } from "@/lib/i18n/config";
import { cn } from "@/lib/utils";

const ALL = "__all__";
const PERIODS: CohortPeriod[] = ["day", "week", "month"];

function ymd(d: Date): string {
  return d.toISOString().slice(0, 10);
}

/** Cohort days are UTC calendar days (internal/ltv), so the range is too. */
function defaultRange(): DateRange {
  const to = new Date();
  to.setUTCHours(0, 0, 0, 0);
  const from = new Date(to);
  from.setUTCDate(from.getUTCDate() - 89);
  return { from, to };
}

function cohortColumns(
  t: TFunction<"ltv">,
  locale: Locale,
  type: CohortType,
  campaignName: (id?: string) => string,
  networkName: (id?: string) => string,
): ColumnDef<typeof dataTableFeatures, Cohort>[] {
  const usd = (n: number) => formatUsd(n, 2, locale);
  const rate = (n?: number) => (n === undefined ? "—" : formatPercent(n * 100, locale));

  const windowColumn = (w: LtvWindow): ColumnDef<typeof dataTableFeatures, Cohort> => ({
    id: w,
    header: t(`windows.${w}`),
    accessorFn: (c) => c.windows[w]?.revenueUsd ?? 0,
    meta: { hint: t(`windowHints.${w}`) },
    cell: ({ row }) => {
      const value = row.original.windows[w];
      if (!value) return "—";
      return value.complete ? (
        usd(value.revenueUsd)
      ) : (
        <span className="text-muted-foreground italic" title={t("incomplete")}>
          {usd(value.revenueUsd)}*
        </span>
      );
    },
  });

  const columns: ColumnDef<typeof dataTableFeatures, Cohort>[] = [
    { accessorKey: "bucket", header: t("columns.cohort"), cell: ({ getValue }) => <span className="tabular-nums">{getValue() as string}</span> },
    { id: "campaign", header: t("columns.campaign"), accessorFn: (c) => campaignName(c.campaignId) },
    { id: "network", header: t("columns.network"), accessorFn: (c) => networkName(c.networkId) },
    { id: "country", header: t("columns.country"), accessorFn: (c) => c.country || "—" },
    {
      accessorKey: "anchorCount",
      header: type === "ftd" ? t("columns.ftds") : t("columns.regs"),
      cell: ({ getValue }) => formatInt(getValue() as number, locale),
    },
    ...LTV_WINDOWS.map(windowColumn),
    { accessorKey: "ltvTotalUsd", header: t("columns.ltvTotal"), cell: ({ getValue }) => usd(getValue() as number) },
    {
      accessorKey: "ltvPerAnchorUsd",
      header: type === "ftd" ? t("columns.ltvPerFtd") : t("columns.ltvPerReg"),
      cell: ({ getValue }) => usd(getValue() as number),
    },
  ];

  if (type === "ftd") {
    columns.push(
      { accessorKey: "ftdToRedepRate", header: t("columns.ftdToRedep"), cell: ({ row }) => rate(row.original.ftdToRedepRate) },
      { accessorKey: "depToRedepRate", header: t("columns.depToRedep"), cell: ({ row }) => rate(row.original.depToRedepRate) },
    );
  } else {
    columns.push({ accessorKey: "regToFtdRate", header: t("columns.regToFtd"), cell: ({ row }) => rate(row.original.regToFtdRate) });
  }

  columns.push(
    {
      id: "lifetime",
      header: t("columns.lifetime"),
      accessorFn: (c) => (c.hasLifetimeData ? (c.lifetimeDaysAvg ?? 0) : -1),
      cell: ({ row }) =>
        row.original.hasLifetimeData ? (row.original.lifetimeDaysAvg ?? 0).toLocaleString(locale, { maximumFractionDigits: 1 }) : "—",
    },
    { accessorKey: "totalDeposits", header: t("columns.totalDeposits"), cell: ({ getValue }) => formatInt(getValue() as number, locale) },
    { accessorKey: "totalDepositRevenueUsd", header: t("columns.totalDepositRevenue"), cell: ({ getValue }) => usd(getValue() as number) },
  );
  return columns;
}

export function LtvView() {
  const { t, i18n } = useTranslation("ltv");
  const errorMessage = useApiErrorMessage();
  const locale = useLocale();
  const [type, setType] = React.useState<CohortType>("ftd");
  const [period, setPeriod] = React.useState<CohortPeriod>("week");
  const [range, setRange] = React.useState<DateRange | undefined>(defaultRange);
  const [campaignId, setCampaignId] = React.useState(ALL);
  const [networkId, setNetworkId] = React.useState(ALL);
  const [country, setCountry] = React.useState("");

  const campaignsData = useCampaigns().data;
  const networksData = useNetworks().data;
  const campaigns = React.useMemo(() => campaignsData?.campaigns ?? [], [campaignsData]);
  const networks = React.useMemo(() => networksData?.networks ?? [], [networksData]);
  const fallback = defaultRange();
  const from = range?.from ?? fallback.from!;
  const to = range?.to ?? range?.from ?? fallback.to!;

  const query = useCohorts({
    type,
    period,
    from: ymd(from),
    to: ymd(to),
    campaignId: campaignId === ALL ? undefined : campaignId,
    networkId: networkId === ALL ? undefined : networkId,
    country: country || undefined,
  });

  const campaignName = React.useCallback(
    (id?: string) => (id ? (campaigns.find((c) => c.id === id)?.name ?? id) : "—"),
    [campaigns],
  );
  const networkName = React.useCallback(
    (id?: string) => (id ? (networks.find((n) => n.id === id)?.name ?? id) : "—"),
    [networks],
  );
  const columns = React.useMemo(
    () => cohortColumns(t, locale, type, campaignName, networkName),
    [t, locale, type, campaignName, networkName],
  );
  const countries = React.useMemo(
    () => [{ value: "", label: t("filters.allCountries") }, ...countryOptions(i18n.language)],
    [t, i18n.language],
  );

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <h1 className="text-2xl font-semibold tracking-tight">{t("title")}</h1>
        <Caption>{t("description")}</Caption>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Tabs value={type} onValueChange={(v) => setType(v as CohortType)}>
          <TabsList>
            <TabsTrigger value="ftd">{t("types.ftd")}</TabsTrigger>
            <TabsTrigger value="reg">{t("types.reg")}</TabsTrigger>
          </TabsList>
        </Tabs>
        <Select value={period} onValueChange={(v) => setPeriod(v as CohortPeriod)}>
          <SelectTrigger size="sm" className="w-32" aria-label={t("filters.period")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {PERIODS.map((p) => (
              <SelectItem key={p} value={p}>
                {t(`periods.${p}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <DateRangePicker value={range} onChange={setRange} timezone="UTC" />
        <Select value={campaignId} onValueChange={setCampaignId}>
          <SelectTrigger size="sm" className="w-48" aria-label={t("filters.campaign")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>{t("filters.allCampaigns")}</SelectItem>
            {campaigns.map((c) => (
              <SelectItem key={c.id} value={c.id}>
                {c.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={networkId} onValueChange={setNetworkId}>
          <SelectTrigger size="sm" className="w-44" aria-label={t("filters.network")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>{t("filters.allNetworks")}</SelectItem>
            {networks.map((n) => (
              <SelectItem key={n.id} value={n.id}>
                {n.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <SearchableSelect
          options={countries}
          value={country}
          onChange={setCountry}
          placeholder={t("filters.allCountries")}
          className="w-48"
        />
      </div>

      {query.isPending && <LoadingState label={t("loading")} />}
      {query.isError && <ErrorState title={t("loadError")} description={errorMessage(query.error)} onRetry={() => query.refetch()} />}
      {query.isSuccess && (
        <div className={cn("flex flex-col gap-2", query.isPlaceholderData && "opacity-60")}>
          <DataTable
            columns={columns}
            data={query.data.cohorts}
            enableSearch={false}
            exportFilename={`ltv-${type}-cohorts`}
            emptyTitle={t("emptyTitle")}
            emptyDescription={type === "ftd" ? t("emptyFtd") : t("emptyReg")}
            pageSize={25}
            persistKey={`ltv-${type}`}
          />
          <Caption>{t("legend")}</Caption>
        </div>
      )}
    </div>
  );
}
