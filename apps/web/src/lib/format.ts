import type { Locale } from "@/lib/i18n/config";

/**
 * A locale change must never alter the underlying numeric value or
 * currency identity (docs/frontend-i18n.md) — only digit grouping,
 * decimal separator, and symbol placement change with `locale`. Callers
 * that don't pass one keep this file's original behavior (pinned to
 * "en-US") exactly as it was before i18n existed, so any not-yet-migrated
 * call site is unaffected.
 */
const INTL_LOCALE: Record<Locale, string> = { en: "en-US", ru: "ru-RU" };

export function formatUsd(n: number, maximumFractionDigits = 0, locale: Locale = "en") {
  return n.toLocaleString(INTL_LOCALE[locale], {
    style: "currency",
    currency: "USD",
    maximumFractionDigits,
  });
}

export function formatInt(n: number, locale: Locale = "en") {
  return Math.round(n).toLocaleString(INTL_LOCALE[locale]);
}

/** Locale-aware percentage — e.g. "12.3%" (en) vs "12,3 %" (ru, per Russian
 * typographic convention: a comma decimal separator and a space before %). */
export function formatPercent(n: number, locale: Locale = "en", maximumFractionDigits = 1) {
  return (n / 100).toLocaleString(INTL_LOCALE[locale], {
    style: "percent",
    maximumFractionDigits,
  });
}
