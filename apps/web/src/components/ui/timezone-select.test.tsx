import { beforeAll, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import { TimezoneSelect } from "@/components/ui/timezone-select";
import { otherTimezoneIds, timezoneFallbackName } from "@/lib/timezones";

beforeAll(() => {
  // cmdk measures and scrolls its list; jsdom has neither.
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
  Element.prototype.scrollIntoView ??= () => {};
});

function open(value: string, onValueChange = vi.fn()) {
  render(<TimezoneSelect value={value} onValueChange={onValueChange} />);
  fireEvent.click(screen.getByRole("combobox"));
  return onValueChange;
}

// Audit UX-4: every IANA zone, searchable; the CIS cities the curated list
// lacked are pinned with their names.
describe("TimezoneSelect", () => {
  it("lists the CIS cities and every other zone the runtime knows", () => {
    const others = otherTimezoneIds();
    expect(others.length).toBeGreaterThan(300);
    expect(others).not.toContain("Europe/Moscow");
    expect(others).not.toContain("Europe/Kiev");
    open("Europe/Moscow");
    for (const city of ["Kyiv", "Almaty", "Istanbul", "Minsk"]) expect(screen.getByText(city)).toBeInTheDocument();
    expect(screen.getByText(timezoneFallbackName("America/Buenos_Aires"))).toBeInTheDocument();
  });

  it("finds a zone by city, id or offset and picks it", () => {
    const onValueChange = open("UTC");
    fireEvent.change(screen.getByPlaceholderText(/City, zone/), { target: { value: "Almaty" } });
    expect(screen.queryByText("Moscow")).not.toBeInTheDocument();
    fireEvent.click(screen.getByText("Almaty"));
    expect(onValueChange).toHaveBeenCalledWith("Asia/Almaty");
  });

  it("keeps a value outside the lists", () => {
    open("Etc/GMT-14");
    expect(screen.getAllByText(/GMT-14/).length).toBeGreaterThan(0);
  });

  it("names a zone without a translation readably", () => {
    expect(timezoneFallbackName("America/Argentina/Buenos_Aires")).toBe("Buenos Aires (America/Argentina)");
    expect(timezoneFallbackName("UTC")).toBe("UTC");
  });
});
