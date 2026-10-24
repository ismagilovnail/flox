import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import type { EventMapping, WaitingStatus } from "@/lib/api/event-mappings";

// Postbacks waiting for their status mapping (audit 2026-10-15, DATA-4):
// the dashboard says so, and the mapping card offers to map each status.
let waiting: WaitingStatus[] = [];
let mappings: EventMapping[] = [];
vi.mock("@/hooks/use-event-mappings", () => ({
  useWaitingStatuses: () => ({ data: { statuses: waiting }, isError: false }),
  useEventMappings: () => ({ data: { eventMappings: mappings, total: mappings.length }, isPending: false, isError: false }),
  useCreateEventMapping: () => ({ mutate: vi.fn(), isPending: false }),
  useDeleteEventMapping: () => ({ mutate: vi.fn(), isPending: false }),
}));
vi.mock("@/hooks/use-networks", () => ({
  useNetworks: () => ({ data: { networks: [{ id: "net-1", name: "Net One" }] }, isPending: false, isError: false }),
}));
vi.mock("@/hooks/use-auth", () => ({ usePermission: () => true }));

const { UnmappedPostbacksNotice } = await import("@/features/dashboard/unmapped-postbacks-notice");
const { EventMappingPanel } = await import("@/features/postbacks/event-mapping-panel");

const lastAt = new Date().toISOString();

afterEach(() => {
  waiting = [];
  mappings = [];
});

describe("UnmappedPostbacksNotice", () => {
  it("counts the waiting postbacks, names their statuses and links to the mapping", () => {
    waiting = [
      { networkId: "net-1", rawStatus: "approved", count: 3, lastAt },
      { networkId: "net-1", rawStatus: "rejected", count: 1, lastAt },
    ];
    render(<UnmappedPostbacksNotice />);
    expect(screen.getByText(/4 postbacks/)).toBeTruthy();
    expect(screen.getByText(/approved, rejected/)).toBeTruthy();
    expect(screen.getByRole("link").getAttribute("href")).toBe("/postbacks?tab=mapping");
  });

  it("renders nothing when nothing waits", () => {
    const { container } = render(<UnmappedPostbacksNotice />);
    expect(container.innerHTML).toBe("");
  });
});

describe("EventMappingPanel waiting statuses", () => {
  it("lists a waiting status and Map puts it in the form", () => {
    waiting = [{ networkId: "net-1", rawStatus: "approved", count: 2, lastAt }];
    render(<EventMappingPanel />);
    expect(screen.getByText("2 postbacks")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Map status approved/ }));
    expect((screen.getByPlaceholderText("e.g. purchase") as HTMLInputElement).value).toBe("approved");
  });

  it("hides a status that has been mapped since", () => {
    waiting = [{ networkId: "net-1", rawStatus: "Approved", count: 2, lastAt }];
    mappings = [
      { id: "m1", organizationId: "o", networkId: "net-1", networkStatus: "approved", floxStatus: "CPA_ACCEPT", createdAt: lastAt, updatedAt: lastAt },
    ];
    render(<EventMappingPanel />);
    expect(screen.queryByText("2 postbacks")).toBeNull();
  });
});
