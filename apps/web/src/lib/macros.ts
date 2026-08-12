/**
 * Cross-cutting macro/placeholder substitution system (§27). One resolver,
 * reused everywhere a URL or payload template needs runtime values: offer
 * links (Phase 11), postback templates and pixel payloads (Phase 13). Do not
 * fork a second token list or resolver per feature.
 */

export type MacroDef = {
  token: string;
  label: string;
  example: string;
};

export const MACROS: MacroDef[] = [
  { token: "{click_id}", label: "Click ID", example: "8f3a1c9e2b7d4f10" },
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
    example: `sub${i + 1}_value`,
  })),
];

/** Replaces every recognized `{token}` in `template` with `values[token]`.
 * Unrecognized or unset tokens are left as-is — never throws on a partial
 * value set (e.g. previewing an offer link before a click has occurred). */
export function resolveMacros(template: string, values: Partial<Record<string, string>>): string {
  return template.replace(/\{[a-z0-9_]+\}/gi, (token) => values[token] ?? token);
}
