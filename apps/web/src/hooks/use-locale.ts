"use client";

import { useTranslation } from "react-i18next";

import { DEFAULT_LOCALE, isSupportedLocale, type Locale } from "@/lib/i18n/config";

/** The active UI locale, narrowed to a real SUPPORTED_LOCALES member —
 * i18n.language can transiently be an unsupported/full BCP-47 tag (e.g.
 * before the detect-on-mount effect settles it), and lib/format.ts's
 * Intl.* calls need a known key into their own locale maps. */
export function useLocale(): Locale {
  const { i18n } = useTranslation();
  return isSupportedLocale(i18n.language) ? i18n.language : DEFAULT_LOCALE;
}
