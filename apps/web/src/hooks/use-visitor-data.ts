"use client";

import { useMutation, useQuery } from "@tanstack/react-query";

import { eraseVisitor, findVisitor } from "@/lib/api/visitor-data";

/** Looks a visitor up once a query is set; null queries nothing. */
export function useVisitorReport(query: { clickId?: string; ip?: string } | null) {
  return useQuery({
    queryKey: ["visitor-data", query],
    queryFn: () => findVisitor(query ?? {}),
    enabled: query !== null,
    retry: false,
  });
}

export function useEraseVisitor() {
  return useMutation({ mutationFn: (clickIds: string[]) => eraseVisitor(clickIds) });
}
