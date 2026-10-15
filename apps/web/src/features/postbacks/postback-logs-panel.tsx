"use client";

import * as React from "react";
import { useTranslation } from "react-i18next";

import { LogGrid } from "@/components/shared/log-grid";
import { useNetworks } from "@/hooks/use-networks";
import { useLogGrid } from "@/hooks/use-log-grid";
import { usePostbackLogs } from "@/hooks/use-postback-logs";
import { useApiErrorMessage } from "@/hooks/use-api-error-message";
import { listPostbackLogs, type ListPostbackLogsParams, type PostbackLog } from "@/lib/api/postback-logs";
import { postbackLogColumns } from "@/features/postbacks/postback-log-columns";

const fetchPostbackLogPage = (filter: ListPostbackLogsParams, offset: number, limit: number) =>
  listPostbackLogs({ ...filter, offset, limit }).then((r) => ({ rows: r.logs, total: r.total }));

export function PostbackLogsPanel() {
  const { t } = useTranslation(["postbacks", "conversions"]);
  const errorMessage = useApiErrorMessage();
  const grid = useLogGrid();
  const logsQuery = usePostbackLogs(grid.params);
  const networksQuery = useNetworks();

  const networkNameById = React.useMemo(
    () => Object.fromEntries((networksQuery.data?.networks ?? []).map((n) => [n.id, n.name])),
    [networksQuery.data],
  );

  const columns = React.useMemo(() => postbackLogColumns(t, networkNameById), [t, networkNameById]);

  return (
    <LogGrid<PostbackLog>
      grid={grid}
      query={logsQuery}
      select={() => logsQuery.data && { rows: logsQuery.data.logs, total: logsQuery.data.total }}
      fetchPage={fetchPostbackLogPage}
      loadingLabel={t("logs.loading")}
      errorTitle={t("logs.loadError")}
      errorMessage={errorMessage}
      ariaLabel={t("logs.title")}
      columns={columns}
      getRowId={(row) => `${row.direction}:${row.networkId}:${row.clickId}:${row.status}:${row.eventRef ?? ""}:${row.attemptCount}:${row.eventAt}`}
      exportFilename="postback-logs"
      emptyTitle={t("logs.emptyTitle")}
      emptyDescription={t("logs.emptyDescription")}
    />
  );
}
