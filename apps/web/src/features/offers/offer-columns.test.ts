import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";

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

describe("offerColumns' cap column", () => {
  function capCell(offer: Offer, caps: Parameters<typeof offerColumns>[2]) {
    const column = offerColumns(i18n.t, {}, caps).find((c) => "accessorKey" in c && c.accessorKey === "cap");
    if (!column || !("cell" in column) || typeof column.cell !== "function") throw new Error("cap column has no cell renderer");
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    return render((column.cell as any)({ row: { original: offer }, getValue: () => offer.cap }));
  }

  it("shows today's count and a badge once the cap is reached", () => {
    const capped = { ...baseOffer, cap: 100 };
    capCell(capped, { "offer-1": { offerId: "offer-1", cap: 100, count: 100, reached: true } });
    expect(screen.getByText("100 / 100")).toBeInTheDocument();
    expect(screen.getByText("Cap reached")).toBeInTheDocument();
  });

  it("shows the count without a badge below the cap, and just the cap when the count is unknown", () => {
    const capped = { ...baseOffer, cap: 100 };
    const { unmount } = capCell(capped, { "offer-1": { offerId: "offer-1", cap: 100, count: 37, reached: false } });
    expect(screen.getByText("37 / 100")).toBeInTheDocument();
    expect(screen.queryByText("Cap reached")).toBeNull();
    unmount();
    capCell(capped, {});
    expect(screen.getByText("100")).toBeInTheDocument();
  });
});
