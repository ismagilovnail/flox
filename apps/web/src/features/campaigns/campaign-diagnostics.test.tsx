import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import type { CampaignDiagnostics as Diagnostics } from "@/lib/api/campaigns";

// Clicks over the tracker's per-IP limit (audit REL-2).
let data: Diagnostics | undefined;
vi.mock("@/hooks/use-campaigns", () => ({ useCampaignDiagnostics: () => ({ data }) }));

const { CampaignDiagnostics } = await import("@/features/campaigns/campaign-diagnostics");

afterEach(() => {
  data = undefined;
});

describe("CampaignDiagnostics", () => {
  it("says how many clicks were not recorded and when", () => {
    data = { ipLimited: { windowDays: 7, clicks: 42, lastDay: "2026-10-23T00:00:00Z" } };
    render(<CampaignDiagnostics campaignId="c1" />);
    expect(screen.getByText("42 clicks in 7 days were not recorded")).toBeTruthy();
    expect(screen.getByText(/Last on October 23 \(UTC\)/)).toBeTruthy();
    expect(screen.getByText(/TRUSTED_PROXIES/)).toBeTruthy();
  });

  it("renders nothing when no click was turned away or nothing is loaded", () => {
    data = { ipLimited: { windowDays: 7, clicks: 0, lastDay: null } };
    const { container } = render(<CampaignDiagnostics campaignId="c1" />);
    expect(container.textContent).toBe("");
    data = undefined;
    const second = render(<CampaignDiagnostics campaignId="c1" />);
    expect(second.container.textContent).toBe("");
  });
});
