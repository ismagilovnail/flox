"use client";

import * as React from "react";
import { useTranslation } from "react-i18next";

import { DataTable } from "@/components/ui/data-table";
import { ErrorState } from "@/components/ui/error-state";
import { LoadingState } from "@/components/ui/loading-state";
import { useNetworks } from "@/hooks/use-networks";
import { usePostbackLogs } from "@/hooks/use-postback-logs";
import { postbackLogColumns } from "@/features/postbacks/postback-log-columns";

export function PostbackLogsPanel() {
  const { t } = useTranslation(["postbacks", "conversions"]);
  const logsQuery = usePostbackLogs();
  const networksQuery = useNetworks();

  const networkNameById = React.useMemo(
    () => Object.fromEntries((networksQuery.data?.networks ?? []).map((n) => [n.id, n.name])),
    [networksQuery.data],
  );

  const columns = React.useMemo(() => postbackLogColumns(t, networkNameById), [t, networkNameById]);

  if (logsQuery.isPending) {
    return <LoadingState label={t("logs.loading")} />;
  }

  if (logsQuery.isError) {
    return (
      <ErrorState
        title={t("logs.loadError")}
        description={logsQuery.error.message}
        onRetry={() => logsQuery.refetch()}
      />
    );
  }

  return (
    <DataTable
      columns={columns}
      data={logsQuery.data.logs}
      getRowId={(row) => `${row.direction}:${row.networkId}:${row.clickId}:${row.eventAt}`}
      searchPlaceholder={t("logs.searchPlaceholder")}
      emptyTitle={t("logs.emptyTitle")}
      emptyDescription={t("logs.emptyDescription")}
      pageSize={15}
    />
  );
}
