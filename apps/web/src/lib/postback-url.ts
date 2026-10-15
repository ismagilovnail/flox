/**
 * The incoming postback URL a network is given: the server's base
 * (Network.incomingPostbackUrl = TRACKER_PUBLIC_URL + /postback/{id}) plus
 * the query apps/tracker/postback.go reads. Values in braces are for the
 * operator to swap for the network's own macros; {secret} is filled in
 * only right after a create or a regenerate, the one time the secret is
 * known (audit 2026-10-15, UX-1).
 */
export function incomingPostbackTemplate(base: string, secret?: string): string {
  const s = secret ? encodeURIComponent(secret) : "{secret}";
  return `${base}?secret=${s}&click_id={click_id}&status={status}&revenue={revenue}&currency={currency}&txn_id={txn_id}`;
}

/** The query parameters /postback understands, in the order they appear. */
export const INCOMING_POSTBACK_PARAMS = ["secret", "click_id", "status", "revenue", "currency", "txn_id", "event_time"] as const;
