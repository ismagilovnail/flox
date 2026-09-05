/**
 * Tags (§30.6) — a color-label system spanning Campaigns, CPA Networks,
 * Offers, Flows, Traffic Sources, PWA apps, and Landing Pages. Exactly
 * those seven — Postlanding/Pixels/Conversions/Domains/Team are NOT
 * taggable per spec. Real domain vocabulary (not mock data — see
 * apps/internal/tags for the real backend, lib/api/tags.ts for the
 * client), so it lives here rather than under lib/mock/.
 */

export type TaggableEntityType = "campaign" | "network" | "offer" | "flow" | "traffic_source" | "pwa" | "landing";

export const TAGGABLE_ENTITY_LABELS: Record<TaggableEntityType, string> = {
  campaign: "Campaign",
  network: "Network",
  offer: "Offer",
  flow: "Flow",
  traffic_source: "Traffic Source",
  pwa: "PWA",
  landing: "Landing",
};

/**
 * A small fixed decorative palette, deliberately separate from the design
 * system's semantic success/warning/danger/info tokens — a user-created
 * tag being "green" must never read as a status badge meaning "success".
 * Mirrors apps/internal/tags' own `Colors` whitelist exactly.
 */
export const TAG_COLORS = [
  { id: "red", hex: "#ef4444" },
  { id: "orange", hex: "#f97316" },
  { id: "amber", hex: "#f59e0b" },
  { id: "green", hex: "#22c55e" },
  { id: "teal", hex: "#14b8a6" },
  { id: "blue", hex: "#3b82f6" },
  { id: "indigo", hex: "#6366f1" },
  { id: "purple", hex: "#a855f7" },
  { id: "pink", hex: "#ec4899" },
  { id: "gray", hex: "#6b7280" },
] as const;

export type TagColorId = (typeof TAG_COLORS)[number]["id"];

export function tagColorHex(colorId: TagColorId): string {
  return TAG_COLORS.find((c) => c.id === colorId)?.hex ?? TAG_COLORS[0].hex;
}

export function randomTagColor(): TagColorId {
  return TAG_COLORS[Math.floor(Math.random() * TAG_COLORS.length)].id;
}

export type Tag = {
  id: string;
  name: string;
  color: TagColorId;
  createdAt: string;
  updatedAt: string;
};

export type TagAssignment = {
  tagId: string;
  entityType: TaggableEntityType;
  entityId: string;
};
