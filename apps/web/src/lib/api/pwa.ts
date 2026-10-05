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
  /** Follow the visitor's device light/dark theme on the install page. */
  themeAuto: boolean;
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
  /** Detail-sheet fields (the listing's "detailed information" table).
   * Free text; "" means never entered and the UI hides that row. */
  version: string;
  ageRating: string;
  category: string;
  interactiveElements: string;
  /** Replaces what used to be a hardcoded "no ads or paid content"
   * claim in the preview. */
  containsAds: boolean;
  inAppPurchases: boolean;
  /** The listing's own "updated on" date as YYYY-MM-DD; undefined when
   * never entered (the preview then hides the row instead of printing
   * today's date, which is what it used to do). */
  updatedDate?: string;
  status: PwaStatus;
  createdAt: string;
  updatedAt: string;
};

export type CreatePwaInput = {
  name: string;
  shortName: string;
  themeColor: string;
  backgroundColor: string;
  /** Follow the visitor's device light/dark theme on the install page. */
  themeAuto: boolean;
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
  version: string;
  ageRating: string;
  category: string;
  interactiveElements: string;
  containsAds: boolean;
  inAppPurchases: boolean;
  updatedDate?: string;
};

export type UpdatePwaInput = Partial<CreatePwaInput> & { status?: PwaStatus };

export function listPwas(): Promise<{ pwas: Pwa[] }> {
  return apiFetch("/pwas");
}

/** A signed apps/tracker URL for this PWA's real install page, marked as a
 * test — served exactly as a visitor gets it, but no events are recorded.
 * Reflects the saved PWA, not unsaved form edits. */
export function getPwaTestLink(id: string): Promise<{ url: string }> {
  return apiFetch(`/pwas/${id}/test-link`, { method: "POST" });
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
