/**
 * Placeholder pickers for the Flow Builder (§24-25). Networks/Offers/
 * Landings/PWAs/Postlandings become real, team-managed entities in Phase
 * 11-12 — until then these are small static lists so the funnel editor has
 * something to bind to. Swap for real entity queries when those phases land;
 * the Flow shape (networkId/offerId/landingId/pwaId/postlandingId) doesn't
 * change.
 */

export type PwaType = "internal" | "external" | "ios_app";
export const PWA_TYPES: PwaType[] = ["internal", "external", "ios_app"];

export type NetworkOption = { id: string; name: string };
export const NETWORKS: NetworkOption[] = [
  { id: "net_afftrust", name: "AffTrust CPA" },
  { id: "net_adcombo", name: "AdCombo" },
  { id: "net_mylead", name: "MyLead" },
  { id: "net_direct", name: "Direct advertiser" },
];

export type OfferOption = { id: string; networkId: string; name: string; url: string };
export const OFFERS: OfferOption[] = [
  { id: "off_sweeps_us", networkId: "net_afftrust", name: "US Sweeps — CPA $12", url: "https://afftrust.example/click?aff_id=1042&click_id={click_id}" },
  { id: "off_nutra_uk", networkId: "net_adcombo", name: "UK Nutra Trial", url: "https://adcombo.example/track?subid={click_id}" },
  { id: "off_dating_de", networkId: "net_mylead", name: "DE Dating — CPL", url: "https://mylead.example/go/{click_id}" },
  { id: "off_crypto_ca", networkId: "net_direct", name: "CA Crypto — RevShare", url: "https://advertiser.example/lp?cid={click_id}" },
];

export type LandingOption = { id: string; name: string; url: string };
export const LANDINGS: LandingOption[] = [
  { id: "lnd_prelander_a", name: "Prelander A/B — Sweeps", url: "https://cdn.floxlink.io/lnd/prelander-a" },
  { id: "lnd_quiz", name: "Quiz Lander", url: "https://cdn.floxlink.io/lnd/quiz" },
  { id: "lnd_advertorial", name: "Advertorial", url: "https://cdn.floxlink.io/lnd/advertorial" },
];

export type PwaOption = { id: string; name: string };
export const PWAS: PwaOption[] = [
  { id: "pwa_sweeps", name: "Sweeps PWA" },
  { id: "pwa_casino", name: "Casino Lite PWA" },
];

export type PostlandingOption = { id: string; name: string; url: string };
export const POSTLANDINGS: PostlandingOption[] = [
  { id: "psl_thankyou", name: "Thank You / Upsell", url: "https://cdn.floxlink.io/psl/thankyou" },
  { id: "psl_survey", name: "Post-install Survey", url: "https://cdn.floxlink.io/psl/survey" },
];
