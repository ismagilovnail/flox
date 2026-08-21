import { apiFetch } from "@/lib/api/client";

/** Mirrors apps/internal/offer's real shape. Not the same file as
 * lib/mock/offers.ts / stores/offers.ts — those stay mocked and in
 * place because stream-sets/conversions (still fully mocked features)
 * import them transitively, same reason Phase 27 kept
 * lib/mock/campaigns.ts around. This file is the real API layer, used
 * only by the Offers page/feature itself. */
export type OfferStatus = "active" | "paused" | "archived";

export type OfferLink = {
  id: string;
  label: string;
  url: string;
};

export type OfferLinkInput = {
  label: string;
  url: string;
};

export type Offer = {
  id: string;
  organizationId: string;
  networkId: string;
  name: string;
  countries: string[];
  payout: number;
  currency: string;
  /** null = uncapped. */
  cap: number | null;
  status: OfferStatus;
  links: OfferLink[];
  createdAt: string;
  updatedAt: string;
};

export type CreateOfferInput = {
  networkId: string;
  name: string;
  countries: string[];
  payout: number;
  currency: string;
  cap: number | null;
  links: OfferLinkInput[];
};

/** cap is deliberately `number | null | undefined`, not just
 * `number | null`: omitted (undefined) means "leave unchanged" — the
 * request body serializer (lib/api/client.ts's apiFetch, via plain
 * JSON.stringify) drops undefined keys entirely, which the backend's
 * OptionalCap reads as "not sent." An explicit `null` still reaches the
 * server and clears the cap to uncapped. links is similarly optional:
 * omitted leaves the offer's link set untouched; present replaces it
 * wholesale (apps/internal/offer's own semantics, matching the form's
 * whole-array submission). */
export type UpdateOfferInput = Partial<Omit<CreateOfferInput, "links">> & {
  status?: OfferStatus;
  links?: OfferLinkInput[];
};

export function listOffers(): Promise<{ offers: Offer[] }> {
  return apiFetch("/offers");
}

export function getOffer(id: string): Promise<Offer> {
  return apiFetch(`/offers/${id}`);
}

export function createOffer(input: CreateOfferInput): Promise<Offer> {
  return apiFetch("/offers", { method: "POST", body: input });
}

export function updateOffer(id: string, input: UpdateOfferInput): Promise<Offer> {
  return apiFetch(`/offers/${id}`, { method: "PATCH", body: input });
}

export function deleteOffer(id: string): Promise<void> {
  return apiFetch(`/offers/${id}`, { method: "DELETE" });
}

export function duplicateOffer(id: string): Promise<Offer> {
  return apiFetch(`/offers/${id}/duplicate`, { method: "POST" });
}

export function pauseOffer(id: string): Promise<Offer> {
  return apiFetch(`/offers/${id}/pause`, { method: "POST" });
}

export function activateOffer(id: string): Promise<Offer> {
  return apiFetch(`/offers/${id}/activate`, { method: "POST" });
}

export function archiveOffer(id: string): Promise<Offer> {
  return apiFetch(`/offers/${id}`, { method: "PATCH", body: { status: "archived" } });
}
