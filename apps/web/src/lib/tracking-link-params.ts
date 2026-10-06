/**
 * Query parameters a buyer appends to a tracking link, and the ad platforms'
 * own macros they are filled from. The tracker reads exactly these keys
 * (apps/tracker/params.go): utm_*, sub1–sub10, and ttclid/fbclid as external
 * click ids. This file only assembles the URL — routing decisions stay in Go.
 */

export const UTM_KEYS = ["utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term"] as const;
export const SUB_KEYS = Array.from({ length: 10 }, (_, i) => `sub${i + 1}`);

/** Keys the constructor offers, in the order they are written to the URL. */
export const PARAM_KEYS: string[] = [...UTM_KEYS, ...SUB_KEYS, "ttclid"];

export type LinkParams = Record<string, string>;

export type LinkPlatform = "facebook" | "tiktok";

/** Macro values are the platforms' own placeholders — they must reach the ad
 * platform literally (it substitutes them when serving the click), so they are
 * never percent-encoded. Facebook: https://www.facebook.com/business/help/2360940870872492
 * (fbclid is appended by Facebook itself). TikTok: __CALLBACK_PARAM__ is its click id. */
export const PLATFORM_PRESETS: Record<LinkPlatform, LinkParams> = {
  facebook: {
    utm_source: "facebook",
    utm_medium: "paid",
    utm_campaign: "{{campaign.name}}",
    utm_content: "{{ad.name}}",
    utm_term: "{{adset.name}}",
    sub1: "{{campaign.id}}",
    sub2: "{{adset.id}}",
    sub3: "{{ad.id}}",
    sub4: "{{placement}}",
    sub5: "{{site_source_name}}",
  },
  tiktok: {
    utm_source: "tiktok",
    utm_medium: "paid",
    utm_campaign: "__CAMPAIGN_NAME__",
    utm_content: "__CID_NAME__",
    utm_term: "__AID_NAME__",
    sub1: "__CAMPAIGN_ID__",
    sub2: "__AID__",
    sub3: "__CID__",
    sub4: "__PLACEMENT__",
    ttclid: "__CALLBACK_PARAM__",
  },
};

/** Which preset a traffic source's `type` maps to; null = no preset for it. */
export function platformForSourceType(type: string | undefined): LinkPlatform | null {
  if (type === "Facebook") return "facebook";
  if (type === "TikTok") return "tiktok";
  return null;
}

const MACRO = /\{\{[^{}]+\}\}|__[A-Z0-9_]+__/g;

/** encodeURIComponent for everything except the platforms' macros. */
function encodeValue(value: string): string {
  let out = "";
  let last = 0;
  for (const m of value.matchAll(MACRO)) {
    out += encodeURIComponent(value.slice(last, m.index)) + m[0];
    last = m.index + m[0].length;
  }
  return out + encodeURIComponent(value.slice(last));
}

/** Appends the non-empty params to `base` in PARAM_KEYS order, keeping any
 * query string `base` already has. Keys outside PARAM_KEYS are ignored. */
export function buildTrackingUrl(base: string, params: LinkParams): string {
  const parts = PARAM_KEYS.flatMap((key) => {
    const value = (params[key] ?? "").trim();
    return value ? [`${key}=${encodeValue(value)}`] : [];
  });
  if (parts.length === 0) return base;
  return `${base}${base.includes("?") ? "&" : "?"}${parts.join("&")}`;
}
