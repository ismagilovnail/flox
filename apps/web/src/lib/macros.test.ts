import { describe, expect, it } from "vitest";

import { POSTLANDING_MACROS } from "@/lib/macros";

describe("POSTLANDING_MACROS", () => {
  // Mirrors what apps/tracker/postlanding.go resolves a postlanding URL with.
  it("offers the click's values and {next_url}, nothing a click doesn't have", () => {
    const tokens = POSTLANDING_MACROS.map((m) => m.token);
    expect(tokens[0]).toBe("{next_url}");
    for (const t of ["{click_id}", "{campaign_id}", "{offer_id}", "{country}", "{device}", "{sub1}", "{sub10}"]) {
      expect(tokens).toContain(t);
    }
    for (const t of ["{status}", "{revenue}", "{currency}", "{payout}", "{source}"]) {
      expect(tokens).not.toContain(t);
    }
  });
});
