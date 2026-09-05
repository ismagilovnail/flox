/**
 * Team (§30) — role/permission vocabulary only. Roles and permission keys
 * match §52 (Phase 28 Auth/RBAC) exactly so nothing gets renamed when that
 * phase wires real enforcement — this UI is the reference, not a
 * placeholder vocabulary. Enforcement itself is server-side in Phase 28;
 * here, permissions only describe what the UI *would* gate.
 *
 * Member roster/activity feed are real — hooks/use-team.ts + lib/api/
 * team.ts (GET /team/members, GET /team/activity) — this file's own
 * TeamMember/TEAM_MEMBERS/ActivityEntry/TEAM_ACTIVITY fixtures were
 * removed once Custom Metrics, Content Gallery, and (last) Referral all
 * migrated off the mock "current member" hook that was their only reader
 * (hooks/use-current-member.ts, itself deleted alongside stores/team.ts —
 * both had zero real consumers left). Role/Permission/ROLES/PERMISSIONS/
 * ROLE_PERMISSIONS/MemberStatus stay — real UI (features/team/*,
 * lib/api/team.ts) still imports this shared vocabulary.
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
