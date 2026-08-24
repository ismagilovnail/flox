/**
 * Pure locale-resolution logic, deliberately kept free of any
 * `i18next`/`react-i18next` import. app/layout.tsx (a Server Component,
 * no "use client") imports resolveLocale/LOCALE_COOKIE directly — Server
 * Components run under React's "react-server" build, which doesn't
 * export client-only APIs like `createContext`; react-i18next creates
 * its own context at module load, so importing anything that pulls
 * react-i18next in (lib/i18n/config.ts does) from a Server Component
 * throws at build/render time. Keeping this file import-free of both
 * keeps it safe to import from either side. lib/i18n/config.ts
 * re-exports everything here, so "use client" call sites are unaffected
 * and can keep importing from config.ts as before.
 */
export const SUPPORTED_LOCALES = ["en", "ru"] as const;
export type Locale = (typeof SUPPORTED_LOCALES)[number];
export const DEFAULT_LOCALE: Locale = "en";
export const LOCALE_COOKIE = "flox-locale";

export function isSupportedLocale(value: string): value is Locale {
  return (SUPPORTED_LOCALES as readonly string[]).includes(value);
}

/** First entry's primary language subtag from an Accept-Language header
 * (e.g. "ru-RU,ru;q=0.9,en;q=0.8" -> "ru") — good enough for a 2-locale
 * app; not full RFC 4647 quality-weighted negotiation across every
 * offered tag. */
function parsePrimaryLanguage(acceptLanguage: string | null | undefined): string | undefined {
  return acceptLanguage?.split(",")[0]?.split(";")[0]?.trim().split("-")[0];
}

/** The single source of truth for "which locale should this request
 * render" — a persisted cookie choice wins; otherwise the browser's own
 * language (Accept-Language), if supported; otherwise DEFAULT_LOCALE.
 * Called server-side only (app/layout.tsx) — the resolved value is then
 * threaded down as a prop, never re-derived client-side. */
export function resolveLocale(cookieValue: string | undefined, acceptLanguage: string | null | undefined): Locale {
  if (cookieValue && isSupportedLocale(cookieValue)) return cookieValue;

  const browserLanguage = parsePrimaryLanguage(acceptLanguage);
  if (browserLanguage && isSupportedLocale(browserLanguage)) return browserLanguage;

  return DEFAULT_LOCALE;
}
