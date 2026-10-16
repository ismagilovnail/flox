import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import type { VisitorReport } from "@/lib/api/visitor-data";

// A visitor's data request (audit 2026-10-15, GDPR-1).
let report: VisitorReport | undefined;
const erase = vi.fn();
const queries: unknown[] = [];
vi.mock("@/hooks/use-visitor-data", () => ({
  useVisitorReport: (q: unknown) => {
    queries.push(q);
    return { data: q ? report : undefined, isFetching: false, isError: false, refetch: vi.fn() };
  },
  useEraseVisitor: () => ({ mutate: erase, isPending: false }),
}));

const { VisitorDataPanel, visitorQuery } = await import("@/features/privacy/visitor-data-panel");

const sample: VisitorReport = {
  clickIds: ["c1", "c2"],
  truncated: false,
  clicks: [
    { at: "2026-10-16T10:00:00Z", type: "SOURCE_CLICK", clickId: "c1", campaignId: "x", country: "DE", city: "", device: "mobile", os: "", browser: "", ip: "203.0.113.5", userAgent: "UA", referrer: "" },
  ],
  events: [],
  conversions: [{ at: "2026-10-16T11:00:00Z", type: "CPA_ACCEPT", clickId: "c1", revenue: 10, currency: "USD" }],
  pushSubscriptions: [],
  telegramUsers: [],
  pixelDeliveries: 3,
};

afterEach(() => {
  report = undefined;
  erase.mockReset();
  queries.length = 0;
});

describe("visitorQuery", () => {
  it("sends an IP address as ip and anything else as a click id", () => {
    expect(visitorQuery(" 203.0.113.5 ")).toEqual({ ip: "203.0.113.5" });
    expect(visitorQuery("2001:db8::1")).toEqual({ ip: "2001:db8::1" });
    expect(visitorQuery("01JABCDEF")).toEqual({ clickId: "01JABCDEF" });
    expect(visitorQuery("  ")).toBeNull();
  });
});

describe("VisitorDataPanel", () => {
  it("says nothing is held when a click id finds nothing", () => {
    report = { ...sample, clickIds: ["gone"], clicks: [], conversions: [], pixelDeliveries: 0 };
    render(<VisitorDataPanel />);
    fireEvent.change(screen.getByLabelText("Click id or IP address"), { target: { value: "gone" } });
    fireEvent.click(screen.getByRole("button", { name: /Find/ }));
    expect(screen.getByText("Nothing found for this visitor.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Erase visitor data/ })).toBeNull();
  });

  it("looks the visitor up and erases only after confirming", () => {
    report = sample;
    render(<VisitorDataPanel />);
    fireEvent.change(screen.getByLabelText("Click id or IP address"), { target: { value: "203.0.113.5" } });
    fireEvent.click(screen.getByRole("button", { name: /Find/ }));
    expect(queries.at(-1)).toEqual({ ip: "203.0.113.5" });
    expect(screen.getByText("Conversions (kept)").nextSibling?.textContent).toBe("1");

    fireEvent.click(screen.getByRole("button", { name: /Erase visitor data/ }));
    expect(erase).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Erase" }));
    expect(erase).toHaveBeenCalledWith(["c1", "c2"], expect.anything());
  });
});
