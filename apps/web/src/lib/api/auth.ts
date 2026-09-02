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

/** Signup no longer returns a session (apps/internal/auth.Service.Signup's
 * own comment: the account can't sign in until it verifies the email this
 * call sends a link to) — this is deliberately NOT AuthResponse, there is
 * no `role`/session to report yet. */
export type SignupResponse = {
  user: AuthUser;
  organization: AuthOrganization;
};

export type VerifyEmailResponse = {
  verified: boolean;
  /** True only when the account had exactly one active org membership to
   * auto-log into (apps/internal/auth.Service.VerifyEmail's own comment
   * — true for the overwhelming majority of real verifications). `auth`
   * is present exactly when this is true; the session cookie is already
   * set as a side effect of the response by then. */
  loggedIn: boolean;
  auth?: AuthResponse;
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

export function signup(input: SignupInput): Promise<SignupResponse> {
  return apiFetch("/auth/signup", { method: "POST", body: input });
}

export function verifyEmail(token: string): Promise<VerifyEmailResponse> {
  return apiFetch("/auth/verify-email", { method: "POST", body: { token } });
}

/** Always resolves — apps/internal/auth.Service.ResendVerification is
 * deliberately silent about whether the email exists/was already
 * verified/was actually re-sent (anti-enumeration), so there's no
 * distinct failure branch for a caller to handle here beyond a genuine
 * network/server error. */
export function resendVerification(email: string): Promise<{ message: string }> {
  return apiFetch("/auth/resend-verification", { method: "POST", body: { email } });
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
