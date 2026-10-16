/**
 * Traffic source templates (audit 2026-10-15, INT-1): for an ad network,
 * which of its macros go into the tracking link — its click id into
 * `external_click_id`, the rest into sub1… — and the URL FLOX calls back on
 * a conversion (traffic_sources.postback_url, sent by the conversion engine
 * with {external_click_id} and {revenue} filled in).
 *
 * Only what a source documents publicly is here, each with its source URL.
 * The account-specific parts of a postback (REPLACE_…) come from the
 * network's own dashboard; the API refuses a URL that still has one.
 */

import type { LinkParams } from "@/lib/tracking-link-params";

export type SourceTemplate = {
  key: string;
  name: string;
  /** Query parameters for the tracking link: FLOX key → the network's macro. */
  link: LinkParams;
  /** Postback to the network; {external_click_id}/{revenue} are FLOX macros. */
  postbackUrl: string;
  /** What each sub carries, for the form's hint. */
  subs: Record<string, string>;
  /** Where the macros come from. */
  sources: string[];
  /** Set when a part rests on one secondary source: check it in the account. */
  caution?: "postback" | "clickId";
};

export const SOURCE_TEMPLATES: SourceTemplate[] = [
  {
    key: "propellerads",
    name: "PropellerAds",
    link: { external_click_id: "${SUBID}", sub1: "{zoneid}", sub2: "{campaignid}", sub3: "{country}", sub4: "{region}", sub5: "{isp}", sub6: "{carrier}" },
    postbackUrl: "https://ad.propellerads.com/conversion.php?aid=REPLACE_AID&tid=REPLACE_TID&visitor_id={external_click_id}&payout={revenue}",
    subs: { sub1: "zone", sub2: "campaign", sub3: "country", sub4: "region", sub5: "ISP", sub6: "carrier" },
    sources: [
      "https://help.funnelflux.pro/en/article/tracking-setup-with-propellerads-ao9elh/",
      "https://wecantrack.com/installation/propellerads-integration/",
    ],
  },
  {
    key: "exoclick",
    name: "ExoClick",
    link: { external_click_id: "{conversions_tracking}", sub1: "{zone_id}", sub2: "{campaign_id}", sub3: "{site_id}", sub4: "{category_id}", sub5: "{format}", sub6: "{variation_id}", sub7: "{src_hostname}" },
    postbackUrl: "https://main.exoclick.com/tag.php?goal=REPLACE_GOAL&tag={external_click_id}",
    subs: { sub1: "zone", sub2: "campaign", sub3: "site", sub4: "category", sub5: "format", sub6: "variation", sub7: "hostname" },
    sources: [
      "https://help.funnelflux.pro/en/article/tracking-setup-with-exoclick-jkle9j/",
      "https://stape.io/helpdesk/documentation/exoclick-tag",
    ],
  },
  {
    key: "adsterra",
    name: "Adsterra",
    link: { external_click_id: "##SUB_ID_SHORT(action)##", sub1: "##ZONE_ID##", sub2: "##CAMPAIGN_ID##", sub3: "##PLACEMENT_ID##", sub4: "##PUBLISHER_ID##", sub5: "##BANNER_ID##" },
    postbackUrl: "https://www.pbterra.com/name/REPLACE_NAME/at?subid_short={external_click_id}&atpay={revenue}",
    subs: { sub1: "zone", sub2: "campaign", sub3: "placement", sub4: "publisher", sub5: "banner" },
    sources: [
      "https://help.funnelflux.pro/en/article/tracking-setup-with-adsterra-o4k7yz/",
      "https://adsterra.com/blog/es/guia-de-configuracion-de-campana-adsterra-para-afiliados/",
    ],
  },
  {
    key: "trafficstars",
    name: "TrafficStars",
    link: { external_click_id: "{click_id}" },
    postbackUrl: "https://tsyndicate.com/api/v1/cpa/action?key=REPLACE_KEY&clickid={external_click_id}&value={revenue}",
    subs: {},
    sources: [
      "https://stape.io/helpdesk/documentation/trafficstars-tag",
      "https://help.funnelflux.pro/en/article/tracking-setup-with-trafficstars-1ftuvng/",
    ],
  },
  {
    key: "clickadu",
    name: "Clickadu",
    link: { external_click_id: "${SUBID}", sub1: "{zoneid}", sub2: "{campaignid}", sub3: "{os}", sub4: "{device}", sub5: "{browser}", sub6: "{connection_type}", sub7: "{carrier}", sub8: "{pn_type}" },
    postbackUrl: "https://sconvtrk.com/conversion/REPLACE_ID/?aid=REPLACE_AID&visitor_id={external_click_id}",
    subs: { sub1: "zone", sub2: "campaign", sub3: "OS", sub4: "device", sub5: "browser", sub6: "connection type", sub7: "carrier", sub8: "push type" },
    sources: ["https://help.funnelflux.pro/en/article/tracking-setup-with-clickadu-7y1y8v/"],
  },
  {
    key: "hilltopads",
    name: "HilltopAds",
    link: { external_click_id: "{{token}}", sub1: "{{zoneid}}", sub2: "{{campaignid}}", sub3: "{{geo}}", sub4: "{{adid}}", sub5: "{{category}}", sub6: "{{browsername}}" },
    postbackUrl: "https://postback.hilltopads.com/close/?token={external_click_id}&price={revenue}",
    subs: { sub1: "zone", sub2: "campaign", sub3: "country", sub4: "ad", sub5: "category", sub6: "browser" },
    sources: [
      "https://hilltopads.com/advertisers-help/en/articles/9133118-what-tokens-are-available-at-hilltopads",
      "https://cpvlab.pro/docs/integrations/traffic-source-tokens.html",
    ],
    caution: "postback",
  },
  {
    key: "galaksion",
    name: "Galaksion",
    link: { external_click_id: "[:click_id]", sub1: "{zoneid}", sub2: "{cid}" },
    postbackUrl: "https://postback.report/postback?cid=REPLACE_CID&click_id={external_click_id}",
    subs: { sub1: "zone", sub2: "campaign" },
    sources: ["https://help.funnelflux.pro/en/article/tracking-setup-with-galaksion-1d39htz/"],
    caution: "clickId",
  },
  {
    key: "evadav",
    name: "EvaDav",
    link: { external_click_id: "{CLICKID}", sub1: "{ZONE_ID}", sub2: "{CAMPAIGN_ID}", sub3: "{SOURCE_ID}", sub4: "{CREATIVE_ID}", sub5: "{FORMAT}", sub6: "{SUB_PERIOD}" },
    postbackUrl: "https://evadav.com/phpb?click_id={external_click_id}&payout={revenue}",
    subs: { sub1: "zone", sub2: "campaign", sub3: "source", sub4: "creative", sub5: "format", sub6: "subscription age" },
    sources: ["https://help.funnelflux.pro/en/article/tracking-setup-with-evadav-18vif3y/"],
  },
];

export function sourceTemplate(key: string | undefined): SourceTemplate | undefined {
  return SOURCE_TEMPLATES.find((t) => t.key === key);
}
