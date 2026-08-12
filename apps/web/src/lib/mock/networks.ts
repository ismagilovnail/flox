/**
 * Networks (§27) — real, team-managed entities. IDs match the ones already
 * baked into stream-set flows (see mock/stream-sets.ts) so existing flow
 * destinations keep resolving once the Flow Builder switches from the old
 * flow-entities.ts placeholder list to this store's data.
 */

export type NetworkStatus = "active" | "paused" | "archived";

export type Network = {
  id: string;
  name: string;
  /** Outgoing postback URL template — resolved via the shared macro system (§27, lib/macros.ts). */
  postbackUrl: string;
  status: NetworkStatus;
  createdAt: string;
  updatedAt: string;
};

export const NETWORKS: Network[] = [
  {
    id: "net_afftrust",
    name: "AffTrust CPA",
    postbackUrl: "https://afftrust.example/postback?click_id={click_id}&status={status}&payout={payout}&currency={currency}",
    status: "active",
    createdAt: "2026-03-02T00:00:00Z",
    updatedAt: "2026-07-18T00:00:00Z",
  },
  {
    id: "net_adcombo",
    name: "AdCombo",
    postbackUrl: "https://adcombo.example/api/postback?subid={click_id}&status={status}&payout={payout}",
    status: "active",
    createdAt: "2026-03-14T00:00:00Z",
    updatedAt: "2026-06-30T00:00:00Z",
  },
  {
    id: "net_mylead",
    name: "MyLead",
    postbackUrl: "https://mylead.example/postback?cid={click_id}&event={status}&amount={payout}&currency={currency}",
    status: "active",
    createdAt: "2026-04-01T00:00:00Z",
    updatedAt: "2026-07-02T00:00:00Z",
  },
  {
    id: "net_direct",
    name: "Direct advertiser",
    postbackUrl: "https://advertiser.example/s2s?click_id={click_id}&status={status}&revenue={revenue}",
    status: "paused",
    createdAt: "2026-05-20T00:00:00Z",
    updatedAt: "2026-08-01T00:00:00Z",
  },
];
