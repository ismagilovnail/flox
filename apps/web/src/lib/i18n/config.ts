/**
 * i18n foundation (docs/frontend-i18n.md). English (`en`) is the source/
 * fallback locale; Russian (`ru`) is the first additional one. Resources
 * are bundled statically (imported JSON, one file per namespace per
 * locale) rather than loaded over the network — the string volume this
 * phase migrates is small enough that async namespace loading would add
 * real complexity (loading states, suspense boundaries) for no benefit;
 * see docs/frontend-i18n.md for how to add a locale/namespace later if
 * that changes.
 *
 * SSR note: the server resolves the active locale per request —
 * resolveLocale, reading the persisted LOCALE_COOKIE then falling back
 * to the Accept-Language header — and renders it directly (app/
 * layout.tsx); the client hydrates against that exact same value,
 * passed down as a plain prop rather than re-derived from anything
 * client-only. See components/i18n-provider.tsx for why this closes the
 * hydration race a prior "always render DEFAULT_LOCALE, flash to the
 * real value after mount" approach could reduce but never fully
 * eliminate (docs/postlanding.md documents that approach and its
 * limits).
 *
 * The pure locale-resolution pieces (resolveLocale, LOCALE_COOKIE,
 * isSupportedLocale, etc.) actually live in lib/i18n/locale.ts, not
 * here, and are just re-exported below — this file unconditionally
 * imports react-i18next, which app/layout.tsx (a Server Component)
 * cannot, so it imports locale.ts directly instead. See that file's own
 * doc comment.
 */
import i18next from "i18next";
import { initReactI18next } from "react-i18next";

import { DEFAULT_LOCALE, isSupportedLocale, LOCALE_COOKIE, resolveLocale, SUPPORTED_LOCALES, type Locale } from "@/lib/i18n/locale";
import enCommon from "@/lib/i18n/locales/en/common.json";
import enNav from "@/lib/i18n/locales/en/nav.json";
import enCampaigns from "@/lib/i18n/locales/en/campaigns.json";
import enCost from "@/lib/i18n/locales/en/cost.json";
import enTrafficSources from "@/lib/i18n/locales/en/trafficSources.json";
import enNetworks from "@/lib/i18n/locales/en/networks.json";
import enLandings from "@/lib/i18n/locales/en/landings.json";
import enPwa from "@/lib/i18n/locales/en/pwa.json";
import enPostlanding from "@/lib/i18n/locales/en/postlanding.json";
import enOffers from "@/lib/i18n/locales/en/offers.json";
import enStreamSets from "@/lib/i18n/locales/en/streamSets.json";
import enRoutingSimulator from "@/lib/i18n/locales/en/routingSimulator.json";
import enConversions from "@/lib/i18n/locales/en/conversions.json";
import enPostbacks from "@/lib/i18n/locales/en/postbacks.json";
import enPixels from "@/lib/i18n/locales/en/pixels.json";

import ruCommon from "@/lib/i18n/locales/ru/common.json";
import ruNav from "@/lib/i18n/locales/ru/nav.json";
import ruCampaigns from "@/lib/i18n/locales/ru/campaigns.json";
import ruCost from "@/lib/i18n/locales/ru/cost.json";
import ruTrafficSources from "@/lib/i18n/locales/ru/trafficSources.json";
import ruNetworks from "@/lib/i18n/locales/ru/networks.json";
import ruLandings from "@/lib/i18n/locales/ru/landings.json";
import ruPwa from "@/lib/i18n/locales/ru/pwa.json";
import ruPostlanding from "@/lib/i18n/locales/ru/postlanding.json";
import ruOffers from "@/lib/i18n/locales/ru/offers.json";
import ruStreamSets from "@/lib/i18n/locales/ru/streamSets.json";
import ruRoutingSimulator from "@/lib/i18n/locales/ru/routingSimulator.json";
import ruConversions from "@/lib/i18n/locales/ru/conversions.json";
import ruPostbacks from "@/lib/i18n/locales/ru/postbacks.json";
import ruPixels from "@/lib/i18n/locales/ru/pixels.json";

// Re-exported so existing "use client" call sites can keep importing
// locale/cookie concerns from this module as before — the actual
// definitions live in lib/i18n/locale.ts, which app/layout.tsx (a
// Server Component) imports directly instead, to avoid pulling
// react-i18next into the server module graph (see locale.ts's doc
// comment).
export { DEFAULT_LOCALE, isSupportedLocale, LOCALE_COOKIE, resolveLocale, SUPPORTED_LOCALES, type Locale };

export const NAMESPACES = [
  "common",
  "nav",
  "campaigns",
  "cost",
  "trafficSources",
  "networks",
  "landings",
  "pwa",
  "postlanding",
  "offers",
  "streamSets",
  "routingSimulator",
  "conversions",
  "postbacks",
  "pixels",
] as const;

const resources = {
  en: {
    common: enCommon,
    nav: enNav,
    campaigns: enCampaigns,
    cost: enCost,
    trafficSources: enTrafficSources,
    networks: enNetworks,
    landings: enLandings,
    pwa: enPwa,
    postlanding: enPostlanding,
    offers: enOffers,
    streamSets: enStreamSets,
    routingSimulator: enRoutingSimulator,
    conversions: enConversions,
    postbacks: enPostbacks,
    pixels: enPixels,
  },
  ru: {
    common: ruCommon,
    nav: ruNav,
    campaigns: ruCampaigns,
    cost: ruCost,
    trafficSources: ruTrafficSources,
    networks: ruNetworks,
    landings: ruLandings,
    pwa: ruPwa,
    postlanding: ruPostlanding,
    offers: ruOffers,
    streamSets: ruStreamSets,
    routingSimulator: ruRoutingSimulator,
    conversions: ruConversions,
    postbacks: ruPostbacks,
    pixels: ruPixels,
  },
};

function initOptions(locale: Locale) {
  return {
    resources,
    lng: locale,
    fallbackLng: DEFAULT_LOCALE,
    defaultNS: "common" as const,
    ns: NAMESPACES,
    interpolation: { escapeValue: false }, // React already escapes interpolated values
    returnEmptyString: false, // an intentionally-empty translation must not silently render as blank
  };
}

/** Builds a fresh, independent i18next instance — never a shared
 * module-level singleton. I18nProvider's render runs once server-side
 * PER REQUEST (Next.js can have multiple requests from different tabs/
 * users in flight on the same Node process) and once client-side per
 * browser tab; a shared mutable global's `.language` would race across
 * concurrent requests resolving to different locales. React's
 * useState(() => createI18nInstance(locale)) lazy-initializer pattern
 * guarantees exactly one instance per component lifetime with nothing
 * shared to race. */
export function createI18nInstance(locale: Locale) {
  const instance = i18next.createInstance();
  void instance.use(initReactI18next).init(initOptions(locale));
  return instance;
}

/** A shared, lazily-initialized default instance for this module's own
 * tests (translation output/pluralization/formatting), where per-
 * request isolation doesn't matter. The running app never uses this
 * instance directly — every render path goes through
 * createI18nInstance. */
if (!i18next.isInitialized) {
  void i18next.use(initReactI18next).init(initOptions(DEFAULT_LOCALE));
}

export default i18next;
