import { apiFetch } from "@/lib/api/client";

/** Mirrors apps/internal/network's real shape. Not the same file as
 * lib/mock/networks.ts / stores/networks.ts — those stay mocked and
 * in place because stream-sets/postbacks/conversions (still fully
 * mocked features) import them transitively, the same reason Phase 27
 * kept lib/mock/campaigns.ts around. This file is the real API layer,
 * used only by the Networks page/feature itself. */
export type NetworkStatus = "active" | "paused" | "archived";

export type Network = {
  id: string;
  organizationId: string;
  name: string;
  postbackUrl: string;
  acceptDuplicates: boolean;
  status: NetworkStatus;
  createdAt: string;
  updatedAt: string;
};

export type CreateNetworkInput = {
  name: string;
  postbackUrl: string;
  acceptDuplicates: boolean;
};

export type UpdateNetworkInput = Partial<CreateNetworkInput> & { status?: NetworkStatus };

export function listNetworks(): Promise<{ networks: Network[] }> {
  return apiFetch("/networks");
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
