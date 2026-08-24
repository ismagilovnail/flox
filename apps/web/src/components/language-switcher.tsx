"use client";

import { useTranslation } from "react-i18next";
import { LanguagesIcon } from "lucide-react";

import { SUPPORTED_LOCALES, isSupportedLocale, type Locale } from "@/lib/i18n/config";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

/** Small, unobtrusive selector for the topbar — language names, never
 * flags (§9/accessibility), keyboard-navigable via the underlying Radix
 * Select. i18n.language is always resolved server-side to a
 * SUPPORTED_LOCALES member already (lib/i18n/config's resolveLocale);
 * the isSupportedLocale guard below is cheap insurance, not a real
 * fallback path. */
export function LanguageSwitcher() {
  const { t, i18n } = useTranslation("common");

  const current = isSupportedLocale(i18n.language) ? i18n.language : "en";

  return (
    <Select value={current} onValueChange={(value) => void i18n.changeLanguage(value)}>
      <SelectTrigger
        size="sm"
        aria-label={t("language.switcherLabel")}
        className="w-auto gap-1.5 border-transparent px-2 shadow-none hover:bg-muted"
      >
        <LanguagesIcon className="size-3.5 text-muted-foreground" />
        <SelectValue>{t(`language.${current}` as const)}</SelectValue>
      </SelectTrigger>
      <SelectContent align="end">
        {SUPPORTED_LOCALES.map((locale: Locale) => (
          <SelectItem key={locale} value={locale}>
            {t(`language.${locale}` as const)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
