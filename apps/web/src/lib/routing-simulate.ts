/**
 * Mock implementation of the future `POST /routing/simulate` contract
 * (§6-SHARED, §26). This is a pure function of (stream sets, campaign
 * fallback, request) → result — the exact shape the real Go endpoint
 * (Phase 19) will expose, so Phase 27 swaps this call for a `fetch` with no
 * other UI changes. Do not let this drift into a second permanent routing
 * engine: once Phase 19 ports the decision logic to Go, this file is
 * replaced, not kept running alongside it.
 */

import type { FilterCondition, FilterField, FilterGroupNode, FilterOperator } from "@/lib/filters";
import { FILTER_FIELDS } from "@/lib/filters";
import type { Flow, StreamSet, StreamSetStatus } from "@/lib/mock/stream-sets";

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
  status: StreamSetStatus;
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

export type Destination = {
  kind: "offer" | "redirect" | "stream_set_fallback" | "campaign_fallback" | "none";
  url: string;
  label: string;
};

export type SimulateResult = {
  request: SimulateRequest;
  streamSetEvaluations: StreamSetEvaluation[];
  matchedStreamSet: StreamSetEvaluation | null;
  flowCandidates: FlowCandidate[];
  selectedFlow: Flow | null;
  destination: Destination;
  stickyNote: string;
};

function compareValues(a: string, b: string): number {
  const na = Number(a);
  const nb = Number(b);
  if (a.trim() !== "" && b.trim() !== "" && !Number.isNaN(na) && !Number.isNaN(nb)) return na - nb;
  return a.localeCompare(b);
}

function evaluateCondition(condition: FilterCondition, request: SimulateRequest): ConditionTrace {
  const requestValue = request[condition.field] ?? "";
  const value = condition.value;
  const norm = (s: string) => s.trim().toLowerCase();
  let passed: boolean;

  switch (condition.operator) {
    case "IS":
      passed = norm(requestValue) === norm(value);
      break;
    case "IS_NOT":
      passed = norm(requestValue) !== norm(value);
      break;
    case "IN":
      passed = value.split(",").map((v) => norm(v)).includes(norm(requestValue));
      break;
    case "NOT_IN":
      passed = !value.split(",").map((v) => norm(v)).includes(norm(requestValue));
      break;
    case "CONTAINS":
      passed = norm(requestValue).includes(norm(value));
      break;
    case "NOT_CONTAINS":
      passed = !norm(requestValue).includes(norm(value));
      break;
    case "STARTS_WITH":
      passed = norm(requestValue).startsWith(norm(value));
      break;
    case "ENDS_WITH":
      passed = norm(requestValue).endsWith(norm(value));
      break;
    case "MATCHES":
      try {
        passed = new RegExp(value, "i").test(requestValue);
      } catch {
        passed = false;
      }
      break;
    case "EXISTS":
      passed = requestValue.trim() !== "";
      break;
    case "NOT_EXISTS":
      passed = requestValue.trim() === "";
      break;
    case "GT":
      passed = compareValues(requestValue, value) > 0;
      break;
    case "GTE":
      passed = compareValues(requestValue, value) >= 0;
      break;
    case "LT":
      passed = compareValues(requestValue, value) < 0;
      break;
    case "LTE":
      passed = compareValues(requestValue, value) <= 0;
      break;
    case "BETWEEN":
      passed = compareValues(requestValue, condition.value) >= 0 && compareValues(requestValue, condition.valueTo) <= 0;
      break;
  }

  return {
    kind: "condition",
    field: condition.field,
    operator: condition.operator,
    value,
    valueTo: condition.valueTo,
    requestValue,
    passed,
  };
}

function evaluateGroup(group: FilterGroupNode, request: SimulateRequest): GroupTrace {
  const children = group.children.map((child) =>
    child.type === "condition" ? evaluateCondition(child, request) : evaluateGroup(child, request),
  );
  const passed = group.children.length === 0 ? true : group.joiner === "AND" ? children.every((c) => c.passed) : children.some((c) => c.passed);
  return { kind: "group", joiner: group.joiner, passed, children };
}

// Written as BigInt() calls rather than `123n` literals so the file compiles
// under the project's ES2017 target (TS2737) without dragging the whole
// frontend's tsconfig to ES2020 for one function.
const FNV_OFFSET_64 = BigInt("14695981039346656037");
const FNV_PRIME_64 = BigInt("1099511628211");
const U64_MASK = BigInt("18446744073709551615"); // 2^64 - 1
const ZERO = BigInt(0);

/**
 * FNV-1a/64 — the byte-for-byte mirror of internal/routing's VisitHash.
 * BigInt because the multiply overflows a JS number long before 64 bits.
 *
 * Hashing UTF-8 bytes (not UTF-16 code units) is what keeps it identical to
 * the Go side, where a string index yields a byte: a non-ASCII character in a
 * user agent would otherwise diverge the two implementations on exactly the
 * traffic that is hardest to debug.
 */
