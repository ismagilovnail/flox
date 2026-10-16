import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import type { OrganizationDeletion } from "@/lib/api/organization";

// Organization deletion (audit 2026-10-15, GDPR-1): only an owner gets the
// buttons; the dialog submits once the exact name and a password are typed;
// while scheduled, every page shows the date.
let role = "Owner";
let deletion: OrganizationDeletion = { scheduledAt: null, requestedAt: null, requestedBy: "" };
const schedule = vi.fn();
const cancel = vi.fn();
vi.mock("@/hooks/use-auth", () => ({ useMe: () => ({ data: { role } }) }));
vi.mock("@/hooks/use-organization", () => ({
  useOrganization: () => ({ data: { id: "o1", name: "Acme Media" } }),
  useOrganizationDeletion: () => ({ data: deletion }),
  useScheduleOrganizationDeletion: () => ({ mutate: schedule, isPending: false }),
  useCancelOrganizationDeletion: () => ({ mutate: cancel, isPending: false }),
}));

const { OrganizationDeletionCard } = await import("@/features/settings/organization-deletion-card");
const { DeletionBanner } = await import("@/components/shell/deletion-banner");

afterEach(() => {
  role = "Owner";
  deletion = { scheduledAt: null, requestedAt: null, requestedBy: "" };
  schedule.mockReset();
  cancel.mockReset();
});

describe("OrganizationDeletionCard", () => {
  it("lets an owner schedule only with the exact name and a password", () => {
    render(<OrganizationDeletionCard />);
    fireEvent.click(screen.getByRole("button", { name: "Delete organization…" }));
    const submit = screen.getByRole("button", { name: "Delete in 7 days" }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Type Acme Media to confirm"), { target: { value: "Acme" } });
    fireEvent.change(screen.getByLabelText("Your password"), { target: { value: "secret" } });
    expect(submit.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Type Acme Media to confirm"), { target: { value: "Acme Media" } });
    expect(submit.disabled).toBe(false);
  });

  it("shows other members no buttons", () => {
    role = "Admin";
    render(<OrganizationDeletionCard />);
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.getByText("Only an owner can delete the organization.")).toBeTruthy();
  });

  it("shows the date and a cancel button once scheduled", () => {
    deletion = { scheduledAt: "2026-10-23T12:00:00Z", requestedAt: "2026-10-16T12:00:00Z", requestedBy: "Dana" };
    render(<OrganizationDeletionCard />);
    expect(screen.getByText(/requested by Dana/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Cancel deletion" }));
    expect(cancel).toHaveBeenCalled();
  });
});

describe("DeletionBanner", () => {
  it("is shown only while a deletion is scheduled", () => {
    const { container, rerender } = render(<DeletionBanner />);
    expect(container.innerHTML).toBe("");
    deletion = { scheduledAt: "2026-10-23T12:00:00Z", requestedAt: null, requestedBy: "Dana" };
    rerender(<DeletionBanner />);
    expect(screen.getByRole("alert").textContent).toMatch(/will be deleted on/);
  });
});
