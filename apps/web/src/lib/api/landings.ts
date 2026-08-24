import { apiFetch } from "@/lib/api/client";

/** Mirrors apps/internal/landing's real shape. */
export type LandingType = "internal" | "external";
export type LandingStatus = "active" | "paused" | "archived";

export type Landing = {
  id: string;
  organizationId: string;
  name: string;
  type: LandingType;
  /** Resolved URL — hosted on our CDN for `internal` (server-computed from
   * `name`, never trusts a client value for this type), the advertiser's
   * own URL for `external`. */
  url: string;
  /** Page copy/HTML for `internal` landings only; empty for `external`. */
  content: string;
  status: LandingStatus;
  createdAt: string;
  updatedAt: string;
};

export type CreateLandingInput = {
  name: string;
  type: LandingType;
  /** `external` only — ignored (recomputed server-side) for `internal`. */
  url?: string;
  /** `internal` only. */
  content?: string;
};

export type UpdateLandingInput = Partial<CreateLandingInput> & { status?: LandingStatus };

export function listLandings(): Promise<{ landings: Landing[] }> {
  return apiFetch("/landings");
}

export function getLanding(id: string): Promise<Landing> {
  return apiFetch(`/landings/${id}`);
}

export function createLanding(input: CreateLandingInput): Promise<Landing> {
  return apiFetch("/landings", { method: "POST", body: input });
}

export function updateLanding(id: string, input: UpdateLandingInput): Promise<Landing> {
  return apiFetch(`/landings/${id}`, { method: "PATCH", body: input });
}

export function deleteLanding(id: string): Promise<void> {
  return apiFetch(`/landings/${id}`, { method: "DELETE" });
}

export function duplicateLanding(id: string): Promise<Landing> {
  return apiFetch(`/landings/${id}/duplicate`, { method: "POST" });
}

export function pauseLanding(id: string): Promise<Landing> {
  return apiFetch(`/landings/${id}/pause`, { method: "POST" });
}

export function activateLanding(id: string): Promise<Landing> {
  return apiFetch(`/landings/${id}/activate`, { method: "POST" });
}

export function archiveLanding(id: string): Promise<Landing> {
  return apiFetch(`/landings/${id}`, { method: "PATCH", body: { status: "archived" } });
}
