import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

import "@/lib/i18n/config";

// DATA-6: a range reaching back past the raw-click retention says what is
// missing for that time; within it, or before the plan loads, nothing.
let months: number | undefined = 3;
vi.mock("@/hooks/use-billing", () => ({
  useBilling: () => ({ data: months === undefined ? undefined : { rawRetentionMonths: months } }),
}));

const { RawRetentionNotice, rawRetentionCutoff } = await import("@/features/analytics/raw-retention-notice");

const now = new Date("2026-10-23T12:00:00Z");

afterEach(() => {
  cleanup();
  months = 3;
});

describe("RawRetentionNotice", () => {
  it("cuts off whole months back", () => {
    expect(rawRetentionCutoff(3, now).toISOString()).toBe("2026-07-23T12:00:00.000Z");
  });

  it("shows only for a range older than the retention", () => {
    render(<RawRetentionNotice from={new Date("2026-08-01T00:00:00Z")} variant="report" now={now} />);
    expect(screen.queryByText(/Raw clicks are kept/)).toBeNull();
    cleanup();

    render(<RawRetentionNotice from={new Date("2026-06-01T00:00:00Z")} variant="report" now={now} />);
    expect(screen.getByText("Raw clicks are kept 3 mo.")).toBeTruthy();
    expect(screen.getByText(/region, city, the funnel/)).toBeTruthy();
    cleanup();

    render(<RawRetentionNotice from={new Date("2026-06-01T00:00:00Z")} variant="clickLog" now={now} />);
    expect(screen.getByText(/The log is empty for the start of the range/)).toBeTruthy();
  });

  it("says nothing before the plan loads", () => {
    months = undefined;
    render(<RawRetentionNotice from={new Date("2020-01-01T00:00:00Z")} variant="report" now={now} />);
    expect(screen.queryByText(/Raw clicks are kept/)).toBeNull();
  });
});
