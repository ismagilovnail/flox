import { describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import type { OfflineConversions } from "@/lib/api/ad-account-connections";
import { ApiError } from "@/lib/api/client";

const status: OfflineConversions = {
  enabled: false,
  since: null,
  loginAccountId: "",
  leadActionId: "",
  purchaseActionId: "",
  repurchaseActionId: "",
  counts: { sent: 3, pending: 1, failed: 2, skipped: 5 },
  skipped: { no_google_click: 4, no_action: 1 },
  lastError: "PERMISSION_DENIED: insufficient scopes",
  lastErrorAt: "2026-10-24T10:00:00Z",
  lastSentAt: null,
};

const mutate = vi.fn();
vi.mock("@/hooks/use-auth", () => ({ usePermission: () => true }));
vi.mock("@/hooks/use-ad-account-connection", () => ({
  useOfflineConversions: () => ({ isLoading: false, data: status }),
  useUpdateOfflineConversions: () => ({ mutate, isPending: false }),
}));

const { OfflineConversionsSection } = await import("@/features/traffic-sources/offline-conversions-section");

// Audit UNI-2 part 2: the source's export settings, what came of it, and
// Google's last refusal.
describe("OfflineConversionsSection", () => {
  it("shows what was sent, skipped and refused", () => {
    render(<OfflineConversionsSection trafficSourceId="ts" />);
    expect(screen.getByText("3 sent")).toBeTruthy();
    expect(screen.getByText("2 refused")).toBeTruthy();
    expect(screen.getByText(/4 conversions — their clicks had no gclid/)).toBeTruthy();
    expect(screen.getByText(/1 conversion — no action named/)).toBeTruthy();
    expect(screen.getByText(/insufficient scopes/)).toBeTruthy();
  });

  it("saves the edited settings, and shows the server's field errors", () => {
    render(<OfflineConversionsSection trafficSourceId="ts" />);
    const save = screen.getByRole("button", { name: /Save/ }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);

    fireEvent.click(screen.getByRole("switch", { name: "Send conversions to Google Ads" }));
    fireEvent.change(screen.getByLabelText("Action for purchases"), { target: { value: "987" } });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    expect(mutate).toHaveBeenCalledTimes(1);
    expect(mutate.mock.calls[0][0]).toEqual({
      enabled: true,
      loginAccountId: "",
      leadActionId: "",
      purchaseActionId: "987",
      repurchaseActionId: "",
    });

    const onError = mutate.mock.calls[0][1].onError as (err: Error) => void;
    act(() => onError(new ApiError(400, "validation", "invalid", { purchaseActionId: "digits only" })));
    expect(screen.getByText("digits only")).toBeTruthy();
  });
});
