/**
 * Team (§30) — members, roles, permissions, activity. Roles and permission
 * keys match §52 (Phase 28 Auth/RBAC) exactly so nothing gets renamed when
 * that phase wires real enforcement — this UI is the reference, not a
 * placeholder vocabulary. Enforcement itself is server-side in Phase 28;
 * here, permissions only describe what the UI *would* gate.
 */

export type Role = "Owner" | "Admin" | "Manager" | "Buyer" | "Analyst" | "Viewer";
export const ROLES: Role[] = ["Owner", "Admin", "Manager", "Buyer", "Analyst", "Viewer"];

export type Permission =
  | "campaign.read"
  | "campaign.write"
  | "analytics.read"
  | "offer.read"
  | "offer.write"
  | "source.read"
  | "source.write"
  | "team.read"
  | "team.write"
  | "settings.write";

export const PERMISSIONS: Permission[] = [
  "campaign.read",
  "campaign.write",
  "analytics.read",
  "offer.read",
  "offer.write",
  "source.read",
  "source.write",
  "team.read",
  "team.write",
  "settings.write",
];

const ALL_PERMISSIONS = [...PERMISSIONS];

export const ROLE_PERMISSIONS: Record<Role, Permission[]> = {
  Owner: ALL_PERMISSIONS,
  Admin: ALL_PERMISSIONS,
  Manager: ["campaign.read", "campaign.write", "analytics.read", "offer.read", "offer.write", "source.read", "source.write", "team.read"],
  Buyer: ["campaign.read", "campaign.write", "analytics.read", "offer.read", "source.read", "source.write"],
  Analyst: ["campaign.read", "analytics.read", "offer.read", "source.read"],
  Viewer: ["campaign.read", "analytics.read"],
};

export type MemberStatus = "active" | "invited" | "suspended";

export type TeamMember = {
  id: string;
  name: string;
  email: string;
  role: Role;
  status: MemberStatus;
  invitedAt: string;
  lastActiveAt: string | null;
};

/** The Owner row matches the mock signed-in user in components/shell/user-menu.tsx —
 * this IS the workspace owner, not a stand-in. */
export const TEAM_MEMBERS: TeamMember[] = [
  {
    id: "mem_owner",
    name: "Nail Ismagilov",
    email: "nailismagilovnick@gmail.com",
    role: "Owner",
    status: "active",
    invitedAt: "2026-01-15T00:00:00Z",
    lastActiveAt: "2026-08-11T09:12:00Z",
  },
  {
    id: "mem_admin",
    name: "Elena Popova",
    email: "elena@floxlink.io",
    role: "Admin",
    status: "active",
    invitedAt: "2026-02-01T00:00:00Z",
    lastActiveAt: "2026-08-10T18:40:00Z",
  },
  {
    id: "mem_manager",
    name: "Marcus Webb",
    email: "marcus@floxlink.io",
    role: "Manager",
    status: "active",
    invitedAt: "2026-02-20T00:00:00Z",
    lastActiveAt: "2026-08-09T14:05:00Z",
  },
  {
    id: "mem_buyer",
    name: "Priya Shah",
    email: "priya@floxlink.io",
    role: "Buyer",
    status: "active",
    invitedAt: "2026-03-12T00:00:00Z",
    lastActiveAt: "2026-08-11T07:55:00Z",
  },
  {
    id: "mem_analyst",
    name: "Tomas Novak",
    email: "tomas@floxlink.io",
    role: "Analyst",
    status: "invited",
    invitedAt: "2026-08-05T00:00:00Z",
    lastActiveAt: null,
  },
  {
    id: "mem_viewer",
    name: "Guest Reviewer",
    email: "guest@partner.example",
    role: "Viewer",
    status: "suspended",
    invitedAt: "2026-04-01T00:00:00Z",
    lastActiveAt: "2026-06-30T11:20:00Z",
  },
];

export type ActivityEntry = {
  id: string;
  memberId: string;
  action: string;
  createdAt: string;
};

export const TEAM_ACTIVITY: ActivityEntry[] = [
  { id: "act_1", memberId: "mem_buyer", action: "Created campaign \"UK Crypto — Push\"", createdAt: "2026-08-11T07:58:00Z" },
  { id: "act_2", memberId: "mem_owner", action: "Archived offer \"CA Crypto — RevShare\"", createdAt: "2026-08-11T06:40:00Z" },
  { id: "act_3", memberId: "mem_admin", action: "Updated network \"MyLead\" postback URL", createdAt: "2026-08-10T18:42:00Z" },
  { id: "act_4", memberId: "mem_manager", action: "Added stream set \"Bot & Proxy Block\" to \"US Sweeps — FB\"", createdAt: "2026-08-10T15:10:00Z" },
  { id: "act_5", memberId: "mem_buyer", action: "Paused campaign \"AU Sweeps — FB\"", createdAt: "2026-08-09T20:05:00Z" },
  { id: "act_6", memberId: "mem_manager", action: "Edited flow \"Primary offer\" weight to 70%", createdAt: "2026-08-09T14:12:00Z" },
  { id: "act_7", memberId: "mem_admin", action: "Invited tomas@floxlink.io as Analyst", createdAt: "2026-08-05T09:00:00Z" },
  { id: "act_8", memberId: "mem_owner", action: "Created network \"Direct advertiser\"", createdAt: "2026-08-01T11:30:00Z" },
  { id: "act_9", memberId: "mem_buyer", action: "Created offer \"DE Dating — CPL\"", createdAt: "2026-07-30T16:20:00Z" },
  { id: "act_10", memberId: "mem_manager", action: "Updated filter on \"Mobile — Tier 1 GEOs\"", createdAt: "2026-07-28T13:45:00Z" },
  { id: "act_11", memberId: "mem_admin", action: "Suspended guest@partner.example", createdAt: "2026-06-30T11:22:00Z" },
  { id: "act_12", memberId: "mem_owner", action: "Changed Marcus Webb's role to Manager", createdAt: "2026-02-20T10:00:00Z" },
];
