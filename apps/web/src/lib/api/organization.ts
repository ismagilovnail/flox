import { apiFetch } from "@/lib/api/client";
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
