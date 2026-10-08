"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createAlertRule,
  deleteAlertRule,
  listAlertRules,
  listNotifications,
  markNotificationsRead,
  updateAlertRule,
} from "@/lib/api/alerts";
import type { AlertRuleInput } from "@/lib/alerts";

const rulesKey = ["alert-rules"] as const;
const notificationsKey = ["notifications"] as const;

/** The worker evaluates rules every two minutes; polling once a minute
 * shows a new notification within that. */
const NOTIFICATIONS_POLL_MS = 60_000;

export function useAlertRules() {
  return useQuery({ queryKey: rulesKey, queryFn: listAlertRules });
}

export function useSaveAlertRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id?: string; input: AlertRuleInput }) =>
      id ? updateAlertRule(id, input) : createAlertRule(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: rulesKey }),
  });
}

export function useDeleteAlertRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteAlertRule(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: rulesKey }),
  });
}

export function useNotifications() {
  return useQuery({ queryKey: notificationsKey, queryFn: listNotifications, refetchInterval: NOTIFICATIONS_POLL_MS });
}

export function useMarkNotificationsRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: markNotificationsRead,
    onSuccess: () => qc.invalidateQueries({ queryKey: notificationsKey }),
  });
}
