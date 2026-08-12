/**
 * Placeholder pickers for the Flow Builder (§24-25). Landings/PWAs/
 * Postlandings become real, team-managed entities in Phase 12 — until then
 * these are small static lists so the funnel editor has something to bind
 * to. Swap for real entity queries when that phase lands; the Flow shape
 * (landingId/pwaId/postlandingId) doesn't change.
 *
 * Networks and Offers are real entities as of Phase 11 — see
 * stores/networks.ts and stores/offers.ts (backed by mock/networks.ts and
 * mock/offers.ts). Flow Builder components read those stores directly.
 */

export type PwaType = "internal" | "external" | "ios_app";
export const PWA_TYPES: PwaType[] = ["internal", "external", "ios_app"];

export type LandingOption = { id: string; name: string; url: string };
export const LANDINGS: LandingOption[] = [
  { id: "lnd_prelander_a", name: "Prelander A/B — Sweeps", url: "https://cdn.floxlink.io/lnd/prelander-a" },
  { id: "lnd_quiz", name: "Quiz Lander", url: "https://cdn.floxlink.io/lnd/quiz" },
  { id: "lnd_advertorial", name: "Advertorial", url: "https://cdn.floxlink.io/lnd/advertorial" },
];

export type PwaOption = { id: string; name: string };
export const PWAS: PwaOption[] = [
  { id: "pwa_sweeps", name: "Sweeps PWA" },
  { id: "pwa_casino", name: "Casino Lite PWA" },
];

export type PostlandingOption = { id: string; name: string; url: string };
export const POSTLANDINGS: PostlandingOption[] = [
  { id: "psl_thankyou", name: "Thank You / Upsell", url: "https://cdn.floxlink.io/psl/thankyou" },
  { id: "psl_survey", name: "Post-install Survey", url: "https://cdn.floxlink.io/psl/survey" },
];
