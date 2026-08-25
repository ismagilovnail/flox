import { apiFetch } from "@/lib/api/client";
import { genId } from "@/lib/id";
import type { FilterField, FilterGroupNode, FilterNode, FilterOperator } from "@/lib/filters";

/** Mirrors apps/internal/streamset's real shape — the write path for
 * Stream Sets/Filters/Flows. Not the same file as lib/mock/stream-sets.ts
 * / stores/stream-sets.ts — those stay mocked and in place because the
 * Routing Simulator (still fully mocked, out of scope this phase) imports
 * them. This file is the real API layer, used only by the Stream Sets
 * card on the campaign detail page. */
export type StreamSetStatus = "active" | "paused";

/** The wire shape of a filter tree node — apps/internal/streamset.FilterNode
 * flattened the same way (one struct, `omitempty` fields), no `id` (the
 * routing engine never needed one; `id` is purely a local React-key /
 * tree-editing concern). hydrateFilterNode/dehydrateFilterNode below
 * convert to and from lib/filters.ts's own FilterNode (which does carry
 * an id, since filter-group-builder.tsx's tree-mutation helpers address
 * nodes by it). */
export type ApiFilterCondition = {
  type: "condition";
  field: string;
  operator: string;
  value: string;
  valueTo: string;
};
export type ApiFilterGroup = {
  type: "group";
  joiner: "AND" | "OR";
  children: ApiFilterNode[];
};
export type ApiFilterNode = ApiFilterCondition | ApiFilterGroup;

/** No offerUrl on the wire: flows only stores destination_offer_id /
 * destination_network_id server-side — the offer's URL is resolved at
 * routing-decision time from its offer_links (routingstore.loadFlows),
 * not carried on this CRUD-facing shape. */
export type ApiDestination =
  | { kind: "offer"; networkId: string; offerId: string }
  | { kind: "redirect"; url: string };

/** The per-Flow display mode for the PWA stage (how *this* flow shows the
 * PWA: an internal page, an external redirect, or an iOS app-store link) —
 * independent of which pwa manifest (pwaId) is selected. Mirrors
 * apps/internal/streamset.PwaType and the flows table's own pwa_type CHECK
 * constraint (migration 00006). Not a real entity (unlike Landing/Pwa/
 * Postlanding below), so it has no id of its own. */
export type PwaType = "internal" | "external" | "ios_app";
export const PWA_TYPES: PwaType[] = ["internal", "external", "ios_app"];

/** §25's canonical funnel stages a Flow can optionally run before its
 * Destination (Landing -> PWA -> Postlanding -> Destination). Each stage
 * always has an `enabled` flag independent of its id — toggling a stage
 * off keeps its previous pick around rather than clearing it, matching
 * apps/internal/streamset's own FlowLanding/FlowPwa/FlowPostlanding. */
export type ApiFlowLanding = { enabled: boolean; landingId: string; asPwa: boolean };
export type ApiFlowPwa = { enabled: boolean; pwaId: string; pwaType: PwaType | "" };
export type ApiFlowPostlanding = { enabled: boolean; postlandingId: string };

export type ApiFlow = {
  id: string;
  name: string;
  active: boolean;
  weight: number;
  landing: ApiFlowLanding;
  pwa: ApiFlowPwa;
  postlanding: ApiFlowPostlanding;
  destination: ApiDestination;
};

export type ApiFlowInput = Omit<ApiFlow, "id">;

export type StreamSet = {
  id: string;
  organizationId: string;
  campaignId: string;
  name: string;
  priority: number;
  status: StreamSetStatus;
  fallbackUrl: string;
  rootFilter: ApiFilterNode;
  flows: ApiFlow[];
  /** Which of the org's Pixels this Stream Set fires — a many-to-many via
   * stream_set_pixels (migration 00008), a Stream-Set-level concern, not
   * a per-Flow one. See docs/stream-sets.md/docs/pixels.md. */
  pixelIds: string[];
  createdAt: string;
  updatedAt: string;
};

export type CreateStreamSetInput = {
  name: string;
  fallbackUrl: string;
  rootFilter: ApiFilterNode;
  flows: ApiFlowInput[];
  pixelIds: string[];
};

export type UpdateStreamSetInput = Partial<CreateStreamSetInput> & { status?: StreamSetStatus };

/** Assigns a fresh client-side id to every node — ids never round-trip
 * from the server (see ApiFilterNode's own doc comment); this is what
 * lets filter-group-builder.tsx's id-addressed mutation helpers
 * (addConditionToGroup, updateCondition, removeNode, …) operate on a
 * freshly-loaded tree unchanged. */
export function hydrateFilterNode(node: ApiFilterNode): FilterNode {
  if (node.type === "condition") {
    return {
      id: genId(),
      type: "condition",
      field: node.field as FilterField,
      operator: node.operator as FilterOperator,
      value: node.value,
      valueTo: node.valueTo,
    };
  }
  return {
    id: genId(),
    type: "group",
    joiner: node.joiner,
    children: node.children.map(hydrateFilterNode),
  };
}

/** Strips the client-only id back out before sending to the API. */
export function dehydrateFilterNode(node: FilterNode): ApiFilterNode {
  if (node.type === "condition") {
    return { type: "condition", field: node.field, operator: node.operator, value: node.value, valueTo: node.valueTo };
  }
  return { type: "group", joiner: node.joiner, children: node.children.map(dehydrateFilterNode) };
}

export function hydrateRootFilter(node: ApiFilterNode): FilterGroupNode {
  const hydrated = hydrateFilterNode(node);
  if (hydrated.type !== "group") {
    throw new Error("root filter must be a group");
  }
  return hydrated;
}

export function listStreamSets(campaignId: string): Promise<{ streamSets: StreamSet[] }> {
  return apiFetch(`/campaigns/${campaignId}/stream-sets`);
}

export function getStreamSet(campaignId: string, id: string): Promise<StreamSet> {
  return apiFetch(`/campaigns/${campaignId}/stream-sets/${id}`);
}

export function createStreamSet(campaignId: string, input: CreateStreamSetInput): Promise<StreamSet> {
  return apiFetch(`/campaigns/${campaignId}/stream-sets`, { method: "POST", body: input });
}

export function updateStreamSet(campaignId: string, id: string, input: UpdateStreamSetInput): Promise<StreamSet> {
  return apiFetch(`/campaigns/${campaignId}/stream-sets/${id}`, { method: "PATCH", body: input });
}

export function deleteStreamSet(campaignId: string, id: string): Promise<void> {
  return apiFetch(`/campaigns/${campaignId}/stream-sets/${id}`, { method: "DELETE" });
}

export function duplicateStreamSet(campaignId: string, id: string): Promise<StreamSet> {
  return apiFetch(`/campaigns/${campaignId}/stream-sets/${id}/duplicate`, { method: "POST" });
}

export function reorderStreamSets(campaignId: string, orderedIds: string[]): Promise<{ streamSets: StreamSet[] }> {
  return apiFetch(`/campaigns/${campaignId}/stream-sets/reorder`, { method: "POST", body: { orderedIds } });
}
