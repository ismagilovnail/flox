/**
 * The frontend API boundary (§32, Phase 15). Every other domain's "server
 * calls" today are Zustand store actions over in-memory mock arrays — that
 * already satisfies "don't scatter fetch() in components," since components
 * never touch data directly, only through a store's typed action surface.
 * `stores/*.ts` action functions ARE that domain's mock API contract; there
 * is no separate `src/lib/api/<domain>.ts` per store, because that would be
 * an empty pass-through wrapper duplicating the store for no benefit until
 * a domain's Phase 27 integration actually needs one.
 *
 * Routing is the one exception, and the reason this directory exists now
 * instead of at Phase 27: `docs/architecture.md`'s §6-SHARED decision
 * already promises the Routing Simulator "runs against a local mock that
 * implements the exact same request/response contract... in Phase 27 it is
 * switched to the real endpoint with no UI changes." A plain synchronous
 * function call (the old `lib/routing-simulate.ts` call site) can't keep
 * that promise — swapping it for a real `fetch()` later would force the UI
 * to become async then, not now. Wrapping it as a promise-returning call
 * today, backed by the exact same pure mock function, means Phase 27 only
 * ever changes this file's body.
 */

import { simulateRoute as simulateRouteMock, type SimulateRequest, type SimulateResult } from "@/lib/routing-simulate";
import type { StreamSet } from "@/lib/mock/stream-sets";

export async function simulateRoute(
  streamSets: StreamSet[],
  campaignFallbackUrl: string,
  request: SimulateRequest,
): Promise<SimulateResult> {
  return simulateRouteMock(streamSets, campaignFallbackUrl, request);
}
