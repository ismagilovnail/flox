import { create } from "zustand";

import { genId } from "@/lib/id";
import { TEAM_MEMBERS, type MemberStatus, type Role, type TeamMember } from "@/lib/mock/team";

export type InviteMemberInput = {
  name: string;
  email: string;
  role: Role;
};

type TeamState = {
  members: TeamMember[];
  getById: (id: string) => TeamMember | undefined;
  inviteMember: (input: InviteMemberInput) => string;
  updateRole: (id: string, role: Role) => void;
  setStatus: (id: string, status: MemberStatus) => void;
  resendInvite: (id: string) => void;
  removeMember: (id: string) => void;
};

export const useTeamStore = create<TeamState>()((set, get) => ({
  members: [...TEAM_MEMBERS],

  getById: (id) => get().members.find((m) => m.id === id),

  inviteMember: (input) => {
    const id = genId();
    const member: TeamMember = {
      id,
      ...input,
      status: "invited",
      invitedAt: new Date().toISOString(),
      lastActiveAt: null,
    };
    set((s) => ({ members: [...s.members, member] }));
    return id;
  },

  updateRole: (id, role) => {
    set((s) => ({ members: s.members.map((m) => (m.id === id ? { ...m, role } : m)) }));
  },

  setStatus: (id, status) => {
    set((s) => ({ members: s.members.map((m) => (m.id === id ? { ...m, status } : m)) }));
  },

  resendInvite: (id) => {
    set((s) => ({
      members: s.members.map((m) => (m.id === id ? { ...m, invitedAt: new Date().toISOString() } : m)),
    }));
  },

  removeMember: (id) => {
    set((s) => ({ members: s.members.filter((m) => m.id !== id) }));
  },
}));
