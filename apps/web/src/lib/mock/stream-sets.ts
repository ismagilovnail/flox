import { genId } from "@/lib/id";
import { emptyGroup, type FilterGroupNode } from "@/lib/filters";
import { LANDINGS, OFFERS, PWAS, type PwaType } from "@/lib/mock/flow-entities";

function mulberry32(seed: number) {
  return function rand() {
    seed |= 0;
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export type StreamSetStatus = "active" | "paused";

export type LandingStage = { enabled: boolean; landingId: string; asPwa: boolean };
export type PwaStage = { enabled: boolean; pwaId: string; pwaType: PwaType };
export type PostlandingStage = { enabled: boolean; postlandingId: string };

/** Terminal step of a flow — always one or the other (§25: Offer / Redirect
 * node types). Offer carries CPA/network attribution; Redirect is a plain
 * URL with none. */
export type Destination =
  | { kind: "offer"; networkId: string; offerId: string; offerUrl: string }
  | { kind: "redirect"; url: string };

export type Flow = {
  id: string;
  name: string;
  active: boolean;
  /** Arbitrary raw integer — normalized to % for display, per §24. */
  weight: number;
  landing: LandingStage;
  pwa: PwaStage;
  postlanding: PostlandingStage;
  destination: Destination;
};

export type StreamSet = {
  id: string;
  campaignId: string;
  name: string;
  priority: number;
  status: StreamSetStatus;
  rootFilter: FilterGroupNode;
  flows: Flow[];
  pixels: string[];
  fallbackUrl: string;
  createdAt: string;
  updatedAt: string;
};

const SET_NAMES = ["Mobile — Tier 1 GEOs", "Desktop — Retarget", "Bot & Proxy Block", "Everything Else"];

function offerDestination(networkId: string, offerId: string): Destination {
  const offer = OFFERS.find((o) => o.id === offerId);
  return { kind: "offer", networkId, offerId, offerUrl: offer?.url ?? "" };
}

const DISABLED_LANDING: LandingStage = { enabled: false, landingId: "", asPwa: false };
const DISABLED_PWA: PwaStage = { enabled: false, pwaId: "", pwaType: "internal" };
const DISABLED_POSTLANDING: PostlandingStage = { enabled: false, postlandingId: "" };

/** Deterministic per-campaign stream sets, seeded from the campaign id so the
 * set resolves consistently within a session (same pattern as campaign
 * daily-trend generation). Set 0 mirrors the §23 filter example, and its
 * first flow demos the full Landing → PWA → Offer funnel; set 2 (bot/proxy
 * block) demos a Redirect terminal step instead of an Offer. */
export function generateStreamSets(campaignId: string): StreamSet[] {
  let seed = 0;
  for (let i = 0; i < campaignId.length; i++) seed = (seed * 31 + campaignId.charCodeAt(i)) | 0;
  const rand = mulberry32(seed || 1);
  const now = new Date("2026-08-11T00:00:00Z").toISOString();

  return SET_NAMES.map((name, i) => {
    const rootFilter: FilterGroupNode =
      i === 0
        ? {
            id: genId(rand),
            type: "group",
            joiner: "AND",
            children: [
              { id: genId(rand), type: "condition", field: "country", operator: "IS", value: "US", valueTo: "" },
              {
                id: genId(rand),
                type: "condition",
                field: "device",
                operator: "IN",
                value: "mobile, tablet",
                valueTo: "",
              },
              {
                id: genId(rand),
                type: "group",
                joiner: "OR",
                children: [
                  { id: genId(rand), type: "condition", field: "os", operator: "IS", value: "android", valueTo: "" },
                  { id: genId(rand), type: "condition", field: "os", operator: "IS", value: "ios", valueTo: "" },
                ],
              },
            ],
          }
        : i === 1
          ? {
              id: genId(rand),
              type: "group",
              joiner: "AND",
              children: [
                { id: genId(rand), type: "condition", field: "device", operator: "IS", value: "desktop", valueTo: "" },
              ],
            }
          : i === 2
            ? {
                id: genId(rand),
                type: "group",
                joiner: "OR",
                children: [
                  { id: genId(rand), type: "condition", field: "bot", operator: "IS", value: "1", valueTo: "" },
                  { id: genId(rand), type: "condition", field: "proxy", operator: "IS", value: "1", valueTo: "" },
                ],
              }
            : emptyGroup();

    const flows: Flow[] =
      i === 0
        ? [
            {
              id: genId(rand),
              name: "Primary offer",
              active: true,
              weight: 70,
              landing: { enabled: true, landingId: LANDINGS[0].id, asPwa: false },
              pwa: { enabled: true, pwaId: PWAS[0].id, pwaType: "internal" },
              postlanding: DISABLED_POSTLANDING,
              destination: offerDestination("net_afftrust", "off_sweeps_us"),
            },
            {
              id: genId(rand),
              name: "Split 2",
              active: true,
              weight: 30,
              landing: DISABLED_LANDING,
              pwa: DISABLED_PWA,
              postlanding: DISABLED_POSTLANDING,
              destination: offerDestination("net_adcombo", "off_nutra_uk"),
            },
          ]
        : i === 1
          ? [
              {
                id: genId(rand),
                name: "Primary offer",
                active: true,
                weight: 100,
                landing: DISABLED_LANDING,
                pwa: DISABLED_PWA,
                postlanding: DISABLED_POSTLANDING,
                destination: offerDestination("net_direct", "off_crypto_ca"),
              },
            ]
          : i === 2
            ? [
                {
                  id: genId(rand),
                  name: "Safe redirect",
                  active: true,
                  weight: 100,
                  landing: DISABLED_LANDING,
                  pwa: DISABLED_PWA,
                  postlanding: DISABLED_POSTLANDING,
                  destination: { kind: "redirect", url: "https://example.com/safe" },
                },
              ]
            : [
                {
                  id: genId(rand),
                  name: "Primary offer",
                  active: true,
                  weight: 100,
                  landing: DISABLED_LANDING,
                  pwa: DISABLED_PWA,
                  postlanding: DISABLED_POSTLANDING,
                  destination: offerDestination("net_mylead", "off_dating_de"),
                },
              ];

    return {
      id: genId(rand),
      campaignId,
      name,
      priority: i + 1,
      status: i === 2 && rand() > 0.5 ? "paused" : "active",
      rootFilter,
      flows,
      pixels: i === 0 ? [`https://px.floxlink.io/s2s/${genId(rand).slice(0, 8)}.gif`] : [],
      fallbackUrl: "",
      createdAt: now,
      updatedAt: now,
    };
  });
}
