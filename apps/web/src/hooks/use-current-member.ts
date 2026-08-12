"use client";

import { useTeamStore } from "@/stores/team";

/** Matches the mock signed-in user (Owner) already seeded in stores/team.ts.
 * There is no auth session yet (that's Phase 28) — every role-gated feature
 * (Custom Metrics, Referral, Content Gallery, …) needs the same "who am I,
 * what can I manage" lookup, so it lives here once instead of being
 * re-declared per feature. */
const CURRENT_USER_MEMBER_ID = "mem_owner";

export function useCurrentMember() {
  const member = useTeamStore((s) => s.members.find((m) => m.id === CURRENT_USER_MEMBER_ID));
  const isOwnerOrAdmin = member?.role === "Owner" || member?.role === "Admin";
  return { member, memberId: CURRENT_USER_MEMBER_ID, isOwnerOrAdmin };
}
