import { describe, expect, it } from "vitest";

import { SOURCE_TEMPLATES } from "@/lib/source-templates";
import { buildTrackingUrl, PARAM_KEYS, presetForSource } from "@/lib/tracking-link-params";

/** The tokens apps/internal/macro resolves in a postback (Go's Resolve);
 * anything else would reach the network literally. */
const RESOLVED = new Set(["{click_id}", "{external_click_id}", "{status}", "{revenue}", "{currency}", "{campaign_id}", "{offer_id}", "{country}", "{device}"]);

// Audit 2026-10-15, INT-1: the traffic source templates.
describe("SOURCE_TEMPLATES", () => {
  it("has the eight networks of the list, each with a source", () => {
    expect(SOURCE_TEMPLATES.map((t) => t.name)).toEqual(["PropellerAds", "ExoClick", "Adsterra", "TrafficStars", "Clickadu", "HilltopAds", "Galaksion", "EvaDav"]);
    for (const t of SOURCE_TEMPLATES) {
      expect(t.sources.length, t.key).toBeGreaterThan(0);
      for (const url of t.sources) expect(url, t.key).toMatch(/^https:\/\//);
    }
  });

  it("puts the network's click id into external_click_id and only keys the tracker reads", () => {
    for (const t of SOURCE_TEMPLATES) {
      expect(t.link.external_click_id, t.key).toBeTruthy();
      for (const key of Object.keys(t.link)) expect(PARAM_KEYS, `${t.key}: ${key}`).toContain(key);
      for (const sub of Object.keys(t.subs)) expect(t.link[sub], `${t.key}: ${sub}`).toBeTruthy();
    }
  });

  it("keeps every network macro literal in the link", () => {
    for (const t of SOURCE_TEMPLATES) {
      const url = buildTrackingUrl("https://t.example/t/go", t.link);
      for (const [key, macro] of Object.entries(t.link)) expect(url, t.key).toContain(`${key}=${macro}`);
    }
  });

  it("sends the click id back with macros FLOX resolves", () => {
    for (const t of SOURCE_TEMPLATES) {
      expect(t.postbackUrl, t.key).toContain("{external_click_id}");
      expect(t.postbackUrl, t.key).toMatch(/^https:\/\//);
      for (const token of t.postbackUrl.match(/\{[a-z0-9_]+\}/g) ?? []) expect(RESOLVED, `${t.key}: ${token}`).toContain(token);
    }
  });
});

describe("presetForSource", () => {
  it("prefers the source's template, then its type's platform", () => {
    expect(presetForSource({ type: "Push", templateKey: "propellerads" })).toBe("propellerads");
    expect(presetForSource({ type: "Facebook", templateKey: "" })).toBe("facebook");
    expect(presetForSource({ type: "Push", templateKey: "unknown" })).toBeNull();
    expect(presetForSource(undefined)).toBeNull();
  });
});
