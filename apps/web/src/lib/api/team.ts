import { apiFetch } from "@/lib/api/client";
import type { MemberStatus, Role } from "@/lib/mock/team";

/** Mirrors apps/internal/auth.membershipResponse's JSON shape exactly —
 * the real Team page's own type. (Historical note: this used to differ
 * from a lib/mock/team.ts TeamMember type that a mock "current member"
 * hook exposed to Custom Metrics/Content Gallery/Referral before each got
 * its own real backend — that mock roster and hook are gone now that all
 * three read real session/team data directly.) */
export type Membership = {
  id: string;
  userId: string;
  name: string;
  email: string;
  role: Role;
  status: MemberStatus;
  invitedAt: string;
  lastActiveAt: string | null;
};

export type InviteMemberInput = {
  name: string;
  email: string;
  role: Role;
};

export type UpdateMemberInput = {
  role?: Role;
  status?: MemberStatus;
};

/** apps/internal/auth's audit_logs-backed activity feed — populated only
 * by this package's own membership actions (see docs/auth.md), not a
 * cross-domain audit trail. `action` is a raw key like "team.invited";
 * activity-panel.tsx maps it to a human sentence. */
export type ActivityEntry = {
  id: string;
  action: string;
  entityType: string;
  entityId: string;
  actorName: string | null;
  createdAt: string;
};

export function listMembers(): Promise<Membership[]> {
  return apiFetch("/team/members");
}

export function inviteMember(input: InviteMemberInput): Promise<{ inviteUrl: string }> {
  return apiFetch("/team/members/invite", { method: "POST", body: input });
}

export function updateMember(id: string, input: UpdateMemberInput): Promise<Membership> {
  return apiFetch(`/team/members/${id}`, { method: "PATCH", body: input });
}

export function resendInvite(id: string): Promise<{ inviteUrl: string }> {
  return apiFetch(`/team/members/${id}/resend-invite`, { method: "POST" });
}

export function removeMember(id: string): Promise<void> {
  return apiFetch(`/team/members/${id}`, { method: "DELETE" });
}

export function listActivity(): Promise<ActivityEntry[]> {
  return apiFetch("/team/activity");
}
