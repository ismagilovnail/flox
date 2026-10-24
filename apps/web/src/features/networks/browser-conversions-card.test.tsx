import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import type { Network } from "@/lib/api/networks";

// Audit UNI-1 part 2: a site without a server reports with flox.js.
const mutate = vi.fn();
vi.mock("@/hooks/use-networks", () => ({ useUpdateNetwork: () => ({ mutate, isPending: false }) }));

const { BrowserConversionsCard, floxSnippets, trackerOrigin } = await import("@/features/networks/browser-conversions-card");

const network = {
  id: "01NET",
  name: "Shop",
  templateKey: "own_site",
  incomingPostbackUrl: "https://t.example/postback/01NET",
  browserConversions: false,
  browserDomains: [],
} as unknown as Network;

afterEach(() => mutate.mockReset());

describe("BrowserConversionsCard", () => {
  it("gives the tags with the tracker's origin and the source's id", () => {
    expect(trackerOrigin(network.incomingPostbackUrl)).toBe("https://t.example");
    const s = floxSnippets("https://t.example", "01NET");
    expect(s.everyPage).toBe('<script src="https://t.example/flox.js" data-source="01NET" async></script>');
    expect(s.thankYou).toContain('data-event="purchase"');
    render(<BrowserConversionsCard network={network} canWrite />);
    expect(screen.getByText(s.everyPage)).toBeTruthy();
    expect(screen.getByText(/once per click/)).toBeTruthy();
  });

  it("asks for a domain before turning it on, then saves the list", () => {
    render(<BrowserConversionsCard network={network} canWrite />);
    fireEvent.click(screen.getByRole("switch"));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(screen.getByText("Add your site's domain first.")).toBeTruthy();
    expect(mutate).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText("Your site's domains"), { target: { value: "shop.example.com,\npromo.shop.example.com " } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(mutate).toHaveBeenCalledWith({ browserConversions: true, browserDomains: ["shop.example.com", "promo.shop.example.com"] }, expect.anything());
  });

  it("is read-only without the permission", () => {
    render(<BrowserConversionsCard network={network} canWrite={false} />);
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
    expect(screen.getByRole("switch").matches(":disabled")).toBe(true);
  });
});
