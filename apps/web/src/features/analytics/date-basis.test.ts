import { describe, expect, it } from "vitest";

import i18next from "@/lib/i18n/config";

// Tech audit AN5: the "dated by click" notice counts conversions that kept
// their own date; Russian needs all three plural forms.
describe("date basis notice", () => {
  it.each([
    [1, "1 конверсия без атрибутированного клика учтена"],
    [3, "3 конверсии без атрибутированного клика учтены"],
    [5, "5 конверсий без атрибутированного клика учтены"],
    [21, "21 конверсия без атрибутированного клика учтена"],
  ])("ru: %i", (count, start) => {
    const t = i18next.getFixedT("ru", "analytics");
    expect(t("dateBasis.notice.withoutClick", { count })).toMatch(new RegExp("^" + start));
  });

  it("en", () => {
    const t = i18next.getFixedT("en", "analytics");
    expect(t("dateBasis.notice.withoutClick", { count: 1 })).toMatch(/^1 conversion without .* is counted/);
    expect(t("dateBasis.notice.withoutClick", { count: 2 })).toMatch(/^2 conversions without .* are counted/);
  });
});
