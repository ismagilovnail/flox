"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  inviteMember,
  listActivity,
  listMembers,
  removeMember,
  resendInvite,
  updateMember,
  type InviteMemberInput,
  type UpdateMemberInput,
} from "@/lib/api/team";

const membersKey = ["team", "members"] as const;
const activityKey = ["team", "activity"] as const;

export function useTeamMembers() {
  return useQuery({ queryKey: membersKey, queryFn: listMembers });
}

export function useTeamActivity() {
  return useQuery({ queryKey: activityKey, queryFn: listActivity });
}

/** Every mutation below invalidates both members and activity — a
 * membership change always produces a new audit_logs row too (see
 * apps/internal/auth.Service), so the activity feed would otherwise show
 * stale data until some unrelated refetch happened to run. */
function invalidateTeam(qc: ReturnType<typeof useQueryClient>) {
  return Promise.all([
    qc.invalidateQueries({ queryKey: membersKey }),
    qc.invalidateQueries({ queryKey: activityKey }),
  ]);
}

export function useInviteMember() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: InviteMemberInput) => inviteMember(input),
    onSuccess: () => invalidateTeam(qc),
  });
}

export function useUpdateMember() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateMemberInput }) => updateMember(id, input),
    onSuccess: () => invalidateTeam(qc),
  });
}

export function useResendInvite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => resendInvite(id),
    onSuccess: () => invalidateTeam(qc),
  });
}

export function useRemoveMember() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => removeMember(id),
    onSuccess: () => invalidateTeam(qc),
  });
}
