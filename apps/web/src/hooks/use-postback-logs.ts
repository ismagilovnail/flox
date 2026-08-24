"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  listPostbackLogs,
  replayIncomingPostback,
  replayOutgoingPostback,
  type ListPostbackLogsParams,
} from "@/lib/api/postback-logs";

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

/** Unlike outgoing replay, a successful incoming replay can itself insert
 * a brand-new attempt row synchronously (no worker poll to wait for) —
 * invalidating still just refreshes the list to whatever's current. */
export function useReplayIncomingPostback() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: replayIncomingPostback,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["postback-logs"] }),
  });
}
