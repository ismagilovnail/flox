import { describe, expect, it } from "vitest";

import { describeTrigger, splitDelay } from "@/features/push/trigger-format";

describe("splitDelay", () => {
  it("uses the largest whole unit", () => {
    expect(splitDelay(90)).toEqual({ amount: 90, unit: "minutes" });
    expect(splitDelay(120)).toEqual({ amount: 2, unit: "hours" });
    expect(splitDelay(2880)).toEqual({ amount: 2, unit: "days" });
    expect(splitDelay(30 * 24 * 60)).toEqual({ amount: 30, unit: "days" });
  });
});

describe("describeTrigger", () => {
  // A stand-in t: key plus its interpolation values, so the test checks
  // which keys and values are used, not the wording.
  const t = ((key: string, o?: Record<string, unknown>) => `${key}${o ? JSON.stringify(o) : ""}`) as never;

  it("names the delay, and the condition when there is one", () => {
    expect(describeTrigger({ delayMinutes: 60, condition: "none", statuses: [] }, t)).toBe(
      'trigger.after{"delay":"trigger.units.hours{\\"count\\":1}"}',
    );
    expect(describeTrigger({ delayMinutes: 1440, condition: "without", statuses: ["CPA_ACCEPT", "CPA_REDEP"] }, t)).toBe(
      'trigger.after{"delay":"trigger.units.days{\\"count\\":1}"} · trigger.conditionShort.without{"statuses":"CPA_ACCEPT, CPA_REDEP"}',
    );
  });
});
