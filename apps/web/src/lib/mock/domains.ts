/**
 * Domains (§30) — a real module, not a text field: registrar connection,
 * DNS/nameserver management, own-domain parking + verification, expiry
 * tracking, automatic SSL issuance for campaign/PWA/fallback domains.
 * Registrar/DNS providers are swappable behind interfaces on the Go side
 * (§74) — here they're just a display enum. Seed includes the exact 3
 * strings already used as `mock/campaigns.ts`'s TRACKING_DOMAINS so
 * existing campaigns resolve to a real Domain once the campaign form
 * switches from that static array to this store.
 */

export type DomainStatus = "active" | "pending" | "error" | "expired";
export type SslStatus = "issued" | "pending" | "none" | "error";
export type DomainPurpose = "tracking" | "pwa" | "fallback";
export type Registrar = "namecheap" | "godaddy" | "cloudflare_registrar" | "unmanaged";
export type DnsProvider = "cloudflare" | "route53" | "unmanaged";

export const REGISTRAR_LABELS: Record<Registrar, string> = {
  namecheap: "Namecheap",
  godaddy: "GoDaddy",
  cloudflare_registrar: "Cloudflare Registrar",
  unmanaged: "Unmanaged (external)",
};

export const DNS_PROVIDER_LABELS: Record<DnsProvider, string> = {
  cloudflare: "Cloudflare DNS",
  route53: "AWS Route 53",
  unmanaged: "Unmanaged (external)",
};

export type Domain = {
  id: string;
  domain: string;
  status: DomainStatus;
  ssl: SslStatus;
  purpose: DomainPurpose[];
  registrar: Registrar;
  dnsProvider: DnsProvider;
  expiresAt: string | null;
  verifiedAt: string | null;
  createdAt: string;
  updatedAt: string;
};

export const DOMAINS: Domain[] = [
  {
    id: "dom_track",
    domain: "track.floxlink.io",
    status: "active",
    ssl: "issued",
    purpose: ["tracking"],
    registrar: "cloudflare_registrar",
    dnsProvider: "cloudflare",
    expiresAt: "2027-01-15T00:00:00Z",
    verifiedAt: "2026-01-20T00:00:00Z",
    createdAt: "2026-01-18T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
  },
  {
    id: "dom_go",
    domain: "go.floxtrk.com",
    status: "active",
    ssl: "issued",
    purpose: ["tracking"],
    registrar: "namecheap",
    dnsProvider: "cloudflare",
    expiresAt: "2026-11-02T00:00:00Z",
    verifiedAt: "2026-02-10T00:00:00Z",
    createdAt: "2026-02-05T00:00:00Z",
    updatedAt: "2026-06-14T00:00:00Z",
  },
  {
    id: "dom_clk",
    domain: "clk.floxdsp.net",
    status: "pending",
    ssl: "pending",
    purpose: ["tracking"],
    registrar: "godaddy",
    dnsProvider: "unmanaged",
    expiresAt: "2027-03-22T00:00:00Z",
    verifiedAt: null,
    createdAt: "2026-07-28T00:00:00Z",
    updatedAt: "2026-08-05T00:00:00Z",
  },
  {
    id: "dom_pwa",
    domain: "pwa.floxlink.io",
    status: "active",
    ssl: "issued",
    purpose: ["pwa"],
    registrar: "cloudflare_registrar",
    dnsProvider: "cloudflare",
    expiresAt: "2027-01-15T00:00:00Z",
    verifiedAt: "2026-01-20T00:00:00Z",
    createdAt: "2026-01-18T00:00:00Z",
    updatedAt: "2026-05-30T00:00:00Z",
  },
  {
    id: "dom_safe",
    domain: "safe.floxlink.io",
    status: "active",
    ssl: "issued",
    purpose: ["fallback"],
    registrar: "cloudflare_registrar",
    dnsProvider: "cloudflare",
    expiresAt: "2027-01-15T00:00:00Z",
    verifiedAt: "2026-01-20T00:00:00Z",
    createdAt: "2026-01-18T00:00:00Z",
    updatedAt: "2026-04-11T00:00:00Z",
  },
  {
    id: "dom_expired",
    domain: "oldpromo.floxdsp.net",
    status: "expired",
    ssl: "error",
    purpose: ["tracking"],
    registrar: "namecheap",
    dnsProvider: "unmanaged",
    expiresAt: "2026-06-01T00:00:00Z",
    verifiedAt: "2025-05-10T00:00:00Z",
    createdAt: "2025-05-01T00:00:00Z",
    updatedAt: "2026-06-01T00:00:00Z",
  },
];
