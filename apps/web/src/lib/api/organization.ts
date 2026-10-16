import { API_VERSION_PREFIX, apiFetch, apiUrl } from "@/lib/api/client";
import type { OrganizationSettings } from "@/lib/organization";

export function getOrganization(): Promise<OrganizationSettings> {
  return apiFetch("/organization");
}

export function updateOrganization(input: Partial<Pick<OrganizationSettings, "name" | "timezone" | "currency">>): Promise<OrganizationSettings> {
  return apiFetch("/organization", { method: "PATCH", body: input });
}

/** A scheduled deletion of the organization (internal/orgdeletion, audit
 * GDPR-1): `scheduledAt` null when none. */
export type OrganizationDeletion = {
  scheduledAt: string | null;
  requestedAt: string | null;
  requestedBy: string;
};

export function getOrganizationDeletion(): Promise<OrganizationDeletion> {
  return apiFetch("/organization/deletion");
}

/** Owner only, confirmed with the organization's exact name and the
 * owner's password; the deletion happens 7 days later. */
export function scheduleOrganizationDeletion(input: { confirmName: string; password: string }): Promise<OrganizationDeletion> {
  return apiFetch("/organization/deletion", { method: "POST", body: input });
}

export function cancelOrganizationDeletion(): Promise<OrganizationDeletion> {
  return apiFetch("/organization/deletion", { method: "DELETE" });
}

/** The organization's data export (internal/orgexport, audit GDPR-1). */
export type OrganizationExport = {
  id: string;
  status: "pending" | "running" | "done" | "failed" | "expired";
  from: string;
  to: string;
  sizeBytes: number;
  error: string;
  createdAt: string;
  finishedAt: string | null;
  expiresAt: string | null;
  requestedBy: string;
};

export function listOrganizationExports(): Promise<{ exports: OrganizationExport[] }> {
  return apiFetch("/organization/exports");
}

export function requestOrganizationExport(input: { from: string; to: string }): Promise<OrganizationExport> {
  return apiFetch("/organization/exports", { method: "POST", body: input });
}

/** The archive streams from the API with the session cookie — a plain
 * link, so the browser downloads it rather than the app holding it in
 * memory. */
export function organizationExportDownloadUrl(id: string): string {
  return apiUrl(`${API_VERSION_PREFIX}/organization/exports/${id}/download`);
}
