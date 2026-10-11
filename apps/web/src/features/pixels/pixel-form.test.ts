import { describe, expect, it } from "vitest";

import { toPixelInput, type PixelFormValues } from "@/features/pixels/pixel-form";

const base: PixelFormValues = {
  name: "Pixel",
  provider: "facebook",
  pixelId: "123",
  events: ["CPA_ACCEPT"],
  accessToken: "",
  clearAccessToken: false,
  testEventCode: "TEST1",
  url: "https://stale.example.com/p",
  eventNames: { CPA_ACCEPT: " Deposit ", CPA_HOLD: "Lead" },
  status: "active",
};

describe("toPixelInput", () => {
  it("keeps the saved token when none is typed", () => {
    const input = toPixelInput(base);
    expect("accessToken" in input).toBe(false);
    expect(input.url).toBe("");
    expect(input.testEventCode).toBe("TEST1");
  });

  it("sends a typed token, and an empty one to remove it", () => {
    expect(toPixelInput({ ...base, accessToken: "EAAB" }).accessToken).toBe("EAAB");
    expect(toPixelInput({ ...base, accessToken: "EAAB", clearAccessToken: true }).accessToken).toBe("");
  });

  it("sends event names only for selected events, trimmed", () => {
    expect(toPixelInput(base).eventNames).toEqual({ CPA_ACCEPT: "Deposit" });
  });

  it("sends only the URL for a generic pixel", () => {
    const input = toPixelInput({ ...base, provider: "generic", accessToken: "x" });
    expect(input.url).toBe("https://stale.example.com/p");
    expect(input.pixelId).toBe("");
    expect(input.testEventCode).toBe("");
    expect("accessToken" in input).toBe(false);
  });

  it("drops the test event code for Snapchat, which has none", () => {
    expect(toPixelInput({ ...base, provider: "snapchat" }).testEventCode).toBe("");
  });
});
