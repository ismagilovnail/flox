import { apiFetch } from "@/lib/api/client";
import { fetchAllPages } from "@/lib/api/pagination";

/** Mirrors apps/internal/network's real shape — the single client for
 * the real Networks API, used by the Networks page and by every feature
 * that resolves a network id to a name (offers, conversions, postback
 * logs). No mock counterpart exists anymore: lib/mock/networks.ts and
 * stores/networks.ts were deleted once stream-sets, postbacks and
 * conversions all moved onto their own real backends. */
export type NetworkStatus = "active" | "paused" | "archived";

export type Network = {
  id: string;
  organizationId: string;
  name: string;
  postbackUrl: string;
  acceptDuplicates: boolean;
  /** Payout currency (ISO 4217): a postback with revenue but no currency
   * is priced in it. USD unless the operator chose another. */
  currency: string;
  status: NetworkStatus;
  createdAt: string;
  updatedAt: string;
};

export type CreateNetworkInput = {
  name: string;
  postbackUrl: string;
  acceptDuplicates: boolean;
  currency: string;
};

export type UpdateNetworkInput = Partial<CreateNetworkInput> & { status?: NetworkStatus };

export function listNetworks(): Promise<{ networks: Network[]; total: number }> {
  return fetchAllPages<"networks", Network>("/networks", "networks");
}

export function getNetwork(id: string): Promise<Network> {
  return apiFetch(`/networks/${id}`);
}

/** postbackSecret is the ONE time this ever appears in a response — the
 * value a network operator pastes into the incoming-postback URL FLOX
 * gives them (?secret=...). Not re-derivable afterward except by calling
 * regeneratePostbackSecret, which invalidates the old one (apps/internal/
 * network's own one-way-hash-at-rest design, §54/Phase 30). */
export function createNetwork(input: CreateNetworkInput): Promise<Network & { postbackSecret: string }> {
  return apiFetch("/networks", { method: "POST", body: input });
}

export function regeneratePostbackSecret(id: string): Promise<{ postbackSecret: string }> {
  return apiFetch(`/networks/${id}/regenerate-postback-secret`, { method: "POST" });
}

export function updateNetwork(id: string, input: UpdateNetworkInput): Promise<Network> {
  return apiFetch(`/networks/${id}`, { method: "PATCH", body: input });
}

export function deleteNetwork(id: string): Promise<void> {
  return apiFetch(`/networks/${id}`, { method: "DELETE" });
}

export function duplicateNetwork(id: string): Promise<Network> {
  return apiFetch(`/networks/${id}/duplicate`, { method: "POST" });
}

export function pauseNetwork(id: string): Promise<Network> {
  return apiFetch(`/networks/${id}/pause`, { method: "POST" });
}

export function activateNetwork(id: string): Promise<Network> {
  return apiFetch(`/networks/${id}/activate`, { method: "POST" });
}

export function archiveNetwork(id: string): Promise<Network> {
  return apiFetch(`/networks/${id}`, { method: "PATCH", body: { status: "archived" } });
}
