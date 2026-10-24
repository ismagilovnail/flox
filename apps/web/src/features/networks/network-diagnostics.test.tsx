import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import type { NetworkDiagnostics as Diagnostics } from "@/lib/api/networks";

// Redeposits without txn_id (audit 2026-10-15, DATA-2).
let data: Diagnostics | undefined;
vi.mock("@/hooks/use-networks", () => ({ useNetworkDiagnostics: () => ({ data }) }));

const { NetworkDiagnostics } = await import("@/features/networks/network-diagnostics");

const diag = (received: number, duplicates: number): Diagnostics => ({
  windowDays: 30,
  redepWithoutTxn: { received, duplicates, lastAt: received ? new Date().toISOString() : null },
});

afterEach(() => {
  data = undefined;
});

describe("NetworkDiagnostics", () => {
  it("says how many repeat purchases were lost and how to fix it", () => {
    data = diag(5, 3);
    render(<NetworkDiagnostics networkId="n1" />);
    expect(screen.getByText(/3 repeat purchases lost/)).toBeTruthy();
    expect(screen.getByText(/txn_id=\{transaction_id\}/)).toBeTruthy();
  });

  it("warns before anything is lost", () => {
    data = diag(2, 0);
    render(<NetworkDiagnostics networkId="n1" />);
    expect(screen.getByText("Repeat purchases come without txn_id")).toBeTruthy();
  });

  it("renders nothing when every repeat purchase carries txn_id or nothing is loaded", () => {
    const { container, rerender } = render(<NetworkDiagnostics networkId="n1" />);
    expect(container.innerHTML).toBe("");
    data = diag(0, 0);
    rerender(<NetworkDiagnostics networkId="n1" />);
    expect(container.innerHTML).toBe("");
  });
});
