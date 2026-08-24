import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import i18n, {
  DEFAULT_LOCALE,
  LOCALE_STORAGE_KEY,
  SUPPORTED_LOCALES,
  detectInitialLocale,
  isSupportedLocale,
} from "@/lib/i18n/config";
import { formatInt, formatPercent, formatUsd } from "@/lib/format";
import enCommonBundle from "@/lib/i18n/locales/en/common.json";

describe("i18n config", () => {
  beforeEach(async () => {
    window.localStorage.clear();
    await i18n.changeLanguage(DEFAULT_LOCALE);
  });

  afterEach(async () => {
    window.localStorage.clear();
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

  it("detectInitialLocale reads a persisted locale from localStorage", () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, "ru");
    expect(detectInitialLocale()).toBe("ru");
  });

  it("detectInitialLocale falls back to English for an unsupported persisted value", () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, "fr");
    expect(detectInitialLocale()).toBe("en");
  });

  it("detectInitialLocale falls back to English when nothing is persisted and the browser language is unsupported", () => {
    const spy = vi.spyOn(window.navigator, "language", "get").mockReturnValue("fr-FR");
    expect(detectInitialLocale()).toBe("en");
    spy.mockRestore();
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
