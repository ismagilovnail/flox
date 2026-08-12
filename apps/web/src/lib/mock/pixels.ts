/**
 * Pixels (§29) — client-side ad-platform conversion pixels (Facebook, TikTok,
 * Snap, X, or a generic S2S receiver), fired on the events listed in
 * `events` so the ad platform can optimize delivery. Distinct from a Stream
 * Set's raw `pixels: string[]` S2S URLs (§23/§24) — those fire arbitrary
 * URLs per stream-set match; these are named, provider-typed pixels
 * selected per event.
 */

export type PixelProvider = "facebook" | "tiktok" | "snapchat" | "twitter" | "generic";

export const PIXEL_PROVIDERS: PixelProvider[] = ["facebook", "tiktok", "snapchat", "twitter", "generic"];

export const PIXEL_PROVIDER_LABELS: Record<PixelProvider, string> = {
  facebook: "Facebook Pixel",
  tiktok: "TikTok Pixel",
  snapchat: "Snap Pixel",
  twitter: "X (Twitter) Pixel",
  generic: "Generic S2S",
};

export type PixelStatus = "active" | "paused" | "archived";

/** Curated subset of the §43 event model that a conversion pixel plausibly fires on —
 * same rationale as mock/postlandings.ts's POSTLANDING_EVENT_TYPES, same string values. */
export const PIXEL_EVENT_TYPES = [
  "PWA_INSTALL",
  "CPA_HOLD",
  "CPA_ACCEPT",
  "CPA_REDEP",
  "TG_JOIN",
  "NOTIFICATION_SUBSCRIBE",
] as const;

export type PixelEventType = (typeof PIXEL_EVENT_TYPES)[number];

export type Pixel = {
  id: string;
  name: string;
  provider: PixelProvider;
  pixelId: string;
  events: PixelEventType[];
  status: PixelStatus;
  createdAt: string;
  updatedAt: string;
};

export const PIXELS: Pixel[] = [
  {
    id: "pxl_fb_sweeps",
    name: "Facebook — Sweeps conversions",
    provider: "facebook",
    pixelId: "1029384756102938",
    events: ["PWA_INSTALL", "CPA_ACCEPT"],
    status: "active",
    createdAt: "2026-03-05T00:00:00Z",
    updatedAt: "2026-07-20T00:00:00Z",
  },
  {
    id: "pxl_tt_nutra",
    name: "TikTok — Nutra leads",
    provider: "tiktok",
    pixelId: "CJKQR8BC77U5J8QFH0M0",
    events: ["CPA_HOLD"],
    status: "active",
    createdAt: "2026-04-01T00:00:00Z",
    updatedAt: "2026-06-18T00:00:00Z",
  },
  {
    id: "pxl_generic_dating",
    name: "Generic S2S — Dating",
    provider: "generic",
    pixelId: "s2s-dating-eu",
    events: ["CPA_ACCEPT", "CPA_REDEP"],
    status: "paused",
    createdAt: "2026-05-10T00:00:00Z",
    updatedAt: "2026-07-30T00:00:00Z",
  },
];
