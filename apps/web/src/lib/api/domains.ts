import { apiFetch } from "@/lib/api/client";

/** Mirrors apps/internal/domain's real shape. Registrar connection, DNS
 * record management, and automatic SSL issuance are NOT implemented
 * server-side (see that package's own doc comment) — status/ssl/registrar/
 * dnsProvider are plain operator-set fields, not automated. */
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

export function listDomains(): Promise<{ domains: Domain[] }> {
  return apiFetch("/domains");
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
