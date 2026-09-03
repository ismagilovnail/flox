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
  /** Store-listing preview fields (features/pwa/pwa-preview-card.tsx) —
   * all optional. rating/ratingCount/sizeMb are undefined together when
   * never entered, distinct from 0 (CLAUDE.md #6's "never fabricate a
   * zero" applied to this listing preview, not just cost/ROI). */
  developer: string;
  description: string;
  rating?: number;
  ratingCount?: number;
  downloadsLabel: string;
  sizeMb?: number;
  /** Ordered gallery image URLs, and the per-star breakdown percentages
   * — each independently optional, same "undefined stays undefined, not
   * a fabricated 0" rule as rating/ratingCount/sizeMb above. */
  screenshots: string[];
  whatsNew: string;
  ratingStars5?: number;
  ratingStars4?: number;
  ratingStars3?: number;
  ratingStars2?: number;
  ratingStars1?: number;
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
  developer: string;
  description: string;
  rating?: number;
  ratingCount?: number;
  downloadsLabel: string;
  sizeMb?: number;
  screenshots: string[];
  whatsNew: string;
  ratingStars5?: number;
  ratingStars4?: number;
  ratingStars3?: number;
  ratingStars2?: number;
  ratingStars1?: number;
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
