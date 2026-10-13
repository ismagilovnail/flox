import type { TFunction } from "i18next";
import { describe, expect, it } from "vitest";

import { buildStreamSetFormSchema } from "@/features/stream-sets/stream-set-schema";
import { dehydrateFilterNode, hydrateRootFilter, type ApiFilterNode } from "@/lib/api/stream-sets";

const t = ((key: string) => key) as unknown as TFunction;

// What GET /campaigns/{id}/stream-sets returns: Go's `omitempty` leaves
// valueTo out of every condition that isn't BETWEEN, and value out of
// EXISTS / NOT_EXISTS.
const wireRoot: ApiFilterNode = {
  type: "group",
  joiner: "AND",
  children: [
    { type: "condition", field: "bot", operator: "IS", value: "0" },
    { type: "condition", field: "os_version", operator: "BETWEEN", value: "10", valueTo: "14" },
    { type: "condition", field: "sub1", operator: "EXISTS" },
  ],
};

describe("hydrateRootFilter", () => {
  it("gives a condition missing value/valueTo on the wire empty ones", () => {
    const root = hydrateRootFilter(wireRoot);
    const [plain, range, exists] = root.children;
    expect(plain).toMatchObject({ type: "condition", value: "0", valueTo: "" });
    expect(range).toMatchObject({ type: "condition", value: "10", valueTo: "14" });
    expect(exists).toMatchObject({ type: "condition", value: "", valueTo: "" });
  });

  it("produces a filter the stream set form accepts, so a saved set can be saved again", () => {
    const rootFilter = buildStreamSetFormSchema(t).shape.rootFilter;
    expect(rootFilter.safeParse(hydrateRootFilter(wireRoot)).success).toBe(true);
  });

  it("round-trips back to the wire shape", () => {
    const back = dehydrateFilterNode(hydrateRootFilter(wireRoot));
    expect(back).toEqual({
      type: "group",
      joiner: "AND",
      children: [
        { type: "condition", field: "bot", operator: "IS", value: "0", valueTo: "" },
        { type: "condition", field: "os_version", operator: "BETWEEN", value: "10", valueTo: "14" },
        { type: "condition", field: "sub1", operator: "EXISTS", value: "", valueTo: "" },
      ],
    });
  });
});
