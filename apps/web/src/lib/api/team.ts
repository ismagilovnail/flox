import { apiFetch } from "@/lib/api/client";
import type { MemberStatus, Role } from "@/lib/mock/team";

/** Mirrors apps/internal/auth.membershipResponse's JSON shape exactly.
 * Not the same type as lib/mock/team.ts's TeamMember — that mock roster
 * still backs the still-mock custom-metrics/content-gallery/referral
 * features' own "who am I" lookup (hooks/use-current-member.ts), which
 * this phase deliberately does not touch (those features aren't wired to
 * a real backend yet; rewiring their identity source ahead of their own
 * backend-integration phase would silently break "is this mine" checks
 * against their still-mock seed data). This is the real Team page's own
 * type instead. */
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
