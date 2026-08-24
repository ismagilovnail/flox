"use client";

import * as React from "react";
import { I18nextProvider } from "react-i18next";

import i18n, { LOCALE_STORAGE_KEY, detectInitialLocale } from "@/lib/i18n/config";

/**
 * Mirrors ThemeProvider's shape (components/theme-provider.tsx): a thin
 * wrapper so the rest of the app never imports i18next directly.
 *
 * i18next always starts at DEFAULT_LOCALE ("en") — see lib/i18n/config.ts's
 * doc comment for why. This effect runs once, after mount, and switches to
 * whatever detectInitialLocale() resolves (persisted choice, then browser
 * language, then "en") — the one and only place that decides the app's
 * actual starting locale. Every subsequent change (the language switcher)
 * both updates <html lang> and persists to localStorage here too, so there
 * is exactly one place either happens, not one per call site.
 */
export function I18nProvider({ children }: { children: React.ReactNode }) {
  React.useEffect(() => {
    const initial = detectInitialLocale();
    if (initial !== i18n.language) {
      void i18n.changeLanguage(initial);
    } else {
      document.documentElement.lang = initial;
    }

    function onLanguageChanged(lng: string) {
      document.documentElement.lang = lng;
      window.localStorage.setItem(LOCALE_STORAGE_KEY, lng);
    }
    i18n.on("languageChanged", onLanguageChanged);
    return () => {
      i18n.off("languageChanged", onLanguageChanged);
    };
  }, []);

  return <I18nextProvider i18n={i18n}>{children}</I18nextProvider>;
}
