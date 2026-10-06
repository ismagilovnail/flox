import { describe, expect, it } from "vitest";

import { buildTrackingUrl, PLATFORM_PRESETS, platformForSourceType } from "@/lib/tracking-link-params";

describe("buildTrackingUrl", () => {
  it("returns the bare url when there are no params", () => {
    expect(buildTrackingUrl("http://t.example/t/go", {})).toBe("http://t.example/t/go");
    expect(buildTrackingUrl("http://t.example/t/go", { sub1: "  " })).toBe("http://t.example/t/go");
  });

  it("keeps platform macros literal and encodes everything else", () => {
    const url = buildTrackingUrl("http://t.example/t/go", {
      utm_campaign: "{{campaign.name}}",
      sub1: "a b&c",
      sub2: "x_{{ad.id}}_y z",
    });
    expect(url).toBe("http://t.example/t/go?utm_campaign={{campaign.name}}&sub1=a%20b%26c&sub2=x_{{ad.id}}_y%20z");
  });

  it("orders keys utm → sub1..10 → ttclid and ignores unknown keys", () => {
    const url = buildTrackingUrl("http://t.example/t/go", { sub10: "j", sub2: "b", ttclid: "T", utm_source: "s", evil: "1" });
    expect(url).toBe("http://t.example/t/go?utm_source=s&sub2=b&sub10=j&ttclid=T");
  });

  it("appends with & when the base already has a query", () => {
    expect(buildTrackingUrl("http://t.example/t/go?x=1", { sub1: "a" })).toBe("http://t.example/t/go?x=1&sub1=a");
  });

  it("builds the TikTok preset with its double-underscore macros intact", () => {
    const url = buildTrackingUrl("http://t.example/t/go", PLATFORM_PRESETS.tiktok);
    expect(url).toContain("sub1=__CAMPAIGN_ID__&sub2=__AID__&sub3=__CID__");
    expect(url).toContain("ttclid=__CALLBACK_PARAM__");
  });
});

describe("platformForSourceType", () => {
  it("maps Facebook/TikTok and nothing else", () => {
    expect(platformForSourceType("Facebook")).toBe("facebook");
    expect(platformForSourceType("TikTok")).toBe("tiktok");
    expect(platformForSourceType("Google")).toBeNull();
    expect(platformForSourceType(undefined)).toBeNull();
  });
});
