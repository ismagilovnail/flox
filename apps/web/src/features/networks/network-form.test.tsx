import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import { NetworkForm } from "@/features/networks/network-form";

// The Radix selects measure themselves; jsdom has no ResizeObserver.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

// Audit UNI-1: creating a source asks where conversions come from; an own
// site has no partner platform to pick.
describe("NetworkForm kind", () => {
  const props = { title: "New", submitLabel: "Create", onSubmit: vi.fn() };

  it("an own site: its hint, its words, no platform", () => {
    render(<NetworkForm {...props} chooseKind defaultValues={{ templateKey: "own_site" }} />);
    expect(screen.getByLabelText("Where conversions come from")).toBeTruthy();
    expect(screen.getByText(/reports its leads, purchases and refunds itself/)).toBeTruthy();
    expect(screen.getByLabelText("Currency of amounts")).toBeTruthy();
    expect(screen.queryByLabelText("Affiliate platform")).toBeNull();
  });

  it("a partner network: the platform to pick", () => {
    render(<NetworkForm {...props} chooseKind defaultValues={{}} />);
    expect(screen.getByText(/whose offers you send traffic to/)).toBeTruthy();
    expect(screen.getByLabelText("Affiliate platform")).toBeTruthy();
  });

  it("editing does not ask again", () => {
    render(<NetworkForm {...props} defaultValues={{ templateKey: "own_site" }} />);
    expect(screen.queryByLabelText("Where conversions come from")).toBeNull();
    expect(screen.queryByLabelText("Affiliate platform")).toBeNull();
  });
});
