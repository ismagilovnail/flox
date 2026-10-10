import { apiFetch } from "@/lib/api/client";
import { fetchAllPages } from "@/lib/api/pagination";

/** Mirrors apps/internal/domain's real shape. Registrar connection (via
 * Namecheap) and DNS zone/SSL status + record management (via
 * Cloudflare) are real for those two providers, using the org's own
 * connected Settings > Integrations credentials — verify()/renew()/
 * syncDns() below. GoDaddy/Route53/cloudflare_registrar/unmanaged have
 * no real adapter and stay plain operator-set fields. */
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
  organizationId: string;
  domain: string;
  status: DomainStatus;
  ssl: SslStatus;
  purpose: DomainPurpose[];
  registrar: Registrar;
  dnsProvider: DnsProvider;
  expiresAt: string | null;
  verifiedAt: string | null;
  /** When the organization proved it controls the domain's DNS; null until
   * then. Only a verified domain can be active, and only an active one
   * tracks (tech audit T6). */
  ownershipVerifiedAt: string | null;
  /** The TXT record that proves it: `name` must hold `value`. */
  ownershipRecord: { name: string; value: string };
  /** From a real TLS connection to the domain (worker, every few minutes
   * until issued, then every few hours): the served certificate's expiry,
   * when it was last checked, and why the check found no valid one. */
  sslExpiresAt: string | null;
  sslCheckedAt: string | null;
  sslError: string;
  createdAt: string;
  updatedAt: string;
};

export type CreateDomainInput = {
  domain: string;
  purpose: DomainPurpose[];
  registrar: Registrar;
  dnsProvider: DnsProvider;
  status: DomainStatus;
};

export type UpdateDomainInput = Partial<CreateDomainInput>;

export function listDomains(): Promise<{ domains: Domain[]; total: number }> {
  return fetchAllPages<"domains", Domain>("/domains", "domains");
}

export function getDomain(id: string): Promise<Domain> {
  return apiFetch(`/domains/${id}`);
}

export function createDomain(input: CreateDomainInput): Promise<Domain> {
  return apiFetch("/domains", { method: "POST", body: input });
}

export function updateDomain(id: string, input: UpdateDomainInput): Promise<Domain> {
  return apiFetch(`/domains/${id}`, { method: "PATCH", body: input });
}

export function deleteDomain(id: string): Promise<void> {
  return apiFetch(`/domains/${id}`, { method: "DELETE" });
}

/** Read-only: a real Namecheap check for registrar=namecheap, a real
 * Cloudflare check for dnsProvider=cloudflare. Neither charges anything
 * nor changes any DNS record. */
/** Checks the domain's ownership TXT record; on success the domain is
 * verified and active. A 422 says what DNS returned. */
export function verifyDomainOwnership(id: string): Promise<Domain> {
  return apiFetch(`/domains/${id}/verify-ownership`, { method: "POST" });
}

export function verifyDomain(id: string): Promise<Domain> {
  return apiFetch(`/domains/${id}/verify`, { method: "POST" });
}

/** A real, billable action against the org's connected Namecheap
 * account — registrar must be namecheap. Never a dry run: a success
 * response means Namecheap actually charged and extended the
 * registration. */
export function renewDomain(id: string, years: number): Promise<Domain> {
  return apiFetch(`/domains/${id}/renew`, { method: "POST", body: { years } });
}

/** Creates or updates a real, proxied CNAME record at Cloudflare
 * pointing this domain at FLOX's own tracker — dnsProvider must be
 * cloudflare. Mutates the org's actual live DNS. */
export function syncDomainDNS(id: string): Promise<Domain> {
  return apiFetch(`/domains/${id}/sync-dns`, { method: "POST" });
}

/** Connects to the domain over TLS now and records the certificate it
 * serves — the worker's scheduled check, on demand. Read-only. */
export function checkDomainSSL(id: string): Promise<Domain> {
  return apiFetch(`/domains/${id}/check-ssl`, { method: "POST" });
}
