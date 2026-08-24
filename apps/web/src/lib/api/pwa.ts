import { apiFetch } from "@/lib/api/client";

/** Mirrors apps/internal/pwa's real shape. */
export type PwaStatus = "active" | "paused" | "archived";

export type Pwa = {
  id: string;
  organizationId: string;
  name: string;
  shortName: string;
  themeColor: string;
  backgroundColor: string;
  iconUrl: string;
  startUrl: string;
  bounceInAppWebview: boolean;
  status: PwaStatus;
  createdAt: string;
  updatedAt: string;
};

export type CreatePwaInput = {
  name: string;
  shortName: string;
  themeColor: string;
  backgroundColor: string;
  iconUrl: string;
  startUrl: string;
  bounceInAppWebview: boolean;
};

export type UpdatePwaInput = Partial<CreatePwaInput> & { status?: PwaStatus };

export function listPwas(): Promise<{ pwas: Pwa[] }> {
  return apiFetch("/pwas");
}

export function getPwa(id: string): Promise<Pwa> {
  return apiFetch(`/pwas/${id}`);
}

export function createPwa(input: CreatePwaInput): Promise<Pwa> {
  return apiFetch("/pwas", { method: "POST", body: input });
}

export function updatePwa(id: string, input: UpdatePwaInput): Promise<Pwa> {
  return apiFetch(`/pwas/${id}`, { method: "PATCH", body: input });
}

export function deletePwa(id: string): Promise<void> {
  return apiFetch(`/pwas/${id}`, { method: "DELETE" });
}

export function duplicatePwa(id: string): Promise<Pwa> {
  return apiFetch(`/pwas/${id}/duplicate`, { method: "POST" });
}

export function pausePwa(id: string): Promise<Pwa> {
  return apiFetch(`/pwas/${id}/pause`, { method: "POST" });
}

export function activatePwa(id: string): Promise<Pwa> {
  return apiFetch(`/pwas/${id}/activate`, { method: "POST" });
}

export function archivePwa(id: string): Promise<Pwa> {
  return apiFetch(`/pwas/${id}`, { method: "PATCH", body: { status: "archived" } });
}
