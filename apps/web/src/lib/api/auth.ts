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
  /** Optional (§30.7) — attributes this signup to whoever's referral link
   * it came through. Invalid/unknown codes are a silent no-op server-side,
   * never a validation error on this form. */
  referralCode?: string;
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

/** What /auth/login, /auth/accept-invite, and (when it auto-logs in)
 * /auth/verify-email all actually resolve to now that §30 Security's
 * real 2FA is live — a full AuthResponse is only one of three real
 * shapes the same 200 response can carry:
 *   - `user`/`organization`/`role` present, `pendingToken` absent: no
 *     second factor needed, a session cookie is already set.
 *   - `pendingToken` present, `user` absent: `requiresTotp` (this
 *     account already has TOTP — exchange the token at
 *     POST /auth/login/totp) or `requiresTotpEnrollment` (the org's own
 *     policy requires 2FA and this account hasn't set it up yet —
 *     exchange the token via enrollTOTP/confirmTOTP instead). Neither
 *     flag is ever true at the same time as the other.
 * Fields are simply absent from whichever branch doesn't apply — this
 * mirrors the real Go JSON exactly rather than inventing a discriminant
 * tag the backend doesn't send. */
export type LoginOutcome = {
  user?: AuthUser;
  organization?: AuthOrganization;
  role?: Role;
  requiresTotp?: boolean;
  requiresTotpEnrollment?: boolean;
  pendingToken?: string;
};

export function isPendingTOTP(
  outcome: LoginOutcome,
): outcome is { requiresTotp: boolean; requiresTotpEnrollment: boolean; pendingToken: string } {
  return outcome.pendingToken !== undefined;
}

export type AcceptInviteInput = {
  token: string;
  name: string;
  password: string;
};

/** A real per-user TOTP enrollment — shown to the user exactly once,
 * immediately after POST /auth/2fa/enroll. */
export type TOTPEnrollment = {
  secret: string;
  qrCodeDataUri: string;
  backupCodes: string[];
};

/** POST /auth/2fa/confirm resolves differently depending on how it was
 * reached (see apps/internal/auth.TOTPHandler.confirm's own doc
 * comment): confirming a forced enrollment mid-login completes that
 * login and returns a real AuthResponse; a self-serve confirm (an
 * already-signed-in user opting in) just flips 2FA on and returns
 * `{enabled: true}`. */
export type ConfirmTOTPResponse = { enabled: true } | AuthResponse;

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

export function login(input: LoginInput): Promise<LoginOutcome> {
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

export function acceptInvite(input: AcceptInviteInput): Promise<LoginOutcome> {
  return apiFetch("/auth/accept-invite", { method: "POST", body: input });
}

/** pendingToken is set only for a forced-enrollment-mid-login (no session
 * exists yet); omitted for a self-serve opt-in by an already-signed-in
 * user, who is instead identified by their session cookie. */
export function enrollTOTP(pendingToken?: string): Promise<TOTPEnrollment> {
  return apiFetch("/auth/2fa/enroll", { method: "POST", body: pendingToken ? { pendingToken } : undefined });
}

export function confirmTOTP(input: { code: string; pendingToken?: string }): Promise<ConfirmTOTPResponse> {
  return apiFetch("/auth/2fa/confirm", { method: "POST", body: input });
}

export function loginTOTP(pendingToken: string, code: string): Promise<AuthResponse> {
  return apiFetch("/auth/login/totp", { method: "POST", body: { pendingToken, code } });
}

export function totpStatus(): Promise<{ enabled: boolean }> {
  return apiFetch("/auth/2fa/status");
}

export function disableTOTP(password: string): Promise<void> {
  return apiFetch("/auth/2fa/disable", { method: "POST", body: { password } });
}
