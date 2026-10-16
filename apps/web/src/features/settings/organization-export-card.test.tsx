import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import type { OrganizationExport } from "@/lib/api/organization";

// The organization's data export (audit 2026-10-15, GDPR-1).
let canManage = true;
let exports: OrganizationExport[] = [];
vi.mock("@/hooks/use-auth", () => ({ usePermission: () => canManage }));
vi.mock("@/hooks/use-organization", () => ({
  useOrganizationExports: () => ({ data: { exports } }),
  useRequestOrganizationExport: () => ({ mutate: vi.fn(), isPending: false }),
}));

const { OrganizationExportCard } = await import("@/features/settings/organization-export-card");

const base: OrganizationExport = {
  id: "e1", status: "done", from: "2026-07-01", to: "2026-10-01", sizeBytes: 3 * 1024 * 1024, error: "",
  createdAt: "2026-10-16T10:00:00Z", finishedAt: "2026-10-16T10:01:00Z", expiresAt: "2026-10-23T10:01:00Z", requestedBy: "Dana",
};

afterEach(() => {
  canManage = true;
  exports = [];
});

describe("OrganizationExportCard", () => {
  it("links a ready export to the API download and shows its size", () => {
    exports = [base];
    render(<OrganizationExportCard />);
    expect(screen.getByRole("link", { name: /Download/ }).getAttribute("href")).toMatch(/\/v1\/organization\/exports\/e1\/download$/);
    expect(screen.getByText("3.0 MB")).toBeTruthy();
  });

  it("allows one export at a time", () => {
    exports = [{ ...base, id: "e2", status: "running", expiresAt: null }];
    render(<OrganizationExportCard />);
    expect((screen.getByRole("button", { name: "Prepare export" }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("is hidden from members without settings.write", () => {
    canManage = false;
    const { container } = render(<OrganizationExportCard />);
    expect(container.innerHTML).toBe("");
  });
});
