/**
 * Security settings (§30) — real backend: apps/internal/auth's
 * SecurityHandler + TOTPHandler. Session timeout, IP allowlist, and the
 * org-wide "require 2FA" policy toggle all live on the same
 * `organizations` row Organization settings does; real per-user TOTP
 * enrollment is a separate, personal-account concern (see
 * hooks/use-auth.ts's TOTP hooks) surfaced on the same Settings >
 * Security page.
 *
 * None of this is specified anywhere in FLOX-master-prompt-v3.md
 * (confirmed by grep before the backend was built) — this vocabulary was
 * new product design from the start, not moved out of a pre-existing
 * mock (lib/mock/settings.ts's own SecuritySettings/SECURITY_SETTINGS
 * were trimmed away once this became real).
 */

export type SecuritySettings = {
  sessionTimeoutMinutes: number;
  ipAllowlist: string[];
  twoFactorRequired: boolean;
};
