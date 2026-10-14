import { describe, expect, it } from "vitest";

import { formatInZone, utcToZonedLocal, zonedLocalToUtc } from "@/lib/timezones";

describe("zonedLocalToUtc / utcToZonedLocal", () => {
  it("converts a wall-clock time in a fixed-offset zone", () => {
    expect(zonedLocalToUtc("2026-10-20T09:30", "Asia/Yekaterinburg").toISOString()).toBe("2026-10-20T04:30:00.000Z");
    expect(utcToZonedLocal(new Date("2026-10-20T04:30:00Z"), "Asia/Yekaterinburg")).toBe("2026-10-20T09:30");
  });

  it("uses the offset in force on that date across DST", () => {
    // Berlin: UTC+2 in summer, UTC+1 in winter.
    expect(zonedLocalToUtc("2026-07-01T12:00", "Europe/Berlin").toISOString()).toBe("2026-07-01T10:00:00.000Z");
    expect(zonedLocalToUtc("2026-12-01T12:00", "Europe/Berlin").toISOString()).toBe("2026-12-01T11:00:00.000Z");
    expect(utcToZonedLocal(new Date("2026-12-01T11:00:00Z"), "Europe/Berlin")).toBe("2026-12-01T12:00");
  });

  it("is UTC for UTC", () => {
    expect(zonedLocalToUtc("2026-10-20T00:05", "UTC").toISOString()).toBe("2026-10-20T00:05:00.000Z");
  });
});

describe("formatInZone", () => {
  it("shows the time in the zone, with the zone", () => {
    const s = formatInZone(new Date("2026-10-20T04:30:00Z"), "Asia/Yekaterinburg", "en");
    expect(s).toContain("Oct 20, 2026");
    expect(s).toContain("09:30");
    expect(s).toMatch(/GMT\+5/);
  });
});
