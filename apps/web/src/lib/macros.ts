/**
 * Cross-cutting macro/placeholder substitution system (§27). One resolver,
 * reused everywhere a URL or payload template needs runtime values: offer
 * links (Phase 11), postback templates and pixel payloads (Phase 13). Do not
 * fork a second token list or resolver per feature.
 */

export type MacroDef = {
  token: string;
  label: string;
  /** sub1–sub10: the number, for the translated label. */
  sub?: number;
  example: string;
};

export const MACROS: MacroDef[] = [
  { token: "{click_id}", label: "Click ID", example: "8f3a1c9e2b7d4f10" },
  { token: "{external_click_id}", label: "Traffic source click ID", example: "a1b2c3d4" },
  { token: "{status}", label: "Conversion status", example: "accept" },
  { token: "{revenue}", label: "Revenue (original currency)", example: "12.00" },
  { token: "{currency}", label: "Currency code", example: "USD" },
  { token: "{payout}", label: "Offer payout", example: "12.00" },
  { token: "{campaign_id}", label: "Campaign ID", example: "8K3N2Q7T4R1V" },
  { token: "{offer_id}", label: "Offer ID", example: "off_sweeps_us" },
  { token: "{source}", label: "Traffic source", example: "Facebook" },
  { token: "{country}", label: "Geo (ISO country code)", example: "US" },
  { token: "{device}", label: "Device type", example: "mobile" },
  ...Array.from({ length: 10 }, (_, i) => ({
    token: `{sub${i + 1}}`,
    label: `Sub parameter ${i + 1}`,
    sub: i + 1,
    example: `sub${i + 1}_value`,
  })),
];

/** A postlanding's onward link: a signed tracker URL that records
 * POSTLANDING_CLICK and continues to the offer (apps/tracker/postlanding.go).
 * Resolved only in a postlanding URL. */
export const NEXT_URL_MACRO: MacroDef = { token: "{next_url}", label: "Next step link", example: "https://track.example/pc/…" };

/** What a postlanding URL is resolved with: the click's own values (no
 * conversion yet, so no status/revenue/currency) and {next_url}. */
const CLICK_TIME_TOKENS = new Set(["{click_id}", "{campaign_id}", "{offer_id}", "{country}", "{device}"]);
export const POSTLANDING_MACROS: MacroDef[] = [
  NEXT_URL_MACRO,
  ...MACROS.filter((m) => CLICK_TIME_TOKENS.has(m.token) || m.sub !== undefined),
];

/** {pwa_link} is the signed tracker link an external PWA reports to:
 * {pwa_link}installed records PWA_INSTALL, {pwa_link}open records PWA_OPEN
 * and continues to the offer (apps/tracker/pwa_external.go). Resolved only
 * in an external PWA's URL. */
export const PWA_LINK_MACRO: MacroDef = { token: "{pwa_link}", label: "PWA reporting link", example: "https://track.example/pwx/…/" };

/** What a PWA stage URL is resolved with: the click's own values, and for
 * an external PWA {pwa_link}. */
export const EXTERNAL_PWA_MACROS: MacroDef[] = [
  PWA_LINK_MACRO,
  ...MACROS.filter((m) => CLICK_TIME_TOKENS.has(m.token) || m.sub !== undefined),
];
export const IOS_APP_MACROS: MacroDef[] = MACROS.filter((m) => CLICK_TIME_TOKENS.has(m.token) || m.sub !== undefined);

/** Replaces every recognized `{token}` in `template` with `values[token]`.
 * Unrecognized or unset tokens are left as-is — never throws on a partial
 * value set (e.g. previewing an offer link before a click has occurred). */
export function resolveMacros(template: string, values: Partial<Record<string, string>>): string {
  return template.replace(/\{[a-z0-9_]+\}/gi, (token) => values[token] ?? token);
}
