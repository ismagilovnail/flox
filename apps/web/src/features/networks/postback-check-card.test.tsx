import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import type { CheckPostbackResult } from "@/lib/api/postback-logs";
import type { Network } from "@/lib/api/networks";

let data: CheckPostbackResult | undefined;
const mutate = vi.fn();
vi.mock("@/hooks/use-postback-logs", () => ({
  useCheckPostback: () => ({ mutate, data, isPending: false, isError: false }),
}));
vi.mock("@/hooks/use-event-mappings", () => ({
  useEventMappings: () => ({ data: { eventMappings: [{ id: "m1", networkId: "net-1", networkStatus: "ftd", floxStatus: "CPA_ACCEPT" }] } }),
}));

const { PostbackCheckCard, checkStartURL } = await import("@/features/networks/postback-check-card");

const network = { id: "net-1", name: "Net", incomingPostbackUrl: "https://t.example/postback/net-1" } as Network;

afterEach(() => {
  data = undefined;
  mutate.mockReset();
});

describe("PostbackCheckCard", () => {
  it("starts from the network's URL with its first mapped status and sends what is typed", () => {
    render(<PostbackCheckCard network={network} />);
    const box = screen.getByLabelText(/postback url/i) as HTMLTextAreaElement;
    expect(box.value).toBe(checkStartURL(network.incomingPostbackUrl!, "ftd"));
    expect(box.value).toContain("status=ftd");

    fireEvent.change(box, { target: { value: " https://t.example/postback/net-1?secret=s&click_id=c&status=ftd " } });
    fireEvent.click(screen.getByRole("button", { name: /check/i }));
    expect(mutate).toHaveBeenCalledWith("https://t.example/postback/net-1?secret=s&click_id=c&status=ftd");
  });

  it("explains a refused secret as the tracker's 401", () => {
    data = { networkId: "net-1", httpStatus: 401, result: "error", message: "invalid or missing secret", notes: [] };
    render(<PostbackCheckCard network={network} />);
    expect(screen.getByText("HTTP 401")).toBeTruthy();
    expect(screen.getByText(/secret is missing or wrong/i)).toBeTruthy();
  });

  it("shows the money, the click and the outgoing postback of an applied one", () => {
    data = {
      networkId: "net-1",
      httpStatus: 200,
      result: "success",
      status: "CPA_ACCEPT",
      attribution: { outcome: "attributed", campaignId: "camp-1" },
      revenue: 40,
      currency: "EUR",
      currencyAssumed: true,
      usdValue: 44,
      outgoingUrl: "https://partner.example/pb?cid=c",
      notes: ["the URL's host is not the tracker's"],
    };
    render(<PostbackCheckCard network={network} />);
    expect(screen.getByText(/would be recorded and attributed/i)).toBeTruthy();
    expect(screen.getByRole("link", { name: /open the campaign/i }).getAttribute("href")).toBe("/campaigns/camp-1");
    expect(screen.getByText(/\$44\.00/)).toBeTruthy();
    expect(screen.getByText("https://partner.example/pb?cid=c")).toBeTruthy();
    expect(screen.getByText(/host is not the tracker/)).toBeTruthy();
  });

  it("renders nothing without a tracker address", () => {
    const { container } = render(<PostbackCheckCard network={{ ...network, incomingPostbackUrl: "" }} />);
    expect(container.innerHTML).toBe("");
  });
});
