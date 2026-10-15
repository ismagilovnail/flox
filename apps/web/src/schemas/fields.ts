import { z } from "zod";

/**
 * Field rules several forms share, each mirroring the API's own check so a
 * form refuses what the server would. A form's own schema (its fields, its
 * messages) stays in its feature; only the rule lives here.
 */

/** Names of things (campaigns, offers, API keys, members…): the API refuses
 * fewer than 2 characters (e.g. apikey.Service, custommetrics.Service,
 * push.Service, auth's organization settings). */
export const MIN_NAME_LENGTH = 2;

/** auth.minPasswordLength (apps/internal/auth/service.go). */
export const MIN_PASSWORD_LENGTH = 8;

/**
 * A name: at least MIN_NAME_LENGTH characters, at most `max` (each entity's
 * own limit). `trim` drops surrounding spaces before checking, for the
 * forms whose API trims too.
 */
export function nameField(message: string, { max, maxMessage, trim = false }: { max: number; maxMessage?: string; trim?: boolean }) {
  const base = trim ? z.string().trim() : z.string();
  return base.min(MIN_NAME_LENGTH, message).max(max, maxMessage);
}

/** A new password (sign-up, accepting an invite). Signing in only requires one. */
export function newPasswordField(message: string) {
  return z.string().min(MIN_PASSWORD_LENGTH, message);
}

export function emailField(message: string) {
  return z.email(message);
}
