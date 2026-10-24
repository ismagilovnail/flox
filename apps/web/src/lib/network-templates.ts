/**
 * Affiliate-platform templates (audit 2026-10-15, INT-3): for a platform,
 * which of its macros fill FLOX's incoming postback URL, which offer-link
 * parameter carries FLOX's click id to it, and how its status values map to
 * FLOX statuses. Picking one when creating a network shows the URL with
 * those macros and creates the mappings.
 *
 * Only platforms whose postback macros are documented publicly are here,
 * each with its source; most iGaming platforms (Affilka, Cellxpert,
 * MyAffiliates, Income Access, NetRefer) keep theirs behind login.
 */

import type { CpaStatus } from "@/lib/api/conversions";

export type NetworkTemplate = {
  key: string;
  name: string;
  /** Incoming postback query values: FLOX parameter → the platform's macro. */
  incoming: Partial<Record<"click_id" | "status" | "revenue" | "currency" | "txn_id", string>>;
  /** The offer-link parameter the platform passes back as the click id. */
  offerLinkParam: string;
  /** The platform's status values → FLOX statuses, created with the network. */
  statusMappings: { networkStatus: string; floxStatus: CpaStatus }[];
  sources: string[];
  /** "perGoal": the platform sends one postback per goal with no status
   * macro; the operator puts the goal's name in status. */
  note?: "perGoal";
};

export const NETWORK_TEMPLATES: NetworkTemplate[] = [
  {
    key: "affise",
    name: "Affise",
    // "There is no clickid macro for affiliate postback on Affise. Use
    // {sub1} - {sub8} macros to pass this value."
    incoming: { click_id: "{sub1}", status: "{status}", revenue: "{sum}", currency: "{currency}", txn_id: "{transactionid}" },
    offerLinkParam: "sub1",
    // 1 Approved, 2 Pending, 3 Declined, 5 approved and put on Hold.
    statusMappings: [
      { networkStatus: "1", floxStatus: "CPA_ACCEPT" },
      { networkStatus: "2", floxStatus: "CPA_HOLD" },
      { networkStatus: "3", floxStatus: "CPA_DECLINE" },
      { networkStatus: "5", floxStatus: "CPA_HOLD" },
    ],
    sources: [
      "https://help-center.affise.com/en/articles/6483741-affiliate-postback-url-macros",
      "https://help-center.affise.com/en/articles/6655842-affiliate-postback-macros-affiliates",
    ],
  },
  {
    key: "scaleo",
    name: "Scaleo",
    incoming: { click_id: "{aff_click_id}", status: "REPLACE_GOAL", revenue: "{payout}" },
    offerLinkParam: "aff_click_id",
    statusMappings: [],
    sources: ["https://help.scaleo.io/article/384-affiliate-parameters", "https://help.scaleo.io/article/517-affiliate-postbacks"],
    note: "perGoal",
  },
];

/** "Own site" (audit 2026-10-24, UNI-1): not a platform but a source of
 * conversions without a partner — a shop, a service, an app reporting its
 * own leads and sales from its server. Its statuses are plain words, mapped
 * when the source is created, so the site can send status=purchase at once.
 * The URL shows what the site's developer fills in: the click id the site
 * kept from the visit, the order's amount and id (txn_id, which also tells
 * one repeat purchase from the next). */
export const OWN_SITE_KEY = "own_site";

export const OWN_SITE_TEMPLATE: NetworkTemplate = {
  key: OWN_SITE_KEY,
  name: "Own site",
  incoming: { click_id: "{click_id}", status: "purchase", revenue: "{amount}", currency: "{currency}", txn_id: "{order_id}" },
  offerLinkParam: "click_id",
  statusMappings: [
    { networkStatus: "lead", floxStatus: "CPA_HOLD" },
    { networkStatus: "purchase", floxStatus: "CPA_ACCEPT" },
    { networkStatus: "repeat_purchase", floxStatus: "CPA_REDEP" },
    { networkStatus: "refund", floxStatus: "CPA_DECLINE" },
    { networkStatus: "cancel", floxStatus: "CPA_DECLINE" },
    { networkStatus: "spam", floxStatus: "CPA_TRASH" },
  ],
  sources: [],
};

export function isOwnSite(key: string | undefined): boolean {
  return key === OWN_SITE_KEY;
}

export function networkTemplate(key: string | undefined): NetworkTemplate | undefined {
  if (key === OWN_SITE_KEY) return OWN_SITE_TEMPLATE;
  return NETWORK_TEMPLATES.find((t) => t.key === key);
}
