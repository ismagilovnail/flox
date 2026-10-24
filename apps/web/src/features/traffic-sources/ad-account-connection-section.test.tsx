import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

import "@/lib/i18n/config";

const connect = vi.fn();
vi.mock("@/hooks/use-auth", () => ({ usePermission: () => true }));
vi.mock("@/hooks/use-ad-account-connection", () => ({
  useAdAccountConnection: () => ({ isLoading: false, data: null }),
  useConnectAdAccount: () => ({ mutate: connect, isPending: false }),
  useDisconnectAdAccount: () => ({ mutate: vi.fn(), isPending: false }),
  useSyncAdAccount: () => ({ mutate: vi.fn(), isPending: false }),
}));

const { AdAccountConnectionSection } = await import("@/features/traffic-sources/ad-account-connection-section");

// Audit UNI-3 part 1: a Yandex Direct source connects with the
// advertiser's Direct login and an OAuth token.
describe("AdAccountConnectionSection for Yandex Direct", () => {
  it("asks for the Direct login and a token, and sends them", async () => {
    render(<AdAccountConnectionSection trafficSourceId="ts" provider="yandex_direct" />);
    expect(screen.getByText(/Client-Login/)).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Direct login (the advertiser's)"), { target: { value: "my-shop" } });
    fireEvent.change(screen.getByLabelText("Access token"), { target: { value: "y0_token" } });
    fireEvent.click(screen.getByRole("button", { name: /Connect/ }));
    await vi.waitFor(() => expect(connect).toHaveBeenCalledTimes(1));
    expect(connect.mock.calls[0][0]).toMatchObject({ adAccountId: "my-shop", accessToken: "y0_token" });
  });
});
