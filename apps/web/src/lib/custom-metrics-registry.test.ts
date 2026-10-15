import { describe, expect, it } from "vitest";

import { METRIC_CATALOG, surfaceCanCompute } from "@/lib/custom-metrics-registry";
import { evaluateFormula, validateFormula } from "@/lib/formula-engine";

describe("push metrics in the report builder", () => {
  it("are catalog entries of the push source, not regular ones", () => {
    const push = METRIC_CATALOG.filter((m) => m.dataSource === "push").map((m) => m.id);
    expect(push.sort()).toEqual(["push_ctr", "push_delivered", "push_opened", "push_sent", "push_unsubscribed"]);
    expect(METRIC_CATALOG.filter((m) => m.id.startsWith("push_")).every((m) => m.dataSource === "push")).toBe(true);
  });

  it("let a push formula target the report builder and Push statistics, not the campaigns table", () => {
    const ids = ["push_opened", "push_delivered"];
    expect(surfaceCanCompute("report_builder", ids)).toBe(true);
    expect(surfaceCanCompute("push_stats", ids)).toBe(true);
    expect(surfaceCanCompute("campaigns_table", ids)).toBe(false);
  });

  it("still refuse a formula mixing push and regular metrics (§30.5)", () => {
    expect(validateFormula("{clicks} + {push_sent}", METRIC_CATALOG).valid).toBe(false);
  });

  it("evaluate empty on a row without push numbers", () => {
    expect(evaluateFormula("{push_opened} / {push_sent}", { push_opened: null, push_sent: null })).toBeNull();
    expect(evaluateFormula("{push_opened} / {push_sent}", { push_opened: 3, push_sent: 12 })).toBe(0.25);
  });
});
