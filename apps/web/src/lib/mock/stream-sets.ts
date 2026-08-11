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

/** Subset of §22's full field list — the flat filter editor covers the common
 * routing fields; the remaining fields (sub1-10, browser_version, etc.) land
 * with the nested builder in Phase 8. */
export type FilterField =
  | "country"
  | "device"
  | "platform"
  | "os"
  | "browser"
  | "language"
  | "bot"
  | "proxy"
  | "connection_type"
  | "referrer"
  | "utm_source"
  | "utm_medium"
  | "utm_campaign";

export type FilterOperator =
  | "IS"
  | "IS_NOT"
  | "IN"
  | "NOT_IN"
  | "CONTAINS"
  | "NOT_CONTAINS"
  | "STARTS_WITH"
  | "ENDS_WITH"
  | "MATCHES"
  | "EXISTS"
  | "NOT_EXISTS"
  | "GT"
  | "GTE"
  | "LT"
  | "LTE";

export const FILTER_FIELDS: FilterField[] = [
  "country",
  "device",
  "platform",
  "os",
  "browser",
  "language",
  "bot",
  "proxy",
  "connection_type",
  "referrer",
  "utm_source",
  "utm_medium",
  "utm_campaign",
];

export const FILTER_OPERATORS: FilterOperator[] = [
  "IS",
  "IS_NOT",
  "IN",
  "NOT_IN",
  "CONTAINS",
  "NOT_CONTAINS",
  "STARTS_WITH",
  "ENDS_WITH",
  "MATCHES",
  "EXISTS",
  "NOT_EXISTS",
  "GT",
  "GTE",
  "LT",
  "LTE",
];

/** No value input for existence checks — the field alone is the condition. */
export const OPERATORS_WITHOUT_VALUE: FilterOperator[] = ["EXISTS", "NOT_EXISTS"];

export type FilterCondition = {
  id: string;
  field: FilterField;
  operator: FilterOperator;
  value: string;
};

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
  /** Flat AND/OR join across all conditions — nested groups arrive in Phase 8. */
  joiner: "AND" | "OR";
  filters: FilterCondition[];
  flows: Flow[];
  pixels: string[];
  fallbackUrl: string;
  createdAt: string;
  updatedAt: string;
};

export function genId(rand: () => number = Math.random) {
  const chars = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";
  let out = "";
  for (let i = 0; i < 12; i++) out += chars[Math.floor(rand() * chars.length)];
  return out;
}

const SET_NAMES = ["Mobile — Tier 1 GEOs", "Desktop — Retarget", "Bot & Proxy Block", "Everything Else"];
const COUNTRY_POOL = ["US", "CA", "GB", "DE", "FR", "AU"];

/** Deterministic per-campaign stream sets, seeded from the campaign id so the
 * set resolves consistently within a session (same pattern as campaign
 * daily-trend generation). */
export function generateStreamSets(campaignId: string): StreamSet[] {
  let seed = 0;
  for (let i = 0; i < campaignId.length; i++) seed = (seed * 31 + campaignId.charCodeAt(i)) | 0;
  const rand = mulberry32(seed || 1);
  const now = new Date("2026-08-11T00:00:00Z").toISOString();

  return SET_NAMES.map((name, i) => {
    const filters: FilterCondition[] =
      i === 0
        ? [
            { id: genId(rand), field: "device", operator: "IS", value: "mobile" },
            { id: genId(rand), field: "country", operator: "IN", value: COUNTRY_POOL.slice(0, 3).join(", ") },
          ]
        : i === 1
          ? [{ id: genId(rand), field: "device", operator: "IS", value: "desktop" }]
          : i === 2
            ? [{ id: genId(rand), field: "bot", operator: "IS", value: "1" }]
            : [];

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
      joiner: "AND",
      filters,
      flows,
      pixels: i === 0 ? [`https://px.floxlink.io/s2s/${genId(rand).slice(0, 8)}.gif`] : [],
      fallbackUrl: "",
      createdAt: now,
      updatedAt: now,
    };
  });
}

