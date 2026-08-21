/**
 * The Routing Simulator's real API layer (Phase 27's final slice —
 * "wire the Routing Simulator to /routing/simulate"). This file used to
 * wrap a local pure-function mock (`lib/routing-simulate.ts`) precisely
 * so that swapping it for a real `fetch()` later would change only this
 * file's body — see `docs/architecture.md`'s §6-SHARED decision. That
 * day has arrived: the mock is deleted, and these types now describe
 * the wire contract of `POST /campaigns/{campaignId}/routing/simulate`
 * (apps/internal/routingsimulate), not a client-side re-implementation
 * of routing decisions (CLAUDE.md invariant #1 — there is exactly one
 * routing engine, and it's Go).
 */

import { apiFetch } from "@/lib/api/client";
import { FILTER_FIELDS, type FilterField, type FilterOperator } from "@/lib/filters";

export type SimulateRequest = Record<FilterField, string>;

export function emptySimulateRequest(): SimulateRequest {
  const request = Object.fromEntries(FILTER_FIELDS.map((f) => [f, ""])) as SimulateRequest;
  request.bot = "0";
  request.proxy = "0";
  return request;
}

export type ConditionTrace = {
  kind: "condition";
  field: FilterField;
  operator: FilterOperator;
  value: string;
  valueTo: string;
  requestValue: string;
  passed: boolean;
};

export type GroupTrace = {
  kind: "group";
  joiner: "AND" | "OR";
  passed: boolean;
  children: FilterTrace[];
};

export type FilterTrace = ConditionTrace | GroupTrace;

export type StreamSetEvaluation = {
  streamSetId: string;
  name: string;
  priority: number;
  status: "active" | "paused";
  matched: boolean;
  reasonNotMatched?: string;
  trace: FilterTrace;
};

export type FlowCandidate = {
  flowId: string;
  name: string;
  weight: number;
  normalizedPercent: number;
  selected: boolean;
};

/** No `kind` enum: apps/internal/routingsimulate.Destination sends the
 * already-resolved human label directly ("Offer", "Redirect", "Stream
 * Set fallback", "Campaign fallback", "No destination configured") —
 * the exact same string routing.Explanation.DestinationLabel computed
 * for RouteResult.Reason, so the simulator can never show a label that
 * disagrees with what the engine actually decided. An empty `url`
 * always pairs with the "no destination" label. */
export type Destination = {
  url: string;
  label: string;
};

export type SimulateResult = {
  request: SimulateRequest;
  streamSetEvaluations: StreamSetEvaluation[];
  matchedStreamSet: StreamSetEvaluation | null;
  flowCandidates: FlowCandidate[];
  destination: Destination;
  stickyNote: string;
};

export function simulateRoute(campaignId: string, request: SimulateRequest): Promise<SimulateResult> {
  return apiFetch(`/campaigns/${campaignId}/routing/simulate`, { method: "POST", body: { request } });
}
