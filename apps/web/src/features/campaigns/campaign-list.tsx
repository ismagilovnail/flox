"use client";

import * as React from "react";
import Link from "next/link";
import { PlusIcon, TagIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import type { DateRange } from "react-day-picker";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { DateRangePicker } from "@/components/ui/date-range-picker";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { usePermission } from "@/hooks/use-auth";
import { useCampaigns } from "@/hooks/use-campaigns";
import { useOrganization, useReportingCurrency } from "@/hooks/use-organization";
import { useReport } from "@/hooks/use-analytics-report";
import { useCustomMetrics } from "@/hooks/use-custom-metrics";
import { useApiErrorMessage } from "@/hooks/use-api-error-message";
import { todayInZone } from "@/lib/timezones";
import type { DateBasis } from "@/lib/api/analytics";
import { DateBasisNotice, DateBasisSelect } from "@/features/analytics/date-basis";
import { CostTimezoneNotice } from "@/features/analytics/cost-timezone-notice";
import { campaignTableCustomMetrics, withCampaignMetrics } from "@/features/campaigns/campaign-metrics";
import { useTrafficSources } from "@/hooks/use-traffic-sources";
import { EMPTY_TAG_ASSIGNMENTS, useTagAssignments } from "@/hooks/use-tags";
import { campaignColumns } from "@/features/campaigns/campaign-columns";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";

// One report row per campaign; the sort only orders the response.
const REPORT_DIMS = ["campaign"] as const;
const REPORT_SORT = { key: "clicks", dir: "desc" } as const;
const NO_FILTERS: never[] = [];

export function CampaignList() {
  const { t } = useTranslation(["campaigns", "common"]);
  const currency = useReportingCurrency();
  const errorMessage = useApiErrorMessage();
  const canWrite = usePermission("campaign.write");
  const campaignsQuery = useCampaigns();

  // The period's numbers, cut in the organization's time zone like the
  // dashboard: the last 30 days by default. Waits for the zone so the
  // first report is not fetched in UTC and thrown away.
  const org = useOrganization();
  const timezone = org.data?.timezone || "UTC";
  const defaultTo = React.useMemo(() => todayInZone(timezone), [timezone]);
  const defaultFrom = React.useMemo(() => {
    const d = new Date(defaultTo);
    d.setUTCDate(d.getUTCDate() - 29);
    return d;
  }, [defaultTo]);
  const [range, setRange] = React.useState<DateRange | undefined>();
  const [dateBasis, setDateBasis] = React.useState<DateBasis>("conversion");
  const from = range?.from ?? defaultFrom;
  const to = range?.to ?? range?.from ?? defaultTo;
  const reportQuery = useReport(
    { dims: [...REPORT_DIMS], filters: NO_FILTERS, from, to, sort: REPORT_SORT, timezone, dateBasis },
    { enabled: !org.isPending },
  );
  const customMetricsData = useCustomMetrics().data;
  const customMetrics = React.useMemo(() => campaignTableCustomMetrics(customMetricsData?.metrics ?? []), [customMetricsData]);
  const sourcesQuery = useTrafficSources();
  const assignments = useTagAssignments("campaign").data?.assignments ?? EMPTY_TAG_ASSIGNMENTS;

  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  const sourceNameById = React.useMemo(() => {
    const map: Record<string, string> = {};
    for (const s of sourcesQuery.data?.trafficSources ?? []) map[s.id] = s.name;
    return map;
  }, [sourcesQuery.data]);

  const rows = React.useMemo(
    () => withCampaignMetrics(campaignsQuery.data?.campaigns ?? [], reportQuery.data),
    [campaignsQuery.data, reportQuery.data],
  );
  const filtered = React.useMemo(() => filterByTags("campaign", rows, tagFilter, assignments), [rows, tagFilter, assignments]);

  const columns = React.useMemo(
    () => campaignColumns(t, sourceNameById, customMetrics, reportQuery.isPending, currency),
    [t, sourceNameById, customMetrics, reportQuery.isPending, currency],
  );

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title", { ns: "campaigns" })}</h1>
      {canWrite && (
        <Button asChild>
          <Link href="/campaigns/new">
            <PlusIcon className="size-4" />
            {t("list.newButton", { ns: "campaigns" })}
          </Link>
        </Button>
      )}
    </div>
  );

  if (campaignsQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <LoadingState label={t("list.loading", { ns: "campaigns" })} />
      </div>
    );
  }

  if (campaignsQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState
          title={t("list.loadError", { ns: "campaigns" })}
          description={campaignsQuery.error.message}
          onRetry={() => campaignsQuery.refetch()}
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {header}

      <div className="flex flex-wrap items-center justify-end gap-2">
        <DateBasisSelect value={dateBasis} onValueChange={setDateBasis} />
        <DateRangePicker value={range ?? { from, to }} onChange={setRange} timezone={timezone} />
      </div>
      <CostTimezoneNotice from={from} to={to} timezone={timezone} />
      {reportQuery.data && (
        <DateBasisNotice
          dateBasis={dateBasis}
          datedByConversion={reportQuery.data.reduce((n, r) => n + (r.datedByConversion ?? 0), 0)}
        />
      )}
      {reportQuery.isError && (
        <ErrorState
          title={t("list.metricsError", { ns: "campaigns" })}
          description={errorMessage(reportQuery.error)}
          onRetry={() => reportQuery.refetch()}
        />
      )}

      <DataTable
        columns={columns}
        data={filtered}
        searchPlaceholder={t("list.searchPlaceholder", { ns: "campaigns" })}
        exportFilename="campaigns"
        emptyTitle={t("list.emptyTitle", { ns: "campaigns" })}
        emptyDescription={t("list.emptyDescription", { ns: "campaigns" })}
        pageSize={10}
        enableColumnReorder
        persistKey="campaigns"
        filters={<TagFilterControl selected={tagFilter} onChange={setTagFilter} />}
        enableRowSelection
        getRowId={(row) => row.id}
        bulkActions={({ selectedRows, clearSelection }) => (
          <Button
            size="sm"
            variant="outline"
            onClick={() => setBulkTarget({ ids: selectedRows.map((r) => r.id), clear: clearSelection })}
          >
            <TagIcon className="size-3.5" /> {t("list.editTagsButton", { ns: "campaigns" })}
          </Button>
        )}
      />

      {bulkTarget && (
        <BulkTagDialog
          entityType="campaign"
          entityIds={bulkTarget.ids}
          open
          onOpenChange={(open) => !open && setBulkTarget(null)}
          onApplied={() => {
            bulkTarget.clear();
            setBulkTarget(null);
          }}
        />
      )}
    </div>
  );
}
