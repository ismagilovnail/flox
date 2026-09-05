"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createReportPreset,
  deleteReportPreset,
  listReportPresets,
  updateReportPreset,
} from "@/lib/api/report-presets";
import type { ReportPreset, ReportPresetInput } from "@/lib/report-presets";

const presetsKey = ["report-presets"] as const;

export const EMPTY_REPORT_PRESETS: ReportPreset[] = [];

export function useReportPresets() {
  return useQuery({ queryKey: presetsKey, queryFn: listReportPresets });
}

function useInvalidateReportPresets() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: presetsKey });
}

export function useCreateReportPreset() {
  const invalidate = useInvalidateReportPresets();
  return useMutation({
    mutationFn: (input: ReportPresetInput) => createReportPreset(input),
    onSuccess: invalidate,
  });
}

export function useUpdateReportPreset() {
  const invalidate = useInvalidateReportPresets();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: ReportPresetInput }) => updateReportPreset(id, input),
    onSuccess: invalidate,
  });
}

export function useDeleteReportPreset() {
  const invalidate = useInvalidateReportPresets();
  return useMutation({
    mutationFn: (id: string) => deleteReportPreset(id),
    onSuccess: invalidate,
  });
}
