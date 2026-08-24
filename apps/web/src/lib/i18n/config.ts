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
 * SSR note: i18next always initializes with lng = DEFAULT_LOCALE below,
 * so the server render and the FIRST client render agree exactly — no
 * hydration mismatch. I18nProvider (components/i18n-provider.tsx) then
 * switches to the persisted/detected locale in a useEffect, same "flash
 * to the real value after mount" pattern this codebase already uses for
 * theme (components/theme-toggle.tsx's useMounted).
 */
import i18next from "i18next";
import { initReactI18next } from "react-i18next";

import enCommon from "@/lib/i18n/locales/en/common.json";
import enNav from "@/lib/i18n/locales/en/nav.json";
import enCampaigns from "@/lib/i18n/locales/en/campaigns.json";
import enCost from "@/lib/i18n/locales/en/cost.json";
import enTrafficSources from "@/lib/i18n/locales/en/trafficSources.json";
import enNetworks from "@/lib/i18n/locales/en/networks.json";
import enLandings from "@/lib/i18n/locales/en/landings.json";
import enPwa from "@/lib/i18n/locales/en/pwa.json";
import enOffers from "@/lib/i18n/locales/en/offers.json";
import enStreamSets from "@/lib/i18n/locales/en/streamSets.json";
import enRoutingSimulator from "@/lib/i18n/locales/en/routingSimulator.json";
import enConversions from "@/lib/i18n/locales/en/conversions.json";
import enPostbacks from "@/lib/i18n/locales/en/postbacks.json";

import ruCommon from "@/lib/i18n/locales/ru/common.json";
import ruNav from "@/lib/i18n/locales/ru/nav.json";
import ruCampaigns from "@/lib/i18n/locales/ru/campaigns.json";
import ruCost from "@/lib/i18n/locales/ru/cost.json";
import ruTrafficSources from "@/lib/i18n/locales/ru/trafficSources.json";
import ruNetworks from "@/lib/i18n/locales/ru/networks.json";
import ruLandings from "@/lib/i18n/locales/ru/landings.json";
import ruPwa from "@/lib/i18n/locales/ru/pwa.json";
import ruOffers from "@/lib/i18n/locales/ru/offers.json";
import ruStreamSets from "@/lib/i18n/locales/ru/streamSets.json";
import ruRoutingSimulator from "@/lib/i18n/locales/ru/routingSimulator.json";
import ruConversions from "@/lib/i18n/locales/ru/conversions.json";
import ruPostbacks from "@/lib/i18n/locales/ru/postbacks.json";

export const SUPPORTED_LOCALES = ["en", "ru"] as const;
export type Locale = (typeof SUPPORTED_LOCALES)[number];
export const DEFAULT_LOCALE: Locale = "en";
export const LOCALE_STORAGE_KEY = "flox-locale";

export const NAMESPACES = [
  "common",
  "nav",
  "campaigns",
  "cost",
  "trafficSources",
  "networks",
  "landings",
  "pwa",
  "offers",
  "streamSets",
  "routingSimulator",
  "conversions",
  "postbacks",
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
    offers: enOffers,
    streamSets: enStreamSets,
    routingSimulator: enRoutingSimulator,
    conversions: enConversions,
    postbacks: enPostbacks,
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
    offers: ruOffers,
    streamSets: ruStreamSets,
    routingSimulator: ruRoutingSimulator,
    conversions: ruConversions,
    postbacks: ruPostbacks,
  },
};

export function isSupportedLocale(value: string): value is Locale {
  return (SUPPORTED_LOCALES as readonly string[]).includes(value);
}

/** Persisted choice (localStorage) wins; otherwise the browser's own
 * language, if supported; otherwise DEFAULT_LOCALE. Returns
 * DEFAULT_LOCALE outright during SSR (no window) — callers only need
 * this after mount anyway. */
export function detectInitialLocale(): Locale {
  if (typeof window === "undefined") return DEFAULT_LOCALE;

  const stored = window.localStorage.getItem(LOCALE_STORAGE_KEY);
  if (stored && isSupportedLocale(stored)) return stored;

  const browserLanguage = window.navigator.language?.split("-")[0];
  if (browserLanguage && isSupportedLocale(browserLanguage)) return browserLanguage;

  return DEFAULT_LOCALE;
}

if (!i18next.isInitialized) {
  void i18next.use(initReactI18next).init({
    resources,
    lng: DEFAULT_LOCALE,
    fallbackLng: DEFAULT_LOCALE,
    defaultNS: "common",
    ns: NAMESPACES,
    interpolation: { escapeValue: false }, // React already escapes interpolated values
    returnEmptyString: false, // an intentionally-empty translation must not silently render as blank
  });
}

export default i18next;
