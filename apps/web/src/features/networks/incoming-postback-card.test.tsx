import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import type { Network } from "@/lib/api/networks";

// Audit UNI-1: a site reporting its own conversions gets its own words and
// what each plain status means; every source shows statuses by name.
vi.mock("@/hooks/use-event-mappings", () => ({
  useEventMappings: () => ({
    isPending: false,
    data: {
      eventMappings: [
        { id: "m1", networkId: "own", networkStatus: "purchase", floxStatus: "CPA_ACCEPT" },
        { id: "m2", networkId: "own", networkStatus: "refund", floxStatus: "CPA_DECLINE" },
        { id: "m3", networkId: "net", networkStatus: "999", floxStatus: "CPA_ACCEPT" },
      ],
    },
  }),
}));
vi.mock("@/hooks/use-networks", () => ({ useRegeneratePostbackSecret: () => ({ mutate: vi.fn(), isPending: false }) }));

const { IncomingPostbackCard } = await import("@/features/networks/incoming-postback-card");

const base = { incomingPostbackUrl: "https://t.example/postback/x", currency: "USD" };

describe("IncomingPostbackCard", () => {
  it("tells an own site's developer what to do, with each status explained", () => {
    render(<IncomingPostbackCard network={{ ...base, id: "own", name: "Shop", templateKey: "own_site" } as Network} canWrite={false} />);
    expect(screen.getByText("Conversion URL — for your site")).toBeTruthy();
    expect(screen.getByText(/status=purchase&revenue=\{amount\}/)).toBeTruthy();
    expect(screen.getByText(/Call it from your server only/)).toBeTruthy();
    expect(screen.getByText("a paid order — the conversion")).toBeTruthy();
    expect(screen.getByText(/takes back that order/)).toBeTruthy();
    expect(screen.queryByText(/partner/i)).toBeNull();
  });

  it("names a partner network's statuses instead of showing the enum", () => {
    render(<IncomingPostbackCard network={{ ...base, id: "net", name: "Net", templateKey: "" } as Network} canWrite={false} />);
    expect(screen.getByText("Incoming postback — for the network")).toBeTruthy();
    const item = screen.getByText("999").closest("li")!;
    expect(item.textContent).toBe("999 → ACCEPT");
    expect(item.getAttribute("title")).toBe("CPA_ACCEPT");
  });
});
