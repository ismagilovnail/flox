import { describe, expect, it } from "vitest";

import { describeRecurrence, weekdayName } from "@/features/push/recurrence-format";

describe("recurrence format", () => {
  it("names ISO weekdays from Monday", () => {
    expect(weekdayName(1, "en")).toBe("Mon");
    expect(weekdayName(7, "en")).toBe("Sun");
  });

  const t = ((key: string, o?: Record<string, unknown>) => `${key}${o ? JSON.stringify(o) : ""}`) as never;

  it("lists days in order, or says every day", () => {
    expect(describeRecurrence({ weekdays: [5, 1, 3], time: "19:00", until: "" }, t, "en")).toBe("Mon, Wed, Fri · 19:00");
    expect(describeRecurrence({ weekdays: [1, 2, 3, 4, 5, 6, 7], time: "09:30", until: "2026-12-31" }, t, "en")).toBe(
      'recurrence.everyDay · 09:30 · recurrence.until{"date":"2026-12-31"}',
    );
  });
});
