"use client";

import * as React from "react";
import { toast } from "sonner";

import { DataTable } from "@/components/ui/data-table";
import { usePostbackLogsStore } from "@/stores/postback-logs";
import { useNetworksStore } from "@/stores/networks";
import { postbackLogColumns } from "@/features/postbacks/postback-log-columns";

export function PostbackLogsPanel() {
  const logs = usePostbackLogsStore((s) => s.logs);
  const replay = usePostbackLogsStore((s) => s.replay);
  const networks = useNetworksStore((s) => s.networks);

  const networkNameById = React.useMemo(
    () => Object.fromEntries(networks.map((n) => [n.id, n.name])),
    [networks],
  );

  const columns = React.useMemo(
    () =>
      postbackLogColumns(networkNameById, (log) => {
        replay(log.id);
        toast("Postback replayed", { description: log.clickId });
      }),
    [networkNameById, replay],
  );

  return (
    <DataTable
      columns={columns}
      data={logs}
      searchPlaceholder="Search by click ID..."
      emptyTitle="No postback activity yet"
      emptyDescription="Every incoming and outgoing postback attempt is logged here, success or not."
      pageSize={15}
    />
  );
}
