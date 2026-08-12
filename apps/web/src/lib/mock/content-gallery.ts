/**
 * Content Gallery (§30.8) — a browsable library of ready-to-use content:
 * landing/PWA/postlanding templates and creative assets. System items ship
 * with the platform (read-only); team items are uploaded by the workspace
 * and private to it (tenant-scoped, like every other §36-TENANCY surface).
 *
 * First-pass scope: templates carry a hand-off payload shaped exactly like
 * the target builder's form `defaultValues` (see landing/pwa/postlanding
 * form sheets), so "Use this" is a real pre-filled create flow, not a
 * simulated one. There is no asset library builder yet (Phase 12 never
 * built one), so a creative asset's "Use this" copies its hosted URL —
 * the same "URL stands in for upload" convention already used for PWA
 * icons — rather than inventing a second, parallel asset system.
 *
 * No real object storage exists in this frontend-first phase, so a team
 * "upload" records a hosted URL the team already has (identical in spirit
 * to how a PWA's icon is just a URL field) rather than performing a real
 * S3 upload — that's real Phase 27 (integration) work, not this phase's.
 */

import type { PostlandingEventType } from "@/lib/mock/postlandings";

export type GalleryCategory = "landing_template" | "pwa_template" | "postlanding_template" | "creative_asset";
export type GallerySource = "system" | "team";

export const GALLERY_CATEGORY_LABELS: Record<GalleryCategory, string> = {
  landing_template: "Landing template",
  pwa_template: "PWA template",
  postlanding_template: "Postlanding template",
  creative_asset: "Creative asset",
};

/** Decorative preview-tile colors — deliberately not real hosted images, so the
 * mock never pretends to be a real CDN/asset pipeline (CLAUDE.md: no fake APIs
 * that look real). */
export const GALLERY_PREVIEW_COLORS = ["#3b82f6", "#22c55e", "#f97316", "#a855f7", "#14b8a6", "#ec4899"] as const;

export type GalleryItem = {
  id: string;
  title: string;
  description: string;
  category: GalleryCategory;
  source: GallerySource;
  previewColor: (typeof GALLERY_PREVIEW_COLORS)[number];
  tags: string[];
  createdAt: string;
  /** Only set for source: "team". */
  uploadedByMemberId?: string;

  landingPayload?: { type: "internal" | "external"; content?: string; url?: string };
  pwaPayload?: {
    shortName: string;
    themeColor: string;
    backgroundColor: string;
    iconUrl: string;
    startUrl: string;
    bounceInAppWebview: boolean;
  };
  postlandingPayload?: { url: string; events: PostlandingEventType[] };
  assetPayload?: { fileType: "image" | "video" | "zip"; fileUrl: string; fileSizeKb: number };
};

