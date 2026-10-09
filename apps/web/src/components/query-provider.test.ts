import { describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api/client";

import { shouldRetry } from "./query-provider";

describe("shouldRetry", () => {
  it("does not retry a 4xx: the same request fails the same way", () => {
    expect(shouldRetry(0, new ApiError(422, "query_too_heavy", "too heavy"))).toBe(false);
    expect(shouldRetry(0, new ApiError(404, "not_found", "gone"))).toBe(false);
  });

  it("retries network errors and 5xx up to three times", () => {
    expect(shouldRetry(0, new ApiError(503, "unavailable", "down"))).toBe(true);
    expect(shouldRetry(2, new Error("Failed to fetch"))).toBe(true);
    expect(shouldRetry(3, new Error("Failed to fetch"))).toBe(false);
  });
});
