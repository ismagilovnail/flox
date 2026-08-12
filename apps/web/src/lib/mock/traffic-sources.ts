/** Traffic Sources (§27) — real, team-managed entities. Names line up with
 * the source strings already used on campaigns (mock/campaigns.ts SOURCES)
 * so campaign data reads as coming from one of these sources. */

export type SourceType =
  | "Facebook"
  | "TikTok"
  | "Google"
  | "Native Ads"
  | "Push"
  | "SEO"
  | "Influencer"
  | "Email"
  | "Other";

export const SOURCE_TYPES: SourceType[] = [
  "Facebook",
  "TikTok",
  "Google",
  "Native Ads",
  "Push",
  "SEO",
  "Influencer",
  "Email",
  "Other",
];

/** Cost data origin for this source. `manual`/`none` are available from
 * Phase 11; the ad-network pulls are wired in Phase 27-COST (§74 CostProvider). */
export type CostIntegration = "none" | "manual" | "facebook_ads" | "tiktok_ads";

export const COST_INTEGRATION_LABELS: Record<CostIntegration, string> = {
  none: "Not connected",
  manual: "Manual entry",
  facebook_ads: "Facebook Ads (OAuth)",
  tiktok_ads: "TikTok Ads (OAuth)",
};

export type SourceStatus = "active" | "paused" | "archived";

export type TrafficSource = {
  id: string;
  name: string;
  type: SourceType;
  /** Landing/redirect tracking template — resolved via the shared macro system (§27, lib/macros.ts). */
  trackingTemplate: string;
  costIntegration: CostIntegration;
  status: SourceStatus;
  createdAt: string;
  updatedAt: string;
};

export const TRAFFIC_SOURCES: TrafficSource[] = [
  {
    id: "src_facebook",
    name: "Facebook",
    type: "Facebook",
    trackingTemplate: "https://track.floxlink.io/click?clickid={click_id}&sub1={sub1}&sub2={sub2}",
    costIntegration: "facebook_ads",
    status: "active",
    createdAt: "2026-02-10T00:00:00Z",
    updatedAt: "2026-07-25T00:00:00Z",
  },
  {
    id: "src_tiktok",
    name: "TikTok",
    type: "TikTok",
    trackingTemplate: "https://track.floxlink.io/click?clickid={click_id}&sub1={sub1}",
    costIntegration: "tiktok_ads",
    status: "active",
    createdAt: "2026-02-18T00:00:00Z",
    updatedAt: "2026-07-19T00:00:00Z",
  },
  {
    id: "src_google",
    name: "Google",
    type: "Google",
    trackingTemplate: "https://track.floxlink.io/click?clickid={click_id}&gclid={sub1}",
    costIntegration: "none",
    status: "active",
    createdAt: "2026-03-01T00:00:00Z",
    updatedAt: "2026-06-11T00:00:00Z",
  },
  {
    id: "src_native",
    name: "Native Ads",
    type: "Native Ads",
    trackingTemplate: "https://track.floxlink.io/click?clickid={click_id}&widget={sub1}",
    costIntegration: "manual",
    status: "active",
    createdAt: "2026-03-22T00:00:00Z",
    updatedAt: "2026-07-02T00:00:00Z",
  },
  {
    id: "src_push",
    name: "Push",
    type: "Push",
    trackingTemplate: "https://track.floxlink.io/click?clickid={click_id}&campaign={sub1}",
    costIntegration: "manual",
    status: "paused",
    createdAt: "2026-04-14T00:00:00Z",
    updatedAt: "2026-06-30T00:00:00Z",
  },
  {
    id: "src_seo",
    name: "SEO",
    type: "SEO",
    trackingTemplate: "https://track.floxlink.io/click?clickid={click_id}",
    costIntegration: "none",
    status: "active",
    createdAt: "2026-01-20T00:00:00Z",
    updatedAt: "2026-05-15T00:00:00Z",
  },
];
