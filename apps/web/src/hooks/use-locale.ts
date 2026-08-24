"use client";

import { useTranslation } from "react-i18next";

import { DEFAULT_LOCALE, isSupportedLocale, type Locale } from "@/lib/i18n/config";

/** The active UI locale, narrowed to a real SUPPORTED_LOCALES member —
 * i18n.language is always resolved server-side to one already
 * (lib/i18n/config's resolveLocale), but lib/format.ts's Intl.* calls
 * need a statically-typed key into their own locale maps, and this
 * guard is cheap insurance against a future caller passing an
 * unresolved instance. */
export function useLocale(): Locale {
  const { i18n } = useTranslation();
  return isSupportedLocale(i18n.language) ? i18n.language : DEFAULT_LOCALE;
}
