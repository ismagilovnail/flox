/**
 * Offers (§27) — real, team-managed entities. Models the real hierarchy
 * Network → Offer → Offer Link: each offer carries one or more named links
 * (primary + backups), URLs resolved via the shared macro system
 * (lib/macros.ts). IDs match the ones already baked into stream-set flows
 * (see mock/stream-sets.ts).
 */

export type OfferStatus = "active" | "paused" | "archived";

export type OfferLink = {
  id: string;
  label: string;
  url: string;
};

export type Offer = {
  id: string;
  networkId: string;
  name: string;
  countries: string[];
  payout: number;
  currency: string;
  /** Daily conversion cap; null = uncapped. */
  cap: number | null;
  status: OfferStatus;
  links: OfferLink[];
  createdAt: string;
  updatedAt: string;
};

export const OFFERS: Offer[] = [
  {
    id: "off_sweeps_us",
    networkId: "net_afftrust",
    name: "US Sweeps — CPA $12",
    countries: ["US"],
    payout: 12,
    currency: "USD",
    cap: 500,
    status: "active",
    links: [
      { id: "lnk_sweeps_us_primary", label: "Primary", url: "https://afftrust.example/click?aff_id=1042&click_id={click_id}" },
    ],
    createdAt: "2026-03-05T00:00:00Z",
    updatedAt: "2026-07-20T00:00:00Z",
  },
  {
    id: "off_nutra_uk",
    networkId: "net_adcombo",
    name: "UK Nutra Trial",
    countries: ["GB"],
    payout: 8,
    currency: "GBP",
    cap: 300,
    status: "active",
    links: [
      { id: "lnk_nutra_uk_primary", label: "Primary", url: "https://adcombo.example/track?subid={click_id}" },
    ],
    createdAt: "2026-03-16T00:00:00Z",
    updatedAt: "2026-06-28T00:00:00Z",
  },
  {
    id: "off_dating_de",
    networkId: "net_mylead",
    name: "DE Dating — CPL",
    countries: ["DE"],
    payout: 3,
    currency: "EUR",
    cap: null,
    status: "active",
    links: [
      { id: "lnk_dating_de_primary", label: "Primary", url: "https://mylead.example/go/{click_id}" },
      { id: "lnk_dating_de_backup", label: "Backup", url: "https://mylead-backup.example/go/{click_id}" },
    ],
    createdAt: "2026-04-03T00:00:00Z",
    updatedAt: "2026-07-05T00:00:00Z",
  },
  {
    id: "off_crypto_ca",
    networkId: "net_direct",
    name: "CA Crypto — RevShare",
    countries: ["CA"],
    payout: 25,
    currency: "USD",
    cap: 100,
    status: "paused",
    links: [
      { id: "lnk_crypto_ca_primary", label: "Primary", url: "https://advertiser.example/lp?cid={click_id}" },
    ],
    createdAt: "2026-05-22T00:00:00Z",
    updatedAt: "2026-08-01T00:00:00Z",
  },
];
