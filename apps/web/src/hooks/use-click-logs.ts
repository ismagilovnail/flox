"use client";

import { useQuery } from "@tanstack/react-query";

import { listClickLogs, type ListClickLogsParams } from "@/lib/api/click-logs";

const clickLogsKey = (params: ListClickLogsParams) => ["click-log", params] as const;

export function useClickLogs(params: ListClickLogsParams = {}) {
  return useQuery({ queryKey: clickLogsKey(params), queryFn: () => listClickLogs(params) });
}
