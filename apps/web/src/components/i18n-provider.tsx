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
 *
 * requestIdleCallback below matters here, not just style: a page whose
 * list component calls useSearchParams() (the Content Gallery
 * `?gallery=<id>` integration — Landings, PWA, Postlanding) must sit
 * inside a <Suspense> boundary (Next.js's own requirement), and
 * Suspense-wrapped content can hydrate on a separate, deferred commit
 * from the rest of the tree. Calling changeLanguage() synchronously (or
 * even via a fixed setTimeout/startTransition — both tried and both
 * still raced) in this effect can fire, and notify every
 * useTranslation() subscriber, before such a boundary's own deferred
 * hydration commit has happened. That boundary then hydrates against the
 * NEW language while the server sent HTML for the default one, and React
 * logs a real "Hydration failed" error (auto-recovered by discarding and
 * re-rendering that subtree, but a genuine defect, not a cosmetic one) —
 * reproduced live on both /landings and /pwa, and confirmed to depend on
 * timing (a long fixed delay avoided it, a short one didn't — exactly
 * what a race predicts, and exactly why a fixed delay is the wrong fix:
 * there's no duration that's both provably safe and not a user-visible
 * stall). requestIdleCallback runs only once the browser's main thread
 * is actually idle — which in practice means React's own scheduler,
 * including any deferred Suspense hydration commits, has drained its
 * queue — instead of gambling on a guessed constant. Safari has no
 * requestIdleCallback; setTimeout is its fallback (Safari also has no
 * Suspense-hydration-deferral behavior to race in the first place, since
 * it lacks the concurrent-rendering machinery that causes this).
 */
export function I18nProvider({ children }: { children: React.ReactNode }) {
  React.useEffect(() => {
    function onLanguageChanged(lng: string) {
      document.documentElement.lang = lng;
      window.localStorage.setItem(LOCALE_STORAGE_KEY, lng);
    }
    i18n.on("languageChanged", onLanguageChanged);

    const initial = detectInitialLocale();
    let cancel = () => {};
    if (initial !== i18n.language) {
      const run = () => void i18n.changeLanguage(initial);
      if (typeof window.requestIdleCallback === "function") {
        const handle = window.requestIdleCallback(run);
        cancel = () => window.cancelIdleCallback(handle);
      } else {
        const timer = setTimeout(run, 0);
        cancel = () => clearTimeout(timer);
      }
    } else {
      document.documentElement.lang = initial;
    }

    return () => {
      i18n.off("languageChanged", onLanguageChanged);
      cancel();
    };
  }, []);

  return <I18nextProvider i18n={i18n}>{children}</I18nextProvider>;
}
