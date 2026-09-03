"use client";

import * as React from "react";
import { useTranslation } from "react-i18next";

import { DataTable } from "@/components/ui/data-table";
import { ErrorState } from "@/components/ui/error-state";
import { LoadingState } from "@/components/ui/loading-state";
import { useCampaigns } from "@/hooks/use-campaigns";
import { useClickLogs } from "@/hooks/use-click-logs";
import { CLICK_LOG_INITIAL_COLUMN_VISIBILITY, clickLogColumns } from "@/features/clicklogs/click-log-columns";

export function ClickLogView() {
  const { t } = useTranslation("clickLog");
  const logsQuery = useClickLogs();
  const campaignsQuery = useCampaigns();

  const campaignNameById = React.useMemo(
    () => Object.fromEntries((campaignsQuery.data?.campaigns ?? []).map((c) => [c.id, c.name])),
    [campaignsQuery.data],
  );

  const columns = React.useMemo(() => clickLogColumns(t, campaignNameById), [t, campaignNameById]);

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{t("view.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("view.description")}</p>
      </div>

      {logsQuery.isPending ? (
        <LoadingState label={t("view.loading")} />
      ) : logsQuery.isError ? (
        <ErrorState title={t("view.loadError")} description={logsQuery.error.message} onRetry={() => logsQuery.refetch()} />
      ) : (
        <DataTable
          columns={columns}
          data={logsQuery.data.logs}
          getRowId={(row) => `${row.clickId}:${row.eventAt}`}
          searchPlaceholder={t("view.searchPlaceholder")}
          emptyTitle={t("view.emptyTitle")}
          emptyDescription={t("view.emptyDescription")}
          initialColumnVisibility={CLICK_LOG_INITIAL_COLUMN_VISIBILITY}
          pageSize={20}
        />
      )}
    </div>
  );
}
