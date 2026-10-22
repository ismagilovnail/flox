import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import type { BillingStatus, Plan } from "@/lib/api/billing";

// Plan and usage (BIZ-1): the banner shows from 80% of the month's clicks
// only; the Plan tab shows the clicks, the counts against their limits and
// every plan, with "—" when the clicks cannot be read.
const start: Plan = {
  code: "start", name: "Start", monthlyClicks: 1_000_000, maxCampaigns: 50, maxDomains: 5, maxMembers: 5, priceCents: 4900, currency: "USD",
};
const business: Plan = {
  code: "business", name: "Business", monthlyClicks: 20_000_000, maxCampaigns: null, maxDomains: null, maxMembers: null, priceCents: 39900, currency: "USD",
};
function status(level: BillingStatus["level"], clicks: number | null): BillingStatus {
  return {
    plan: start, since: "2026-10-01T00:00:00Z", period: { from: "2026-10-01", to: "2026-10-31" },
    usage: { clicks, campaigns: 50, domains: 2, members: 3 }, level,
  };
}
let billing: BillingStatus | undefined;
vi.mock("@/hooks/use-billing", () => ({
  useBilling: () => ({ data: billing, isPending: !billing, isError: false }),
  usePlans: () => ({ data: { plans: [start, business] }, isPending: false, isError: false }),
}));

const { PlanBanner } = await import("@/components/shell/plan-banner");
const { BillingPanel } = await import("@/features/settings/billing-panel");

afterEach(() => {
  cleanup();
  billing = undefined;
});

describe("PlanBanner", () => {
  it("stays hidden within the plan and when usage is unknown", () => {
    for (const s of [undefined, status("ok", 10), status("unknown", null)]) {
      billing = s;
      const { container } = render(<PlanBanner />);
      expect(container.textContent).toBe("");
      cleanup();
    }
  });

  it("warns from 80% and alerts from 100%", () => {
    billing = status("warning", 850_000);
    render(<PlanBanner />);
    expect(screen.getByRole("status").textContent).toContain("Over 80% of this month's clicks on the Start plan.");
    cleanup();

    billing = status("blocked", 1_200_000);
    render(<PlanBanner />);
    expect(screen.getByRole("alert").textContent).toContain("new campaigns and stream sets wait");
    expect(screen.getByRole("link", { name: "Plan and usage" }).getAttribute("href")).toBe("/settings?tab=billing");
  });
});

describe("BillingPanel", () => {
  it("shows the clicks, the counts and the plans", () => {
    billing = status("warning", 850_000);
    render(<BillingPanel />);
    expect(screen.getByText("850,000 / 1,000,000")).toBeTruthy();
    expect(screen.getByRole("progressbar")).toBeTruthy();
    expect(screen.getByText("50 / 50")).toBeTruthy();
    expect(screen.getByText("your plan")).toBeTruthy();
    expect(screen.getByText("$399/mo")).toBeTruthy();
    expect(screen.getAllByText("unlimited").length).toBe(3);
  });

  it("shows a dash, not zero, when the clicks are unknown", () => {
    billing = status("unknown", null);
    render(<BillingPanel />);
    expect(screen.getByText("— / 1,000,000")).toBeTruthy();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });
});
