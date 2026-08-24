"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { listPostbackLogs, replayOutgoingPostback, type ListPostbackLogsParams } from "@/lib/api/postback-logs";

const postbackLogsKey = (params: ListPostbackLogsParams) => ["postback-logs", params] as const;

export function usePostbackLogs(params: ListPostbackLogsParams = {}) {
  return useQuery({ queryKey: postbackLogsKey(params), queryFn: () => listPostbackLogs(params) });
}

/** The replayed attempt only shows up in the log once apps/worker's
 * Deliverer actually dispatches it — invalidating here refreshes the list
 * to whatever's current, not to the replay's own eventual outcome. */
export function useReplayOutgoingPostback() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: replayOutgoingPostback,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["postback-logs"] }),
  });
}
