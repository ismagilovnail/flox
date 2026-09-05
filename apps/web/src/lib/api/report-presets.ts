import { apiFetch } from "@/lib/api/client";
import type { ReportPreset, ReportPresetInput } from "@/lib/report-presets";

export function listReportPresets(): Promise<{ presets: ReportPreset[] }> {
  return apiFetch("/report-presets");
}

export function createReportPreset(input: ReportPresetInput): Promise<ReportPreset> {
  return apiFetch("/report-presets", { method: "POST", body: input });
}

export function updateReportPreset(id: string, input: ReportPresetInput): Promise<ReportPreset> {
  return apiFetch(`/report-presets/${id}`, { method: "PATCH", body: input });
}

export function deleteReportPreset(id: string): Promise<void> {
  return apiFetch(`/report-presets/${id}`, { method: "DELETE" });
}
