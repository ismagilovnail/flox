/**
 * Tags (§30.6) — a color-label system spanning Campaigns, CPA Networks,
 * Offers, Flows, Traffic Sources, PWA apps, and Landing Pages. Exactly
 * those seven — Postlanding/Pixels/Conversions/Domains/Team are NOT
 * taggable per spec.
 *
 * Data model mirrors the spec's generic `tags` table + polymorphic
 * `taggables` join (tag_id, entity_type, entity_id, organization_id) —
 * see mock/tag-assignments.ts for the join side. `organization_id` is
 * implicit here (single mock workspace); real tenant scoping is
 * §36-TENANCY, enforced once the backend exists.
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

export const TAGS: Tag[] = [
  { id: "tag_top_performer", name: "Top Performer", color: "green", createdAt: "2026-03-01T00:00:00Z", updatedAt: "2026-03-01T00:00:00Z" },
  { id: "tag_needs_review", name: "Needs Review", color: "amber", createdAt: "2026-03-01T00:00:00Z", updatedAt: "2026-03-01T00:00:00Z" },
  { id: "tag_q3_push", name: "Q3 Push", color: "blue", createdAt: "2026-06-15T00:00:00Z", updatedAt: "2026-06-15T00:00:00Z" },
  { id: "tag_deprecated", name: "Deprecated", color: "gray", createdAt: "2026-04-10T00:00:00Z", updatedAt: "2026-04-10T00:00:00Z" },
  { id: "tag_high_priority", name: "High Priority", color: "red", createdAt: "2026-02-20T00:00:00Z", updatedAt: "2026-02-20T00:00:00Z" },
];
