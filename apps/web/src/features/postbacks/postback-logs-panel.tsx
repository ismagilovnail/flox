"use client";

import * as React from "react";

import { DataTable } from "@/components/ui/data-table";
import { ErrorState } from "@/components/ui/error-state";
import { LoadingState } from "@/components/ui/loading-state";
import { useNetworks } from "@/hooks/use-networks";
import { usePostbackLogs } from "@/hooks/use-postback-logs";
import { postbackLogColumns } from "@/features/postbacks/postback-log-columns";

export function PostbackLogsPanel() {
  const logsQuery = usePostbackLogs();
  const networksQuery = useNetworks();

  const networkNameById = React.useMemo(
    () => Object.fromEntries((networksQuery.data?.networks ?? []).map((n) => [n.id, n.name])),
    [networksQuery.data],
  );

  const columns = React.useMemo(() => postbackLogColumns(networkNameById), [networkNameById]);

  if (logsQuery.isPending) {
    return <LoadingState label="Loading postback logs…" />;
  }

  if (logsQuery.isError) {
    return (
      <ErrorState
        title="Couldn't load postback logs"
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
      searchPlaceholder="Search by click ID..."
      emptyTitle="No postback activity yet"
      emptyDescription="Every incoming and outgoing postback attempt is logged here, success or not."
      pageSize={15}
    />
  );
}
