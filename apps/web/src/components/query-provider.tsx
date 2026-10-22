"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";

import { ApiError } from "@/lib/api/client";

/** How many times a failed query is tried again. A 4xx is the request
 * itself (validation, access, a report too heavy for the server's limits)
 * and fails the same way every time: retrying it only repeats the work —
 * for query_too_heavy, three more heavy ClickHouse queries before the
 * error shows. Except reports_busy: the organization's other reports were
 * holding its ClickHouse slots (audit CH-1), which frees up in seconds.
 * Anything else (network, 5xx) gets React Query's default three retries,
 * with its growing delay. */
export function shouldRetry(failureCount: number, error: Error): boolean {
  if (error instanceof ApiError && error.code === "reports_busy") return failureCount < 3;
  if (error instanceof ApiError && error.status >= 400 && error.status < 500) return false;
  return failureCount < 3;
}

export function QueryProvider({ children }: { children: React.ReactNode }) {
  // Created once per component instance (not module scope) — a module-level
  // singleton would leak cached data across users on the server; useState's
  // lazy initializer keeps this stable across client re-renders instead.
  const [client] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: shouldRetry } } }));
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