export const CONTENT_GALLERY_ITEMS: GalleryItem[] = [
  {
    id: "gal_lnd_quiz",
    title: "Sweepstakes Quiz Lander",
    description: "A 3-question quiz flow that funnels into a CTA button — the highest-converting internal lander shape for sweeps offers.",
    category: "landing_template",
    source: "system",
    previewColor: "#3b82f6",
    tags: ["quiz", "sweepstakes", "high-cvr"],
    createdAt: "2026-01-10T00:00:00Z",
    landingPayload: {
      type: "internal",
      content: "<h1>Win a $500 Gift Card!</h1><p>Answer 3 quick questions to enter.</p><button>Start Quiz</button>",
    },
  },
  {
    id: "gal_lnd_review",
    title: "Product Review Lander",
    description: "Editorial-style review page with a star rating block and a below-the-fold offer CTA.",
    category: "landing_template",
    source: "system",
    previewColor: "#22c55e",
    tags: ["review", "editorial", "nutra"],
    createdAt: "2026-01-14T00:00:00Z",
    landingPayload: {
      type: "internal",
      content: "<h1>Our Verdict: 4.8/5 Stars</h1><p>We tested it for 30 days. Here's what happened.</p>",
    },
  },
  {
    id: "gal_lnd_advertiser",
    title: "Advertiser-Hosted Passthrough",
    description: "A pass-through starting point for offers where the advertiser already hosts the landing page.",
    category: "landing_template",
    source: "system",
    previewColor: "#f97316",
    tags: ["passthrough", "external"],
    createdAt: "2026-01-20T00:00:00Z",
    landingPayload: { type: "external", url: "https://advertiser.example/landing" },
  },
  {
    id: "gal_pwa_gaming",
    title: "Casino/Gaming PWA Shell",
    description: "Dark-themed PWA manifest tuned for casino and gaming verticals — gold accent, splash-friendly icon slot.",
    category: "pwa_template",
    source: "system",
    previewColor: "#a855f7",
    tags: ["casino", "gaming", "dark"],
    createdAt: "2026-02-02T00:00:00Z",
    pwaPayload: {
      shortName: "Lucky7",
      themeColor: "#7c2d12",
      backgroundColor: "#0f0f0f",
      iconUrl: "https://cdn.floxlink.io/gallery/icons/gaming-icon.png",
      startUrl: "/play",
      bounceInAppWebview: true,
    },
  },
  {
    id: "gal_pwa_ecom",
    title: "E-commerce PWA Shell",
    description: "Light, brand-neutral storefront PWA shell — clean checkout-friendly palette.",
    category: "pwa_template",
    source: "system",
    previewColor: "#14b8a6",
    tags: ["ecommerce", "light"],
    createdAt: "2026-02-05T00:00:00Z",
    pwaPayload: {
      shortName: "Shop",
      themeColor: "#0f766e",
      backgroundColor: "#ffffff",
      iconUrl: "https://cdn.floxlink.io/gallery/icons/ecom-icon.png",
      startUrl: "/shop",
      bounceInAppWebview: true,
    },
  },
  {
    id: "gal_post_notif",
    title: "Push Opt-in Postlanding",
    description: "A minimal engagement page whose only job is to trigger the notification permission prompt.",
    category: "postlanding_template",
    source: "system",
    previewColor: "#3b82f6",
    tags: ["push", "notifications"],
    createdAt: "2026-02-10T00:00:00Z",
    postlandingPayload: {
      url: "https://advertiser.example/postlanding/push-optin",
      events: ["NOTIFICATION_REQUEST", "NOTIFICATION_SUBSCRIBE", "NOTIFICATION_DECLINE"],
    },
  },
  {
    id: "gal_post_tg",
    title: "Telegram Bridge Postlanding",
    description: "Bridges from PWA install straight into a Telegram bot join/start flow.",
    category: "postlanding_template",
    source: "system",
    previewColor: "#22c55e",
    tags: ["telegram", "bridge"],
    createdAt: "2026-02-12T00:00:00Z",
    postlandingPayload: {
      url: "https://advertiser.example/postlanding/tg-bridge",
      events: ["TG_JOIN", "TG_START"],
    },
  },
  {
    id: "gal_asset_banner_300x250",
    title: "Sweeps Banner Pack (300x250)",
    description: "Medium-rectangle display banner set for sweepstakes campaigns, 4 color variants.",
    category: "creative_asset",
    source: "system",
    previewColor: "#ec4899",
    tags: ["banner", "display", "sweepstakes"],
    createdAt: "2026-03-01T00:00:00Z",
    assetPayload: { fileType: "image", fileUrl: "https://cdn.floxlink.io/gallery/assets/sweeps-banner-300x250.zip", fileSizeKb: 842 },
  },
  {
    id: "gal_asset_video_bg",
    title: "Casino Loop Background (MP4)",
    description: "A 10s seamless-loop video background for casino landers and PWA splash screens.",
    category: "creative_asset",
    source: "system",
    previewColor: "#a855f7",
    tags: ["video", "casino", "background"],
    createdAt: "2026-03-04T00:00:00Z",
    assetPayload: { fileType: "video", fileUrl: "https://cdn.floxlink.io/gallery/assets/casino-loop-bg.mp4", fileSizeKb: 4_120 },
  },
  {
    id: "gal_asset_icon_pack",
    title: "App Icon Pack (12 sizes)",
    description: "A full manifest-ready icon set, pre-sized for PWA and iOS home-screen install.",
    category: "creative_asset",
    source: "system",
    previewColor: "#f97316",
    tags: ["icons", "pwa", "ios"],
    createdAt: "2026-03-06T00:00:00Z",
    assetPayload: { fileType: "zip", fileUrl: "https://cdn.floxlink.io/gallery/assets/app-icon-pack.zip", fileSizeKb: 216 },
  },
  {
    id: "gal_team_banner_q3",
    title: "Q3 Promo Banner",
    description: "In-house banner set for the Q3 push campaigns.",
    category: "creative_asset",
    source: "team",
    previewColor: "#3b82f6",
    tags: ["banner", "q3", "in-house"],
    createdAt: "2026-06-20T00:00:00Z",
    uploadedByMemberId: "mem_owner",
    assetPayload: { fileType: "image", fileUrl: "https://assets.example-team.com/q3-promo-banner.png", fileSizeKb: 310 },
  },
  {
    id: "gal_team_teaser_video",
    title: "Product Teaser (15s)",
    description: "Short-form teaser cut for paid social placements.",
    category: "creative_asset",
    source: "team",
    previewColor: "#22c55e",
    tags: ["video", "social"],
    createdAt: "2026-07-02T00:00:00Z",
    uploadedByMemberId: "mem_manager",
    assetPayload: { fileType: "video", fileUrl: "https://assets.example-team.com/teaser-15s.mp4", fileSizeKb: 2_050 },
  },
];
