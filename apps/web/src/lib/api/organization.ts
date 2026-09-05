import { apiFetch } from "@/lib/api/client";
import type { OrganizationSettings } from "@/lib/organization";

export function getOrganization(): Promise<OrganizationSettings> {
  return apiFetch("/organization");
}

export function updateOrganization(input: Partial<Pick<OrganizationSettings, "name" | "timezone" | "currency">>): Promise<OrganizationSettings> {
  return apiFetch("/organization", { method: "PATCH", body: input });
}
