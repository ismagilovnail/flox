"use client";

import { useTeamStore } from "@/stores/team";

/** Matches the mock signed-in user (Owner) already seeded in stores/team.ts.
 * Deliberately NOT wired to the real GET /auth/me session (which has existed
 * since Phase 28) — Custom Metrics, Referral, and Content Gallery are all
 * still reading from lib/mock/* fixtures with no backend of their own,
 * and switching their "who am I, what can I manage" identity source to the
 * real session ahead of each feature's own backend-integration phase would
 * silently break their "is this mine" checks against still-mock seed data
 * that only knows about mock member ids. Revisit this alongside whichever
 * phase gives each of those three features a real backend. */
const CURRENT_USER_MEMBER_ID = "mem_owner";

export function useCurrentMember() {
  const member = useTeamStore((s) => s.members.find((m) => m.id === CURRENT_USER_MEMBER_ID));
  const isOwnerOrAdmin = member?.role === "Owner" || member?.role === "Admin";
  return { member, memberId: CURRENT_USER_MEMBER_ID, isOwnerOrAdmin };
}
