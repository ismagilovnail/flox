import { genId } from "@/lib/id";
import { emptyGroup, type FilterGroupNode } from "@/lib/filters";

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

export type FlowDestinationType = "offer" | "landing" | "pwa" | "postlanding";

export const FLOW_DESTINATION_TYPES: FlowDestinationType[] = ["offer", "landing", "pwa", "postlanding"];

export type Flow = {
  id: string;
  name: string;
  destinationType: FlowDestinationType;
  destinationUrl: string;
  weight: number;
  active: boolean;
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

/** Deterministic per-campaign stream sets, seeded from the campaign id so the
 * set resolves consistently within a session (same pattern as campaign
 * daily-trend generation). Set 0 mirrors the §23 example tree (AND of two
 * conditions plus a nested OR group) to demo real nesting out of the box. */
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

    const flowCount = 1 + Math.floor(rand() * 2);
    const rawWeights = Array.from({ length: flowCount }, () => rand());
    const weightSum = rawWeights.reduce((a, b) => a + b, 0);
    const flows: Flow[] = rawWeights.map((w, fi) => ({
      id: genId(rand),
      name: flowCount === 1 ? "Primary offer" : `Split ${fi + 1}`,
      destinationType: "offer",
      destinationUrl: `https://track.floxlink.io/offer/${genId(rand).slice(0, 8)}`,
      weight: Math.round((w / weightSum) * 100),
      active: true,
    }));
    const drift = 100 - flows.reduce((a, f) => a + f.weight, 0);
    flows[flows.length - 1].weight += drift;

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