export function visitHash(key: string): bigint {
  let h = FNV_OFFSET_64;
  for (const byte of new TextEncoder().encode(key)) {
    h = (h ^ BigInt(byte)) & U64_MASK;
    h = (h * FNV_PRIME_64) & U64_MASK;
  }
  return h;
}

/**
 * Mirrors internal/routing's pickWeighted (§38): same key + same weights →
 * same flow. Deterministic, never Math.random() — a simulator that rolled the
 * dice would show a different answer on every click for an unchanged
 * configuration, which is the opposite of what a simulator is for.
 *
 * Eligibility is decided before the draw: only active, positive-weight flows
 * take part, and shares are relative to their sum rather than to 100.
 */
function pickWeightedFlow(flows: Flow[], visitKey: string): { candidates: FlowCandidate[]; selected: Flow | null } {
  const eligible = flows.filter((f) => f.active && f.weight > 0);
  const weightSum = eligible.reduce((sum, f) => sum + f.weight, 0);

  if (weightSum <= 0) {
    return {
      candidates: flows.map((f) => ({ flowId: f.id, name: f.name, weight: f.weight, normalizedPercent: 0, selected: false })),
      selected: null,
    };
  }

  const point = visitHash(visitKey) % BigInt(weightSum);
  let selectedId = "";
  let running = ZERO;
  for (const f of flows) {
    if (!f.active || f.weight <= 0) continue;
    running += BigInt(f.weight);
    if (point < running) {
      selectedId = f.id;
      break;
    }
  }

  const candidates = flows.map((f) => ({
    flowId: f.id,
    name: f.name,
    weight: f.weight,
    normalizedPercent: f.active && f.weight > 0 ? (f.weight / weightSum) * 100 : 0,
    selected: f.id === selectedId,
  }));
  return { candidates, selected: flows.find((f) => f.id === selectedId) ?? null };
}

/**
 * Derives the visit key from the simulated request.
 *
 * The tracker fingerprints the real visit (campaign + client IP + user agent);
 * the simulator has neither an IP nor a campaign id to hand, so it hashes the
 * filled-in request fields instead. That difference is fine and deliberate:
 * the shared contract is "same key + same weights → same flow", and deriving
 * the key is the caller's job on both sides. What it buys the operator is that
 * changing a request field re-rolls the pick, while re-running an unchanged
 * request does not.
 */
function deriveVisitKey(request: SimulateRequest): string {
  return Object.entries(request)
    .filter(([, value]) => value !== "")
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([field, value]) => `${field}=${value}`)
    .join("|");
}

function resolveDestination(flow: Flow | null, streamSetFallback: string, campaignFallback: string): Destination {
  if (flow) {
    if (flow.destination.kind === "redirect" && flow.destination.url) {
      return { kind: "redirect", url: flow.destination.url, label: "Redirect" };
    }
    if (flow.destination.kind === "offer" && flow.destination.offerUrl) {
      return { kind: "offer", url: flow.destination.offerUrl, label: "Offer" };
    }
  }
  if (streamSetFallback) return { kind: "stream_set_fallback", url: streamSetFallback, label: "Stream Set fallback" };
  if (campaignFallback) return { kind: "campaign_fallback", url: campaignFallback, label: "Campaign fallback" };
  return { kind: "none", url: "", label: "No destination configured" };
}

export function simulateRoute(streamSets: StreamSet[], campaignFallbackUrl: string, request: SimulateRequest): SimulateResult {
  const sorted = [...streamSets].sort((a, b) => a.priority - b.priority);

  const evaluations: StreamSetEvaluation[] = sorted.map((set) => {
    const trace = evaluateGroup(set.rootFilter, request);
    const active = set.status === "active";
    const matched = active && trace.passed;
    return {
      streamSetId: set.id,
      name: set.name,
      priority: set.priority,
      status: set.status,
      matched,
      reasonNotMatched: !active ? "Stream set is paused" : !trace.passed ? "Filters didn't match" : undefined,
      trace,
    };
  });

  const matchedEval = evaluations.find((e) => e.matched) ?? null;
  const matchedSet = matchedEval ? (sorted.find((s) => s.id === matchedEval.streamSetId) ?? null) : null;

  let flowCandidates: FlowCandidate[] = [];
  let selectedFlow: Flow | null = null;
  if (matchedSet) {
    const pick = pickWeightedFlow(matchedSet.flows, deriveVisitKey(request));
    flowCandidates = pick.candidates;
    selectedFlow = pick.selected;
  }

  const destination = resolveDestination(selectedFlow, matchedSet?.fallbackUrl ?? "", campaignFallbackUrl);

  return {
    request,
    streamSetEvaluations: evaluations,
    matchedStreamSet: matchedEval,
    flowCandidates,
    selectedFlow,
    destination,
    stickyNote:
      "Sticky assignment isn't configured on campaigns yet (§39-STICKY lands with tenant/campaign settings in a later phase). Once it is, a returning visitor's sf_{campaignId} cookie would override this pick if present — cookie is the source of truth, Redis is cache only.",
  };
}
