import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import type { MetricaConnection } from "@/lib/api/ad-account-connections";
import { ApiError } from "@/lib/api/client";

const base: MetricaConnection = {
  connected: false,
  tokenPreview: "",
  enabled: false,
  since: null,
  counterId: "",
  currency: "RUB",
  leadGoal: "",
  purchaseGoal: "",
  repurchaseGoal: "",
  counts: {},
  skipped: {},
  processing: 0,
  lastError: "",
  lastErrorAt: null,
  lastSentAt: null,
};
let status: MetricaConnection = base;

const mutate = vi.fn();
const remove = vi.fn();
vi.mock("@/hooks/use-auth", () => ({ usePermission: () => true }));
vi.mock("@/hooks/use-ad-account-connection", () => ({
  useMetricaConnection: () => ({ isLoading: false, isError: false, data: status }),
  useUpdateMetricaConnection: () => ({ mutate, isPending: false }),
  useDeleteMetricaConnection: () => ({ mutate: remove, isPending: false }),
}));

const { MetricaSection } = await import("@/features/traffic-sources/metrica-section");

// Audit UNI-2 part 3: connecting a counter, what came of the export, and
// disconnecting only after a confirmation.
describe("MetricaSection", () => {
  beforeEach(() => {
    mutate.mockReset();
    remove.mockReset();
  });

  it("connects only with a token, and shows the server's field errors", () => {
    status = base;
    render(<MetricaSection trafficSourceId="ts" />);
    const connect = screen.getByRole("button", { name: /Connect/ }) as HTMLButtonElement;
    fireEvent.change(screen.getByLabelText("Counter number"), { target: { value: "4242" } });
    expect(connect.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("OAuth token"), { target: { value: "y0_secret" } });
    fireEvent.change(screen.getByLabelText("Goal for purchases"), { target: { value: "purchase" } });
    expect(connect.disabled).toBe(false);
    fireEvent.click(connect);
    expect(mutate.mock.calls[0][0]).toEqual({
      enabled: false,
      counterId: "4242",
      token: "y0_secret",
      currency: "RUB",
      leadGoal: "",
      purchaseGoal: "purchase",
      repurchaseGoal: "",
    });
    const onError = mutate.mock.calls[0][1].onError as (err: Error) => void;
    act(() => onError(new ApiError(400, "validation", "invalid", { token: "Metrica refused the token" })));
    expect(screen.getByText("Metrica refused the token")).toBeTruthy();
  });

  it("shows what was sent, matched and skipped; disconnects after confirming", () => {
    status = {
      ...base,
      connected: true,
      tokenPreview: "abcd",
      enabled: true,
      counterId: "4242",
      purchaseGoal: "purchase",
      counts: { sent: 3, failed: 1 },
      skipped: { no_yandex_click: 2, visit_too_old: 1 },
      processing: 2,
      lastError: "LINKAGE_FAILURE",
    };
    render(<MetricaSection trafficSourceId="ts" />);
    expect(screen.getByText("3 sent")).toBeTruthy();
    expect(screen.getByText("2 being matched by Metrica")).toBeTruthy();
    expect(screen.getByText(/2 conversions — their clicks had no yclid/)).toBeTruthy();
    expect(screen.getByText(/older than 21 days/)).toBeTruthy();
    expect(screen.getByText(/LINKAGE_FAILURE/)).toBeTruthy();
    expect((screen.getByLabelText("OAuth token") as HTMLInputElement).placeholder).toContain("abcd");

    fireEvent.click(screen.getByRole("button", { name: /Disconnect/ }));
    expect(remove).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Disconnect the counter" }));
    expect(remove).toHaveBeenCalledTimes(1);
  });
});
