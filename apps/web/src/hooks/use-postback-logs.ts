"use client";

import { useQuery } from "@tanstack/react-query";

import { listPostbackLogs, type ListPostbackLogsParams } from "@/lib/api/postback-logs";

const postbackLogsKey = (params: ListPostbackLogsParams) => ["postback-logs", params] as const;

export function usePostbackLogs(params: ListPostbackLogsParams = {}) {
  return useQuery({ queryKey: postbackLogsKey(params), queryFn: () => listPostbackLogs(params) });
}
