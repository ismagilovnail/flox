"use client";

import * as React from "react";
import { useTranslation } from "react-i18next";

import { LogGrid } from "@/components/shared/log-grid";
import { useCampaigns } from "@/hooks/use-campaigns";
import { useClickLogs } from "@/hooks/use-click-logs";
import { useLogGrid } from "@/hooks/use-log-grid";
import { useApiErrorMessage } from "@/hooks/use-api-error-message";
import { listClickLogs, type ClickLog, type ListClickLogsParams } from "@/lib/api/click-logs";
import { CLICK_LOG_INITIAL_COLUMN_VISIBILITY, clickLogColumns } from "@/features/clicklogs/click-log-columns";
import { RawRetentionNotice } from "@/features/analytics/raw-retention-notice";

const fetchClickLogPage = (filter: ListClickLogsParams, offset: number, limit: number) =>
  listClickLogs({ ...filter, offset, limit }).then((r) => ({ rows: r.logs, total: r.total }));

export function ClickLogView() {
  const { t } = useTranslation("clickLog");
  const errorMessage = useApiErrorMessage();
  const grid = useLogGrid();
  const logsQuery = useClickLogs(grid.params);
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
      <RawRetentionNotice from={grid.range.from} variant="clickLog" />

      <LogGrid<ClickLog>
        grid={grid}
        query={logsQuery}
        select={() => logsQuery.data && { rows: logsQuery.data.logs, total: logsQuery.data.total }}
        fetchPage={fetchClickLogPage}
        loadingLabel={t("view.loading")}
        errorTitle={t("view.loadError")}
        errorMessage={errorMessage}
        ariaLabel={t("view.title")}
        columns={columns}
        getRowId={(row) => `${row.clickId}:${row.eventAt}:${row.type}`}
        exportFilename="click-log"
        emptyTitle={t("view.emptyTitle")}
        emptyDescription={t("view.emptyDescription")}
        initialColumnVisibility={CLICK_LOG_INITIAL_COLUMN_VISIBILITY}
        enableColumnReorder
        persistKey="click-log"
      />
    </div>
  );
}
