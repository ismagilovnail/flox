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
import { MultiSelect } from "@/components/ui/multi-select";
import { SearchableSelect } from "@/components/ui/searchable-select";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Caption } from "@/components/ui/typography";
import { useCampaigns } from "@/hooks/use-campaigns";
import { useLocale } from "@/hooks/use-locale";
import { useCohorts } from "@/hooks/use-ltv";
import { useNetworks } from "@/hooks/use-networks";
import { useOffers } from "@/hooks/use-offers";
import { useTrafficSources } from "@/hooks/use-traffic-sources";
import { useApiErrorMessage } from "@/hooks/use-api-error-message";
import {
  LTV_DIMENSIONS,
  LTV_WINDOWS,
  type Cohort,
  type CohortPeriod,
  type CohortType,
  type LtvDimension,
  type LtvWindow,
} from "@/lib/api/ltv";
import { countryOptions } from "@/lib/countries";
import { formatInt, formatMoney, formatPercent } from "@/lib/format";
import { useReportingCurrency } from "@/hooks/use-organization";
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
  groupBy: LtvDimension[],
  valueName: (dim: LtvDimension, c: Cohort) => string,
  currency: string,
): ColumnDef<typeof dataTableFeatures, Cohort>[] {
  const money = (n: number) => formatMoney(n, currency, 2, locale);
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
        money(value.revenueUsd)
      ) : (
        <span className="text-muted-foreground italic" title={t("incomplete")}>
          {money(value.revenueUsd)}*
        </span>
      );
    },
  });

  const columns: ColumnDef<typeof dataTableFeatures, Cohort>[] = [
    { accessorKey: "bucket", header: t("columns.cohort"), cell: ({ getValue }) => <span className="tabular-nums">{getValue() as string}</span> },
    // Only the breakdowns asked for: a dimension not grouped by spans all
    // its values, so a column for it would only ever say "—".
    ...groupBy.map(
      (d): ColumnDef<typeof dataTableFeatures, Cohort> => ({ id: d, header: t(`columns.${d}`), accessorFn: (c) => valueName(d, c) }),
    ),
    {
      accessorKey: "anchorCount",
      header: type === "ftd" ? t("columns.ftds") : t("columns.regs"),
      cell: ({ getValue }) => formatInt(getValue() as number, locale),
    },
    ...LTV_WINDOWS.map(windowColumn),
    { accessorKey: "ltvTotalUsd", header: t("columns.ltvTotal"), cell: ({ getValue }) => money(getValue() as number) },
    {
      accessorKey: "ltvPerAnchorUsd",
      header: type === "ftd" ? t("columns.ltvPerFtd") : t("columns.ltvPerReg"),
      cell: ({ getValue }) => money(getValue() as number),
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
    { accessorKey: "totalDepositRevenueUsd", header: t("columns.totalDepositRevenue"), cell: ({ getValue }) => money(getValue() as number) },
  );
  return columns;
}

export function LtvView() {
  const { t, i18n } = useTranslation("ltv");
  const errorMessage = useApiErrorMessage();
  const locale = useLocale();
  const currency = useReportingCurrency();
  const [type, setType] = React.useState<CohortType>("ftd");
  const [period, setPeriod] = React.useState<CohortPeriod>("week");
  const [range, setRange] = React.useState<DateRange | undefined>(defaultRange);
  const [campaignId, setCampaignId] = React.useState(ALL);
  const [sourceId, setSourceId] = React.useState(ALL);
  const [offerId, setOfferId] = React.useState(ALL);
  const [networkId, setNetworkId] = React.useState(ALL);
  const [country, setCountry] = React.useState("");
  const [breakdown, setBreakdown] = React.useState<LtvDimension[]>([]);
  // Canonical order, so the columns and the query key do not depend on
  // the order the boxes were ticked in.
  const groupBy = React.useMemo(() => LTV_DIMENSIONS.filter((d) => breakdown.includes(d)), [breakdown]);

  const campaignsData = useCampaigns().data;
  const sourcesData = useTrafficSources().data;
  const offersData = useOffers().data;
  const networksData = useNetworks().data;
  const campaigns = React.useMemo(() => campaignsData?.campaigns ?? [], [campaignsData]);
  const sources = React.useMemo(() => sourcesData?.trafficSources ?? [], [sourcesData]);
  const offers = React.useMemo(() => offersData?.offers ?? [], [offersData]);
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
    sourceId: sourceId === ALL ? undefined : sourceId,
    offerId: offerId === ALL ? undefined : offerId,
    networkId: networkId === ALL ? undefined : networkId,
    country: country || undefined,
    groupBy,
  });

  // A row's breakdown value by name; "—" for clicks without one (e.g. a
  // redirect flow has no offer).
  const valueName = React.useCallback(
    (dim: LtvDimension, c: Cohort) => {
      const named = (list: { id: string; name: string }[], id?: string) => (id ? (list.find((x) => x.id === id)?.name ?? id) : "—");
      switch (dim) {
        case "campaign":
          return named(campaigns, c.campaignId);
        case "source":
          return named(sources, c.sourceId);
        case "offer":
          return named(offers, c.offerId);
        case "network":
          return named(networks, c.networkId);
        case "country":
          return c.country || "—";
      }
    },
    [campaigns, sources, offers, networks],
  );
  const columns = React.useMemo(
    () => cohortColumns(t, locale, type, groupBy, valueName, currency),
    [t, locale, type, groupBy, valueName, currency],
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
        <Select value={sourceId} onValueChange={setSourceId}>
          <SelectTrigger size="sm" className="w-44" aria-label={t("filters.source")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>{t("filters.allSources")}</SelectItem>
            {sources.map((s) => (
              <SelectItem key={s.id} value={s.id}>
                {s.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={offerId} onValueChange={setOfferId}>
          <SelectTrigger size="sm" className="w-44" aria-label={t("filters.offer")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>{t("filters.allOffers")}</SelectItem>
            {offers.map((o) => (
              <SelectItem key={o.id} value={o.id}>
                {o.name}
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
        <MultiSelect
          label={t("filters.breakdown")}
          options={LTV_DIMENSIONS.map((d) => ({ value: d, label: t(`dimensions.${d}`) }))}
          selected={breakdown}
          onChange={(v) => setBreakdown(v as LtvDimension[])}
          className="w-56"
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
