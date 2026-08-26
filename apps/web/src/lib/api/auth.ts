import { apiFetch } from "@/lib/api/client";
import type { Role } from "@/lib/mock/team";

/** Mirrors apps/internal/auth.User's JSON shape (userResponse in
 * apps/internal/auth/handler.go) — no password hash, obviously; that
 * field never leaves the Go repository layer in any form. */
export type AuthUser = {
  id: string;
  name: string;
  email: string;
};

export type AuthOrganization = {
  id: string;
  name: string;
};

/** The shared response shape signup/login/acceptInvite all return
 * (apps/internal/auth.authResponse) — a session cookie is set as a
 * side effect of the HTTP response, never present in this body. */
export type AuthResponse = {
  user: AuthUser;
  organization: AuthOrganization;
  role: Role;
};

export type MeResponse = AuthResponse & {
  /** Full permission list for `role`, from role_permissions — for UI
   * gating only (§52: "frontend permissions are only UX"). Every
   * mutating endpoint still checks permissions server-side
   * independently; nothing here should ever be treated as the source
   * of truth for whether an action is actually allowed. */
  permissions: string[];
};

export type SignupInput = {
  organizationName: string;
  name: string;
  email: string;
  password: string;
};

export type LoginInput = {
  email: string;
  password: string;
  /** Only required when the account holds an active membership in more
   * than one org — apps/internal/auth.Service.Login 422s naming this
   * field in that case. Nothing in this app exposes an org switcher, so
   * in practice every real login omits it. */
  organizationId?: string;
};

export type AcceptInviteInput = {
  token: string;
  name: string;
  password: string;
};

export type InvitePreview = {
  organizationName: string;
  email: string;
  role: Role;
};

export function signup(input: SignupInput): Promise<AuthResponse> {
  return apiFetch("/auth/signup", { method: "POST", body: input });
}

export function login(input: LoginInput): Promise<AuthResponse> {
  return apiFetch("/auth/login", { method: "POST", body: input });
}

export function logout(): Promise<void> {
  return apiFetch("/auth/logout", { method: "POST" });
}

export function getMe(): Promise<MeResponse> {
  return apiFetch("/auth/me");
}

export function previewInvite(token: string): Promise<InvitePreview> {
  return apiFetch(`/auth/invites/${encodeURIComponent(token)}`);
}

export function acceptInvite(input: AcceptInviteInput): Promise<AuthResponse> {
  return apiFetch("/auth/accept-invite", { method: "POST", body: input });
}
