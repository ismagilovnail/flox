import { describe, expect, it } from "vitest";

import { fillDays } from "@/features/dashboard/metrics";

// BUG-7: the overview's charts get every day of the range, not only the
// days the API has rows for.
describe("fillDays", () => {
  const day = (s: string) => new Date(`${s}T00:00:00Z`);

  it("fills the days without data and keeps the ones with it", () => {
    const out = fillDays([{ day: "2026-10-15", metrics: { revenue: 90 } }], day("2026-10-13"), day("2026-10-16"));
    expect(out).toEqual([
      { day: "2026-10-13", metrics: {} },
      { day: "2026-10-14", metrics: {} },
      { day: "2026-10-15", metrics: { revenue: 90 } },
      { day: "2026-10-16", metrics: {} },
    ]);
  });

  it("gives a single day for a one-day range", () => {
    expect(fillDays([], day("2026-10-16"), day("2026-10-16"))).toEqual([{ day: "2026-10-16", metrics: {} }]);
  });

  it("covers 30 days for the default range, across a month end", () => {
    const out = fillDays([], day("2026-09-17"), day("2026-10-16"));
    expect(out).toHaveLength(30);
    expect(out[13].day).toBe("2026-09-30");
    expect(out[14].day).toBe("2026-10-01");
  });
});
