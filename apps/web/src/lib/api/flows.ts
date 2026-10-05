/**
 * The org-wide, cross-campaign Flows list (ROADMAP.md: "Flows have no
 * top-level list view") — backed by apps/internal/streamset's own
 * ListFlows/RegisterFlows, mounted at the top-level GET /flows (not
 * nested under one campaign's /campaigns/{id}/stream-sets the way every
 * other flow-shaped read in this codebase is). Read-only: a flow is
 * still only ever created/edited/reordered wholesale through its parent
 * stream set's own editor (features/stream-sets/stream-set-editor.tsx),
 * never through this page.
 */
import { apiFetch } from "@/lib/api/client";
import type { ApiDestination, ApiFlowLanding, ApiFlowPwa, ApiFlowPostlanding } from "@/lib/api/stream-sets";

/**
 * One row of the org-wide Flows list — apps/internal/streamset.
 * FlowSummary's exact wire shape. `id` is NOT a stable, permanent
 * identity the way `streamSetId`/`campaignId` are: opening the flow's
 * parent stream set in its own editor and saving — even having changed
 * nothing about the flow itself — resubmits the whole flows array and
 * mints every flow under that stream set a fresh id (Repository.Update
 * deletes and reinserts rather than patching rows in place). A quick
 * pause/activate toggle does NOT do this. Since apps/tracker records
 * exactly this id as every click's own flow_id, "View statistics"
 * (below) only ever shows history recorded since the id currently on
 * screen was minted — flow-columns.tsx's own row action discloses this
 * directly rather than leaving a sudden "0 clicks" unexplained.
 */
export type FlowSummary = {
  id: string;
  name: string;
  active: boolean;
  weight: number;
  landing: ApiFlowLanding;
  pwa: ApiFlowPwa;
  postlanding: ApiFlowPostlanding;
  destination: ApiDestination;
  streamSetId: string;
  streamSetName: string;
  campaignId: string;
  campaignName: string;
  updatedAt: string;
};

export function getFlows(): Promise<{ flows: FlowSummary[] }> {
  return apiFetch("/flows");
}
