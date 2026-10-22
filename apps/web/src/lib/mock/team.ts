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
 *
 * The tables below are a MANUAL SYNC POINT with the database: the real
 * grants live in migrations 00001/00020 (the original ten keys), 00029
 * (custom_metric), 00030 (report_preset), 00031 (referral) and 00035
 * (content). features/team/roles-permissions-panel.tsx renders this
 * matrix to admins as the org's access policy, so a copy that drifts is
 * a lie told to the person deciding who to invite as what — which is
 * exactly what it had become: it still listed only the original ten keys
 * long after three more pairs were seeded (2026-09-06 audit). Nothing
 * here is enforcement; every mutating endpoint checks server-side
 * independently (apps/api/main.go's require*Write middlewares).
 */

export type Role = "Owner" | "Admin" | "Manager" | "Buyer" | "Analyst" | "Viewer";
export const ROLES: Role[] = ["Owner", "Admin", "Manager", "Buyer", "Analyst", "Viewer"];

export type Permission =
  | "campaign.read"
  | "campaign.write"
  | "campaign.manage_all"
  | "campaign.read_all"
  | "analytics.read"
  | "offer.read"
  | "offer.write"
  | "source.read"
  | "source.write"
  | "content.read"
  | "content.write"
  | "custom_metric.read"
  | "custom_metric.write"
  | "report_preset.read"
  | "report_preset.write"
  | "referral.read"
  | "referral.write"
  | "push.read"
  | "push.write"
  | "team.read"
  | "team.write"
  | "settings.write";

export const PERMISSIONS: Permission[] = [
  "campaign.read",
  "campaign.write",
  "campaign.manage_all",
  "campaign.read_all",
  "analytics.read",
  "offer.read",
  "offer.write",
  "source.read",
  "source.write",
  "content.read",
  "content.write",
  "custom_metric.read",
  "custom_metric.write",
  "report_preset.read",
  "report_preset.write",
  "referral.read",
  "referral.write",
  "push.read",
  "push.write",
  "team.read",
  "team.write",
  "settings.write",
];

const ALL_PERMISSIONS = [...PERMISSIONS];

/** Read keys every role holds — the floor each role below builds on. */
const UNIVERSAL_READS: Permission[] = [
  "campaign.read",
  "analytics.read",
  "content.read",
  "custom_metric.read",
  "report_preset.read",
  "referral.read",
  "push.read",
];

export const ROLE_PERMISSIONS: Record<Role, Permission[]> = {
  Owner: ALL_PERMISSIONS,
  Admin: ALL_PERMISSIONS,
  Manager: [
    ...UNIVERSAL_READS,
    "campaign.write",
    "campaign.manage_all",
    "campaign.read_all",
    "offer.read",
    "offer.write",
    "source.read",
    "source.write",
    "content.write",
    "custom_metric.write",
    "report_preset.write",
    "push.write",
    "team.read",
  ],
  Buyer: [...UNIVERSAL_READS, "campaign.write", "offer.read", "source.read", "source.write"],
  Analyst: [...UNIVERSAL_READS, "campaign.read_all", "offer.read", "source.read"],
  Viewer: [...UNIVERSAL_READS, "campaign.read_all"],
};

export type MemberStatus = "active" | "invited" | "suspended";
