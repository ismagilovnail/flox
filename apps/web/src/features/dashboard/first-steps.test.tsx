import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import type { OnboardingChecklist } from "@/lib/api/onboarding";

let checklist: OnboardingChecklist | undefined;
vi.mock("@/hooks/use-onboarding", () => ({ useOnboarding: () => ({ data: checklist }) }));
vi.mock("@/hooks/use-organization", () => ({ useOrganization: () => ({ data: { id: "org-1" } }) }));

const { FirstSteps } = await import("@/features/dashboard/first-steps");

const ids = ["source", "network", "mapping", "offer", "campaign", "click", "conversion", "cost"] as const;
const make = (done: number, unknownClick = false): OnboardingChecklist => ({
  steps: ids.map((id, i) => ({ id, done: i < done, unknown: id === "click" && unknownClick })),
  done,
  total: ids.length,
});

beforeEach(() => localStorage.clear());
afterEach(() => {
  checklist = undefined;
});

describe("FirstSteps", () => {
  it("shows progress and points at the next step", () => {
    checklist = make(2);
    render(<FirstSteps />);
    expect(screen.getByText(/2.*8/)).toBeTruthy();
    // Done steps carry no action; every open step links to where it is done.
    const links = screen.getAllByRole("link");
    expect(links).toHaveLength(6);
    expect(links[0].getAttribute("href")).toBe("/postbacks"); // mapping, the next step
  });

  it("renders nothing while loading or once everything is done", () => {
    const { container, rerender } = render(<FirstSteps />);
    expect(container.innerHTML).toBe("");
    checklist = make(8);
    rerender(<FirstSteps />);
    expect(container.innerHTML).toBe("");
  });

  it("stays hidden after it is dismissed, per organization", () => {
    checklist = make(1);
    const { container, unmount } = render(<FirstSteps />);
    fireEvent.click(screen.getByRole("button"));
    expect(container.innerHTML).toBe("");
    unmount();
    expect(localStorage.getItem("flox-first-steps-dismissed:org-1")).toBe("1");
    const again = render(<FirstSteps />);
    expect(again.container.innerHTML).toBe("");
  });
});
