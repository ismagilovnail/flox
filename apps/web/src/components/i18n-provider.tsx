"use client";

import * as React from "react";
import { I18nextProvider } from "react-i18next";

import { createI18nInstance, LOCALE_COOKIE, type Locale } from "@/lib/i18n/config";

/**
 * Mirrors ThemeProvider's shape (components/theme-provider.tsx): a thin
 * wrapper so the rest of the app never imports i18next directly.
 *
 * initialLocale is resolved server-side (app/layout.tsx, via
 * lib/i18n/config's resolveLocale reading the persisted LOCALE_COOKIE /
 * Accept-Language header) and passed down as a plain prop — the server
 * and the client's FIRST render both use this exact same value, so
 * there is no "flash to the real value after mount" step left to race
 * against a Suspense-deferred hydration commit. That race was the root
 * cause of a real, reproduced "Hydration failed" error on every page
 * whose list component calls useSearchParams() inside a <Suspense>
 * boundary (Landings/PWA/Postlanding, for the Content Gallery
 * ?gallery=<id> integration): a post-mount i18n.changeLanguage() call
 * notifying every useTranslation() subscriber could fire before such a
 * boundary's own deferred hydration commit, hydrating it against a
 * language the server never rendered. A fixed delay, React.startTransition,
 * and requestIdleCallback were all tried, in that order, as the delay
 * before changeLanguage() — each reduced the race without closing it
 * (requestIdleCallback fires once the main thread is idle, which in
 * practice, not by guarantee, usually means deferred hydration has
 * drained), confirmed still reproducible on both /landings and
 * /postlanding during the Postlanding phase's manual testing (see
 * docs/postlanding.md). Reading the locale from a cookie server-side
 * instead removes the step being raced against entirely, rather than
 * tuning its timing.
 *
 * createI18nInstance builds a NEW instance per call, not a shared
 * singleton — see its own doc comment in lib/i18n/config.ts for why.
 * useState's lazy initializer runs exactly once per component lifetime
 * (once during this server render, independently once again during the
 * client's hydration render), so each render path gets its own instance
 * with nothing shared to race.
 */
export function I18nProvider({ children, initialLocale }: { children: React.ReactNode; initialLocale: Locale }) {
  const [instance] = React.useState(() => createI18nInstance(initialLocale));

  React.useEffect(() => {
    // <html lang> for this exact instance's initial value is already
    // correct — the server rendered it directly (app/layout.tsx). This
    // only needs to run for a LIVE language change after mount (the
    // switcher's i18n.changeLanguage() call), never for the initial
    // render, so it can never race anything hydration-related.
    function onLanguageChanged(lng: string) {
      document.documentElement.lang = lng;
      document.cookie = `${LOCALE_COOKIE}=${lng}; path=/; max-age=31536000; samesite=lax`;
    }
    instance.on("languageChanged", onLanguageChanged);
    return () => {
      instance.off("languageChanged", onLanguageChanged);
    };
  }, [instance]);

  return <I18nextProvider i18n={instance}>{children}</I18nextProvider>;
}
