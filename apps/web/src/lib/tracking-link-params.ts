/**
 * Query parameters a buyer appends to a tracking link, and the ad platforms'
 * own macros they are filled from. The tracker reads exactly these keys
 * (apps/tracker/params.go): utm_*, sub1–sub10, external_click_id (the
 * traffic source's own click id, echoed back in the postback to the source)
 * and ttclid/fbclid as external click ids. This file only assembles the URL
 * — routing decisions stay in Go.
 */

import { SOURCE_TEMPLATES, sourceTemplate } from "@/lib/source-templates";

export const UTM_KEYS = ["utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term"] as const;
export const SUB_KEYS = Array.from({ length: 10 }, (_, i) => `sub${i + 1}`);

/** Keys the constructor offers, in the order they are written to the URL. */
export const PARAM_KEYS: string[] = [...UTM_KEYS, ...SUB_KEYS, "external_click_id", "ttclid"];

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

/** Which preset a traffic source's `type` maps to; null = no preset for it.
 * `type` is a free string server-side (the UI writes "Facebook", older rows
 * hold "facebook"), so the match ignores case and surrounding spaces. */
export function platformForSourceType(type: string | undefined): LinkPlatform | null {
  const normalized = type?.trim().toLowerCase();
  if (normalized === "facebook") return "facebook";
  if (normalized === "tiktok") return "tiktok";
  return null;
}

/** The ad networks' macro syntaxes, which must reach the network literally:
 * {{campaign.id}} (Facebook, HilltopAds), __AID__ (TikTok), ${SUBID}
 * (PropellerAds, Clickadu), {zoneid} (most), ##ZONE_ID## (Adsterra),
 * [:click_id] (Galaksion). */
const MACRO = /\{\{[^{}]+\}\}|__[A-Z0-9_]+__|\$\{[A-Za-z0-9_]+\}|\{[A-Za-z0-9_.]+\}|##[A-Za-z0-9_()]+##|\[:[a-z_]+\]/g;

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

/** A link preset: Facebook, TikTok, or a traffic source template
 * (lib/source-templates.ts). */
export type LinkPreset = { key: string; params: LinkParams; label?: string };

/** Every preset; label undefined for the two platforms (translated by the
 * caller). */
export const LINK_PRESETS: LinkPreset[] = [
  { key: "facebook", params: PLATFORM_PRESETS.facebook },
  { key: "tiktok", params: PLATFORM_PRESETS.tiktok },
  ...SOURCE_TEMPLATES.map((t) => ({ key: t.key, params: t.link, label: t.name })),
];

/** Which preset a traffic source's links start from: its template, else its
 * type's platform. */
export function presetForSource(source: { type?: string; templateKey?: string } | undefined): string | null {
  if (source?.templateKey && sourceTemplate(source.templateKey)) return source.templateKey;
  return platformForSourceType(source?.type);
}
