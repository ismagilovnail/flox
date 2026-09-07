import { describe, expect, it } from "vitest";

import i18n from "@/lib/i18n/config";
import { offerColumns } from "@/features/offers/offer-columns";
import type { Offer } from "@/lib/api/offers";

const baseOffer: Offer = {
  id: "offer-1",
  organizationId: "org-1",
  networkId: "network-1",
  name: "Test Offer",
  countries: ["US"],
  payout: 12.5,
  currency: "USD",
  cap: null,
  status: "active",
  links: [],
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

function networkCell(networkNameById: Record<string, string>, offer: Offer) {
  const columns = offerColumns(i18n.t, networkNameById);
  const networkColumn = columns.find((c) => c.id === "network");
  if (!networkColumn || !("cell" in networkColumn) || typeof networkColumn.cell !== "function") {
    throw new Error("network column has no cell renderer");
  }
  // The cell renderer only reads ctx.row.original, so a minimal stand-in
  // avoids fighting the generated table-features CellContext<> generics for
  // a real (mounted) table this test deliberately doesn't build.
  const ctx = { row: { original: offer } };
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  return (networkColumn.cell as any)(ctx);
}

describe("offerColumns' network column (regression: accessorFn stale-cache bug)", () => {
  // The bug: accessorFn: (row) => networkNameById[row.networkId] ?? row.networkId
  // closed over networkNameById at the time TanStack Table first cached this
  // row's value, and never re-read it once useNetworks() resolved after
  // useOffers() — the raw networkId fallback would freeze in place. A `cell`
  // renderer re-runs on every render, so it must always reflect the
  // networkNameById passed into offerColumns *this* call, not a value frozen
  // from an earlier one.

  it("renders the resolved network name once it's available", () => {
    expect(networkCell({ "network-1": "Ad Network One" }, baseOffer)).toBe("Ad Network One");
  });

  it("falls back to the raw networkId when the name isn't loaded yet", () => {
    expect(networkCell({}, baseOffer)).toBe("network-1");
  });

  it("reflects a newer networkNameById on a later call — not frozen on the first one seen (the actual regression)", () => {
    const beforeLoad = networkCell({}, baseOffer);
    expect(beforeLoad).toBe("network-1");

    // Simulates useNetworks() resolving after useOffers() already rendered
    // once — offerColumns is called again (as it is on every OfferList
    // render) with the now-populated map.
    const afterLoad = networkCell({ "network-1": "Ad Network One" }, baseOffer);
    expect(afterLoad).toBe("Ad Network One");
  });

  it("does not use accessorFn for this column (would reintroduce the cache bug)", () => {
    const columns = offerColumns(i18n.t, { "network-1": "Ad Network One" });
    const networkColumn = columns.find((c) => c.id === "network");
    expect(networkColumn).toBeDefined();
    expect("accessorFn" in (networkColumn ?? {})).toBe(false);
  });
});
