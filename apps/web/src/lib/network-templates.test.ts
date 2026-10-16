import { describe, expect, it } from "vitest";

import { CPA_STATUSES } from "@/lib/api/conversions";
import { NETWORK_TEMPLATES } from "@/lib/network-templates";
import { incomingPostbackTemplate } from "@/lib/postback-url";

// Audit 2026-10-15, INT-3: affiliate-platform templates.
describe("NETWORK_TEMPLATES", () => {
  it("each has sources, an offer-link parameter and valid mappings", () => {
    for (const t of NETWORK_TEMPLATES) {
      expect(t.sources.length, t.key).toBeGreaterThan(0);
      for (const u of t.sources) expect(u, t.key).toMatch(/^https:\/\//);
      expect(t.offerLinkParam, t.key).toMatch(/^[a-z_0-9]+$/);
      expect(t.incoming.click_id, t.key).toBeTruthy();
      for (const m of t.statusMappings) expect(CPA_STATUSES, t.key).toContain(m.floxStatus);
      if (t.note !== "perGoal") expect(t.statusMappings.length, t.key).toBeGreaterThan(0);
    }
  });

  it("maps Affise's documented status codes", () => {
    const affise = NETWORK_TEMPLATES.find((t) => t.key === "affise")!;
    expect(Object.fromEntries(affise.statusMappings.map((m) => [m.networkStatus, m.floxStatus]))).toEqual({
      "1": "CPA_ACCEPT",
      "2": "CPA_HOLD",
      "3": "CPA_DECLINE",
      "5": "CPA_HOLD",
    });
  });
});

describe("incomingPostbackTemplate", () => {
  const base = "https://t.example/postback/N1";
  it("keeps FLOX's placeholders without a template", () => {
    expect(incomingPostbackTemplate(base)).toBe(`${base}?secret={secret}&click_id={click_id}&status={status}&revenue={revenue}&currency={currency}&txn_id={txn_id}`);
  });

  it("puts the platform's macros in, keeping FLOX's where the platform has none", () => {
    expect(incomingPostbackTemplate(base, "s3cr3t", "affise")).toBe(
      `${base}?secret=s3cr3t&click_id={sub1}&status={status}&revenue={sum}&currency={currency}&txn_id={transactionid}`,
    );
    expect(incomingPostbackTemplate(base, undefined, "scaleo")).toBe(
      `${base}?secret={secret}&click_id={aff_click_id}&status=REPLACE_GOAL&revenue={payout}`,
    );
    expect(incomingPostbackTemplate(base, undefined, "unknown")).toBe(incomingPostbackTemplate(base));
  });
});
