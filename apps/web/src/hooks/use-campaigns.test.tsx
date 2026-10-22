import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";

// Audit TEAM-1: who may change a campaign — campaign.write and either
// campaign.manage_all or owning it. UX only; the API checks the same.
let me: { user: { id: string }; permissions: string[] } | undefined;
vi.mock("@/hooks/use-auth", () => ({ useMe: () => ({ data: me }) }));

const { useCanEditCampaign } = await import("@/hooks/use-campaigns");

afterEach(() => {
  me = undefined;
});

const can = (campaign: { ownerUserId: string | null } | undefined) => renderHook(() => useCanEditCampaign(campaign)).result.current;

describe("useCanEditCampaign", () => {
  it("lets a buyer change only campaigns they own", () => {
    me = { user: { id: "ann" }, permissions: ["campaign.read", "campaign.write"] };
    expect(can({ ownerUserId: "ann" })).toBe(true);
    expect(can({ ownerUserId: "bob" })).toBe(false);
    expect(can({ ownerUserId: null })).toBe(false);
    expect(can(undefined)).toBe(false);
  });

  it("lets campaign.manage_all change every campaign", () => {
    me = { user: { id: "lead" }, permissions: ["campaign.write", "campaign.manage_all"] };
    expect(can({ ownerUserId: "bob" })).toBe(true);
    expect(can({ ownerUserId: null })).toBe(true);
  });

  it("never without campaign.write", () => {
    me = { user: { id: "ann" }, permissions: ["campaign.read"] };
    expect(can({ ownerUserId: "ann" })).toBe(false);
    me = undefined;
    expect(can({ ownerUserId: "ann" })).toBe(false);
  });
});
