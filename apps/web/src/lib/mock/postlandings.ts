/**
 * Postlandings (§28) — real, team-managed entities. `events` is a curated
 * subset of the full §43 event model (CLAUDE.md "EVENT MODEL QUICK REF") —
 * the engagement events a page shown after Landing/PWA/Offer can plausibly
 * fire. The full canonical event enum belongs to Phase 13 (Conversions/
 * Postbacks/Pixels); don't duplicate it here, just reference the same
 * string values so nothing has to be renamed when that phase lands. IDs
 * match the ones already baked into stream-set flows.
 */

export type PostlandingStatus = "active" | "paused" | "archived";

export const POSTLANDING_EVENT_TYPES = [
  "PWA_INSTALL",
  "NOTIFICATION_REQUEST",
  "NOTIFICATION_SUBSCRIBE",
  "NOTIFICATION_DECLINE",
  "TG_JOIN",
  "TG_START",
] as const;

export type PostlandingEventType = (typeof POSTLANDING_EVENT_TYPES)[number];

export type Postlanding = {
  id: string;
  name: string;
  url: string;
  events: PostlandingEventType[];
  status: PostlandingStatus;
  createdAt: string;
  updatedAt: string;
};

export const POSTLANDINGS: Postlanding[] = [
  {
    id: "psl_thankyou",
    name: "Thank You / Upsell",
    url: "https://cdn.floxlink.io/psl/thankyou",
    events: ["NOTIFICATION_REQUEST", "NOTIFICATION_SUBSCRIBE"],
    status: "active",
    createdAt: "2026-03-10T00:00:00Z",
    updatedAt: "2026-07-08T00:00:00Z",
  },
  {
    id: "psl_survey",
    name: "Post-install Survey",
    url: "https://cdn.floxlink.io/psl/survey",
    events: ["PWA_INSTALL", "TG_JOIN"],
    status: "active",
    createdAt: "2026-04-22T00:00:00Z",
    updatedAt: "2026-06-19T00:00:00Z",
  },
];
