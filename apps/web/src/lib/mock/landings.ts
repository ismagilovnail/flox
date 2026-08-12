/**
 * Landings (§28) — real, team-managed entities. IDs match the ones already
 * baked into stream-set flows (see mock/stream-sets.ts) so existing flow
 * landing steps keep resolving.
 */

export type LandingType = "internal" | "external";
export type LandingStatus = "active" | "paused" | "archived";

export type Landing = {
  id: string;
  name: string;
  type: LandingType;
  /** Resolved URL — hosted on our CDN for `internal`, the advertiser's own URL for `external`. */
  url: string;
  /** Page copy/HTML for `internal` landings only; empty for `external`. */
  content: string;
  status: LandingStatus;
  createdAt: string;
  updatedAt: string;
};

export const LANDINGS: Landing[] = [
  {
    id: "lnd_prelander_a",
    name: "Prelander A/B — Sweeps",
    type: "internal",
    url: "https://cdn.floxlink.io/lnd/prelander-a",
    content: "<h1>You've been selected!</h1><p>Complete a short quiz to claim your entry.</p>",
    status: "active",
    createdAt: "2026-02-12T00:00:00Z",
    updatedAt: "2026-07-22T00:00:00Z",
  },
  {
    id: "lnd_quiz",
    name: "Quiz Lander",
    type: "internal",
    url: "https://cdn.floxlink.io/lnd/quiz",
    content: "<h1>What's your dream vacation?</h1><p>Answer 3 questions to see your match.</p>",
    status: "active",
    createdAt: "2026-03-01T00:00:00Z",
    updatedAt: "2026-06-15T00:00:00Z",
  },
  {
    id: "lnd_advertorial",
    name: "Advertorial",
    type: "external",
    url: "https://advertorial-partner.example/story/sweeps-2026",
    content: "",
    status: "active",
    createdAt: "2026-04-08T00:00:00Z",
    updatedAt: "2026-07-30T00:00:00Z",
  },
];
