"use client";

import { useQuery } from "@tanstack/react-query";

import { listBuyers } from "@/lib/api/analytics";

/** The members who own campaigns, for buyer labels (audit TEAM-1). */
export function useBuyerNames() {
  return useQuery({ queryKey: ["analytics", "buyers"], queryFn: listBuyers, staleTime: 60_000 });
}
