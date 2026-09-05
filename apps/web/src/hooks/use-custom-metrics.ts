"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createCustomMetric,
  deleteCustomMetric,
  duplicateCustomMetric,
  listCustomMetrics,
  setCustomMetricActive,
  updateCustomMetric,
  type CustomMetricInput,
} from "@/lib/api/custom-metrics";
import type { CustomMetric } from "@/lib/custom-metrics";

const metricsKey = ["custom-metrics"] as const;

export const EMPTY_CUSTOM_METRICS: CustomMetric[] = [];

export function useCustomMetrics() {
  return useQuery({ queryKey: metricsKey, queryFn: listCustomMetrics });
}

function useInvalidateCustomMetrics() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: metricsKey });
}

export function useCreateCustomMetric() {
  const invalidate = useInvalidateCustomMetrics();
  return useMutation({
    mutationFn: (input: CustomMetricInput) => createCustomMetric(input),
    onSuccess: invalidate,
  });
}

export function useUpdateCustomMetric() {
  const invalidate = useInvalidateCustomMetrics();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: Partial<CustomMetricInput> }) => updateCustomMetric(id, input),
    onSuccess: invalidate,
  });
}

export function useDeleteCustomMetric() {
  const invalidate = useInvalidateCustomMetrics();
  return useMutation({
    mutationFn: (id: string) => deleteCustomMetric(id),
    onSuccess: invalidate,
  });
}

export function useDuplicateCustomMetric() {
  const invalidate = useInvalidateCustomMetrics();
  return useMutation({
    mutationFn: (id: string) => duplicateCustomMetric(id),
    onSuccess: invalidate,
  });
}

export function useSetCustomMetricActive() {
  const invalidate = useInvalidateCustomMetrics();
  return useMutation({
    mutationFn: ({ id, active }: { id: string; active: boolean }) => setCustomMetricActive(id, active),
    onSuccess: invalidate,
  });
}
