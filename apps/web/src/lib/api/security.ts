import { apiFetch } from "@/lib/api/client";
import type { SecuritySettings } from "@/lib/security";

export function getSecuritySettings(): Promise<SecuritySettings> {
  return apiFetch("/security");
}

export function updateSecuritySettings(input: Partial<SecuritySettings>): Promise<SecuritySettings> {
  return apiFetch("/security", { method: "PATCH", body: input });
}
