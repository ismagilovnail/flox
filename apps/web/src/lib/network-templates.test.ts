import { describe, expect, it } from "vitest";

import { CPA_STATUSES } from "@/lib/api/conversions";
import { NETWORK_TEMPLATES, OWN_SITE_KEY, networkTemplate } from "@/lib/network-templates";
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

// Audit 2026-10-24, UNI-1: a site reporting its own conversions.
describe("own site", () => {
  it("is not offered as a platform and maps plain-word statuses to every FLOX status", () => {
    expect(NETWORK_TEMPLATES.map((t) => t.key)).not.toContain(OWN_SITE_KEY);
    const own = networkTemplate(OWN_SITE_KEY)!;
    expect(Object.fromEntries(own.statusMappings.map((m) => [m.networkStatus, m.floxStatus]))).toEqual({
      lead: "CPA_HOLD",
      purchase: "CPA_ACCEPT",
      repeat_purchase: "CPA_REDEP",
      refund: "CPA_DECLINE",
      cancel: "CPA_DECLINE",
      spam: "CPA_TRASH",
    });
    expect(new Set(own.statusMappings.map((m) => m.floxStatus))).toEqual(new Set(CPA_STATUSES));
  });

  it("shows the site's developer what to fill in", () => {
    expect(incomingPostbackTemplate("https://t.example/postback/N1", undefined, OWN_SITE_KEY)).toBe(
      "https://t.example/postback/N1?secret={secret}&click_id={click_id}&status=purchase&revenue={amount}&currency={currency}&txn_id={order_id}",
    );
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
