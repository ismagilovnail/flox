import { networkTemplate } from "@/lib/network-templates";

/**
 * The incoming postback URL a network is given: the server's base
 * (Network.incomingPostbackUrl = TRACKER_PUBLIC_URL + /postback/{id}) plus
 * the query apps/tracker/postback.go reads. Values in braces are for the
 * operator to swap for the network's own macros; {secret} is filled in
 * only right after a create or a regenerate, the one time the secret is
 * known (audit 2026-10-15, UX-1).
 */
export function incomingPostbackTemplate(base: string, secret?: string, templateKey?: string): string {
  const s = secret ? encodeURIComponent(secret) : "{secret}";
  // An affiliate-platform template (audit INT-3) carries only the
  // parameters it has macros for: a FLOX placeholder left in would reach
  // FLOX literally ({currency} as the currency).
  const template = networkTemplate(templateKey);
  const keys = ["click_id", "status", "revenue", "currency", "txn_id"] as const;
  const parts = keys.flatMap((k) => {
    if (!template) return [`${k}={${k}}`];
    const v = template.incoming[k];
    return v ? [`${k}=${v}`] : [];
  });
  return `${base}?secret=${s}&${parts.join("&")}`;
}

/** The query parameters /postback understands, in the order they appear. */
export const INCOMING_POSTBACK_PARAMS = ["secret", "click_id", "status", "revenue", "currency", "txn_id", "event_time"] as const;
