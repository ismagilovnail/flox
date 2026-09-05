/** Organization settings (§30) — real domain type, mirrors
 * apps/internal/auth.OrganizationSettings' JSON shape exactly. Not mock
 * data, so it doesn't live under lib/mock/.
 *
 * TIMEZONES is a curated shortlist for the dropdown, not an exhaustive
 * IANA list — the real backend (auth.Service.UpdateOrganizationSettings)
 * validates against Go's own full tzdata via time.LoadLocation, broader
 * than this list on purpose, so this stays a UI convenience rather than
 * a hard constraint. Same relationship CURRENCIES (lib/countries.ts) has
 * to the backend's own 3-letter-code validation.
 */

export type OrganizationSettings = {
  id: string;
  name: string;
  timezone: string;
  currency: string;
};

export const TIMEZONES = [
  "UTC",
  "America/New_York",
  "America/Los_Angeles",
  "America/Chicago",
  "Europe/London",
  "Europe/Berlin",
  "Asia/Dubai",
  "Asia/Singapore",
];
