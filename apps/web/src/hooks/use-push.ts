"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  countPushAudience,
  createPushBroadcast,
  getPushSummary,
  listPushBroadcasts,
  listPushSubscriptions,
  type CreateBroadcastInput,
  type PushFilters,
} from "@/lib/api/push";

const pushKey = ["push"] as const;

export function usePushSummary() {
  return useQuery({ queryKey: [...pushKey, "summary"], queryFn: getPushSummary });
}

export function usePushSubscriptions() {
  return useQuery({ queryKey: [...pushKey, "subscriptions"], queryFn: listPushSubscriptions });
}

/** Broadcasts refresh while the page is open: the counters move as the
 * worker sends and as notifications are clicked. */
export function usePushBroadcasts() {
  return useQuery({ queryKey: [...pushKey, "broadcasts"], queryFn: listPushBroadcasts, refetchInterval: 15_000 });
}

export function usePushAudience(filters: PushFilters, enabled: boolean) {
  return useQuery({
    queryKey: [...pushKey, "audience", filters],
    queryFn: () => countPushAudience(filters),
    enabled,
  });
}

export function useCreatePushBroadcast() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateBroadcastInput) => createPushBroadcast(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: pushKey }),
  });
}
