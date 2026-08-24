import { afterEach, beforeEach, describe, expect, it } from "vitest";

import i18n, {
  DEFAULT_LOCALE,
  LOCALE_COOKIE,
  SUPPORTED_LOCALES,
  createI18nInstance,
  isSupportedLocale,
  resolveLocale,
} from "@/lib/i18n/config";
import { formatInt, formatPercent, formatUsd } from "@/lib/format";
import enCommonBundle from "@/lib/i18n/locales/en/common.json";

describe("i18n config", () => {
  beforeEach(async () => {
    await i18n.changeLanguage(DEFAULT_LOCALE);
  });

  afterEach(async () => {
    await i18n.changeLanguage(DEFAULT_LOCALE);
  });

  it("defaults to English", () => {
    expect(i18n.language).toBe("en");
    expect(i18n.t("actions.save", { ns: "common" })).toBe("Save");
  });

  it("supports exactly en and ru", () => {
    expect(SUPPORTED_LOCALES).toEqual(["en", "ru"]);
    expect(isSupportedLocale("en")).toBe(true);
    expect(isSupportedLocale("ru")).toBe(true);
    expect(isSupportedLocale("fr")).toBe(false);
  });

  it("exposes the cookie name app/layout.tsx and components/i18n-provider.tsx both read/write", () => {
    expect(LOCALE_COOKIE).toBe("flox-locale");
  });

  it("switches en -> ru and back, updating translated output", async () => {
    expect(i18n.t("actions.save", { ns: "common" })).toBe("Save");

    await i18n.changeLanguage("ru");
    expect(i18n.language).toBe("ru");
    expect(i18n.t("actions.save", { ns: "common" })).toBe("Сохранить");

    await i18n.changeLanguage("en");
    expect(i18n.t("actions.save", { ns: "common" })).toBe("Save");
  });

  it("falls back to English for a key an unsupported/mistyped language has no bundle for", async () => {
    // Simulates a corrupted persisted value reaching i18next directly
    // (bypassing detectInitialLocale's own guard) — the library-level
    // safety net, distinct from and in addition to that guard.
    await i18n.changeLanguage("fr");
    expect(i18n.t("actions.save", { ns: "common" })).toBe("Save");
  });

  it("falls back to English for a key genuinely missing from the Russian bundle", async () => {
    i18n.addResourceBundle("en", "common", { __test: { onlyInEnglish: "English only" } }, true, true);
    await i18n.changeLanguage("ru");

    expect(i18n.t("__test.onlyInEnglish", { ns: "common" })).toBe("English only");

    i18n.removeResourceBundle("en", "common");
    // re-attach the real bundle removed above (removeResourceBundle clears
    // the whole namespace, not just the injected test key)
    i18n.addResourceBundle("en", "common", enCommonBundle, true, true);
  });

  it("interpolates dynamic values", () => {
    const text = i18n.t("dataTable.pagination", { ns: "common", page: 2, total: 5, count: 42 });
    expect(text).toBe("Page 2 of 5 · 42 rows");
  });

  it("pluralizes in English (one/other)", () => {
    expect(i18n.t("dataTable.selected", { ns: "common", count: 1 })).toBe("1 selected");
    expect(i18n.t("dataTable.selected", { ns: "common", count: 5 })).toBe("5 selected");
  });

  it("pluralizes in Russian (one/few/many, CLDR plural rules)", async () => {
    await i18n.changeLanguage("ru");
    expect(i18n.t("dataTable.selected", { ns: "common", count: 1 })).toBe("Выбрана 1 строка");
    expect(i18n.t("dataTable.selected", { ns: "common", count: 3 })).toBe("Выбрано 3 строки");
    expect(i18n.t("dataTable.selected", { ns: "common", count: 5 })).toBe("Выбрано 5 строк");
  });
});

describe("resolveLocale (server-side resolution — cookie, then Accept-Language, then default)", () => {
  it("a valid cookie wins outright", () => {
    expect(resolveLocale("ru", "en-US,en;q=0.9")).toBe("ru");
  });

  it("falls through an unsupported/corrupted cookie value to Accept-Language", () => {
    expect(resolveLocale("fr", "ru-RU,ru;q=0.9")).toBe("ru");
  });

  it("falls back to Accept-Language when there is no cookie", () => {
    expect(resolveLocale(undefined, "ru-RU,ru;q=0.9,en;q=0.8")).toBe("ru");
  });

  it("falls back to DEFAULT_LOCALE when neither cookie nor Accept-Language is supported/present", () => {
    expect(resolveLocale(undefined, "fr-FR,fr;q=0.9")).toBe(DEFAULT_LOCALE);
    expect(resolveLocale(undefined, undefined)).toBe(DEFAULT_LOCALE);
    expect(resolveLocale(undefined, null)).toBe(DEFAULT_LOCALE);
  });

  it("parses only the first Accept-Language entry's primary subtag, ignoring quality values", () => {
    expect(resolveLocale(undefined, "en-US;q=0.9,ru;q=0.8")).toBe("en");
  });
});

describe("createI18nInstance (per-render instance isolation)", () => {
  it("returns an instance already set to the requested locale", () => {
    const ru = createI18nInstance("ru");
    expect(ru.language).toBe("ru");
    expect(ru.t("actions.save", { ns: "common" })).toBe("Сохранить");
  });

  // The whole point of createI18nInstance over a shared singleton: two
  // instances (standing in for two concurrent requests resolving to
  // different locales) must never observe each other's language.
  it("two instances are fully independent — changing one never affects the other", async () => {
    const en = createI18nInstance("en");
    const ru = createI18nInstance("ru");

    expect(en.language).toBe("en");
    expect(ru.language).toBe("ru");

    await en.changeLanguage("ru");
    expect(en.language).toBe("ru");
    expect(ru.language).toBe("ru"); // unaffected by en's change

    await ru.changeLanguage("en");
    expect(ru.language).toBe("en");
    expect(en.language).toBe("ru"); // unaffected by ru's change
  });

  it("is independent of the module's own shared default instance", async () => {
    const instance = createI18nInstance("ru");
    await i18n.changeLanguage("en"); // the shared default instance, unrelated to `instance`
    expect(instance.language).toBe("ru");
  });
});

describe("locale-aware formatting (lib/format.ts)", () => {
  it("formats USD with the same value and currency across locales, different grouping/decimal marks", () => {
    const en = formatUsd(14655.87, 2, "en");
    const ru = formatUsd(14655.87, 2, "ru");
    expect(en).toBe("$14,655.87");
    // Russian formatting: (non-breaking) space thousands separator, comma
    // decimal, symbol after the value — \s in regex matches U+00A0 too.
    expect(ru).toMatch(/14\s655,87/);
    expect(ru).toContain("$");
  });

  it("formats integers with locale-appropriate grouping", () => {
    expect(formatInt(1234567, "en")).toBe("1,234,567");
    expect(formatInt(1234567, "ru")).toMatch(/1\s234\s567/);
  });

  it("formats percentages consistently, value unchanged by locale", () => {
    const en = formatPercent(12.3, "en");
    const ru = formatPercent(12.3, "ru");
    expect(en).toBe("12.3%");
    expect(ru).toContain("12,3");
    expect(ru).toContain("%");
  });

  it("defaults to en-US formatting when no locale is passed (unmigrated call sites keep their exact prior behavior)", () => {
    expect(formatUsd(100)).toBe("$100");
    expect(formatInt(1000)).toBe("1,000");
  });
});
