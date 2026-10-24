import { describe, expect, it } from "vitest";

import i18next from "@/lib/i18n/config";
import { flowNodeSummaries } from "@/features/stream-sets/flow-summary";

describe("flowNodeSummaries", () => {
  const t = i18next.getFixedT("en", "streamSets");

  it("formats each node from its own stages", () => {
    const s = flowNodeSummaries(
      { SOURCE_CLICK: 1200, LAND_VIEW: 1000, LAND_CLICK: 250, PWA_VIEW: 200, PWA_INSTALL: 50, CPA_ACCEPT: 7, CPA_REDEP: 3 },
      t,
      "en",
    );
    expect(s.landing).toBe("1,000 views · 250 clicks · CTR 25%");
    expect(s.pwa).toBe("200 views · 50 installs · CR 25%");
    expect(s.destination).toBe("1,200 clicks · 7 conversions · 3 repeat purchases");
  });

  it("shows a rate over zero views as —, never 0%", () => {
    const s = flowNodeSummaries({}, t, "en");
    expect(s.postlanding).toBe("0 views · 0 clicks · CTR —");
  });
});
