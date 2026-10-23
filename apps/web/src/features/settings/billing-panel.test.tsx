import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import type { BillingStatus, Payment, Plan } from "@/lib/api/billing";

// Plan and usage (BIZ-1): the banner shows from 80% of the month's clicks
// and when the paid period runs out; the Plan tab shows the clicks, the
// counts against their limits, every plan, and — only to an owner, only
// when the server can take payments — paying for one.
const start: Plan = {
  code: "start", name: "Start", monthlyClicks: 1_000_000, maxCampaigns: 50, maxDomains: 5, maxMembers: 5,
  priceCents: 4900, currency: "USD", priceRub: 490_000, rawRetentionMonths: 6,
};
const business: Plan = {
  code: "business", name: "Business", monthlyClicks: 20_000_000, maxCampaigns: null, maxDomains: null, maxMembers: null,
  priceCents: 39900, currency: "USD", priceRub: 3_990_000, rawRetentionMonths: 24,
};
const free: Plan = {
  code: "free", name: "Free", monthlyClicks: 100_000, maxCampaigns: 5, maxDomains: 1, maxMembers: 2, priceCents: 0, currency: "USD", priceRub: null,
  rawRetentionMonths: 3,
};
function status(level: BillingStatus["level"], clicks: number | null, extra: Partial<BillingStatus> = {}): BillingStatus {
  return {
    plan: start, since: "2026-10-01T00:00:00Z", period: { from: "2026-10-01", to: "2026-10-31" },
    usage: { clicks, campaigns: 50, domains: 2, members: 3 }, level,
    paidUntil: null, graceUntil: null, renew: false, canPay: false,
    rawRetentionMonths: 6, rawRetentionShortens: null, ...extra,
  };
}
let billing: BillingStatus | undefined;
let role = "Owner";
let returned: Payment | undefined;
let search = "";
const checkout = vi.fn();
vi.mock("@/hooks/use-billing", () => ({
  billingKey: ["billing"],
  useBilling: () => ({ data: billing, isPending: !billing, isError: false }),
  usePlans: () => ({ data: { plans: [free, start, business] }, isPending: false, isError: false }),
  usePayments: () => ({ data: { payments: [] }, isPending: false, isError: false }),
  usePayment: () => ({ data: returned, isError: false }),
  useCheckout: () => ({ mutate: checkout, isPending: false }),
}));
vi.mock("@/hooks/use-auth", () => ({
  useMe: () => ({ data: { role } }),
  usePermission: () => role === "Owner" || role === "Admin",
}));
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: vi.fn() }) }));
vi.mock("next/navigation", () => ({
  usePathname: () => "/settings",
  useRouter: () => ({ replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams(search),
}));

// Radix's radio group measures itself; jsdom has no ResizeObserver.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
};

const { PlanBanner } = await import("@/components/shell/plan-banner");
const { BillingPanel } = await import("@/features/settings/billing-panel");

afterEach(() => {
  cleanup();
  billing = undefined;
  role = "Owner";
  returned = undefined;
  search = "";
  checkout.mockReset();
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

  it("asks to renew when the paid period runs out", () => {
    billing = status("ok", 10, { paidUntil: "2026-10-24T12:00:00Z", graceUntil: "2026-10-27T12:00:00Z", renew: true });
    render(<PlanBanner />);
    expect(screen.getByRole("alert").textContent).toContain("The Start plan is paid until");
    expect(screen.getByRole("link", { name: "Renew" })).toBeTruthy();
  });
});

describe("BillingPanel", () => {
  it("shows the clicks, the counts and the plans in roubles", () => {
    billing = status("warning", 850_000);
    render(<BillingPanel />);
    expect(screen.getByText("850,000 / 1,000,000")).toBeTruthy();
    expect(screen.getByRole("progressbar")).toBeTruthy();
    expect(screen.getByText("50 / 50")).toBeTruthy();
    expect(screen.getByText("your plan")).toBeTruthy();
    expect(screen.getByText("RUB 39,900/mo")).toBeTruthy();
    expect(screen.getAllByText("Free").length).toBe(2); // the plan and its price
    expect(screen.getAllByText("unlimited").length).toBe(3);
  });

  it("shows a dash, not zero, when the clicks are unknown", () => {
    billing = status("unknown", null);
    render(<BillingPanel />);
    expect(screen.getByText("— / 1,000,000")).toBeTruthy();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("lets an owner pay when the server takes payments", () => {
    billing = status("ok", 10, { canPay: true });
    render(<BillingPanel />);
    expect(screen.getAllByRole("button", { name: "Choose" }).length).toBe(1); // Business; Free is not sold
    fireEvent.click(screen.getByRole("button", { name: "Renew" }));
    expect(screen.getByText("Pay for Start")).toBeTruthy();
    fireEvent.click(screen.getByRole("radio", { name: /^12 months/ }));
    fireEvent.click(screen.getByRole("button", { name: /^Pay RUB\s58,800$/ }));
    expect(checkout).toHaveBeenCalledWith({ plan: "start", months: 12 }, expect.anything());
    cleanup();

    // Another plan: unused paid days are mentioned only when there are some.
    render(<BillingPanel />);
    fireEvent.click(screen.getByRole("button", { name: "Choose" }));
    expect(screen.getByText("The new plan starts as soon as the payment goes through.")).toBeTruthy();
    cleanup();
    billing = status("ok", 10, { canPay: true, paidUntil: "2026-11-22T12:00:00Z", graceUntil: "2026-11-25T12:00:00Z" });
    render(<BillingPanel />);
    fireEvent.click(screen.getByRole("button", { name: "Choose" }));
    expect(screen.getByText(/Unused paid days of Start are carried over/)).toBeTruthy();
  });

  // DATA-6: how long raw clicks are kept — per plan, now, and a shorter
  // period a lapsed or downgraded plan brings, with its date.
  it("shows how long raw clicks are kept, and when that shortens", () => {
    billing = status("ok", 10);
    render(<BillingPanel />);
    expect(screen.getByText(/Raw clicks are kept 6 mo\./)).toBeTruthy();
    expect(screen.getByText("Click history")).toBeTruthy();
    expect(screen.getByText("24 mo.")).toBeTruthy();
    expect(screen.queryByText(/older clicks will be deleted/)).toBeNull();
    cleanup();

    billing = status("ok", 10, { plan: free, rawRetentionMonths: 24, rawRetentionShortens: { months: 3, at: "2026-11-22T12:00:00Z" } });
    render(<BillingPanel />);
    expect(screen.getByText(/Raw clicks are kept 24 mo\./)).toBeTruthy();
    expect(screen.getByText(/raw clicks are kept 3 mo\. — older clicks will be deleted/)).toBeTruthy();
  });

  it("offers no payment to an admin, nor without payments set up", () => {
    role = "Admin";
    billing = status("ok", 10, { canPay: true });
    render(<BillingPanel />);
    expect(screen.queryByRole("button", { name: "Choose" })).toBeNull();
    expect(screen.getByText(/An owner of the organization can pay/)).toBeTruthy();
    cleanup();

    role = "Owner";
    billing = status("ok", 10, { canPay: false });
    render(<BillingPanel />);
    expect(screen.queryByRole("button", { name: "Choose" })).toBeNull();
  });

  it("shows the outcome of the payment the owner came back from", () => {
    billing = status("ok", 10, { canPay: true, paidUntil: "2026-11-22T12:00:00Z", graceUntil: "2026-11-25T12:00:00Z" });
    search = "tab=billing&payment=P1";
    returned = {
      id: "P1", planCode: "start", months: 1, amount: 490_000, currency: "RUB", status: "pending", cancellationReason: "",
      confirmationUrl: "https://yoomoney.example/checkout/P1", appliedAt: null, createdAt: "2026-10-22T12:00:00Z",
    };
    render(<BillingPanel />);
    expect(screen.getByText("Waiting for YooKassa to confirm the payment…")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Continue payment" }).getAttribute("href")).toBe("https://yoomoney.example/checkout/P1");
    expect(screen.getByText(/Paid until/)).toBeTruthy();
    cleanup();

    returned = { ...returned, status: "succeeded", confirmationUrl: undefined };
    render(<BillingPanel />);
    expect(screen.getByText("Payment received — your plan is updated.")).toBeTruthy();
  });
});
