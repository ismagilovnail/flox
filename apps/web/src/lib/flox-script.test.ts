import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The script apps/tracker serves at /flox.js (audit UNI-1 part 2), run as a
// page would run it: it keeps the click id from the URL and reports with
// sendBeacon to the tracker it was loaded from.
const source = readFileSync(resolve(__dirname, "../../../tracker/static/flox.js"), "utf8");

function run(attrs: Record<string, string>) {
  const el = document.createElement("script");
  el.src = "https://t.example/flox.js";
  for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
  Object.defineProperty(document, "currentScript", { configurable: true, get: () => el });
  new Function(source)();
}

const beacon = vi.fn<(url: string) => boolean>(() => true);

beforeEach(() => {
  Object.defineProperty(navigator, "sendBeacon", { configurable: true, value: beacon });
  document.cookie = "flox_click_id=; max-age=0; path=/";
  localStorage.clear();
  sessionStorage.clear();
});
afterEach(() => {
  beacon.mockClear();
  window.history.replaceState({}, "", "/");
});

describe("flox.js", () => {
  it("keeps the click id from the landing URL and reports a purchase later", () => {
    window.history.replaceState({}, "", "/?utm=x&click_id=01CLICK");
    run({ "data-source": "01NET" });
    expect(document.cookie).toContain("flox_click_id=01CLICK");
    expect(beacon).not.toHaveBeenCalled();

    window.history.replaceState({}, "", "/thanks");
    run({ "data-source": "01NET", "data-event": "purchase", "data-revenue": "1990", "data-currency": "RUB", "data-order": "A-1" });
    expect(beacon).toHaveBeenCalledWith("https://t.example/c/01NET?click_id=01CLICK&status=purchase&revenue=1990&currency=RUB&txn_id=A-1");
  });

  it("reports from code, and only what it has", () => {
    window.history.replaceState({}, "", "/?sub=01OTHER");
    run({ "data-source": "01NET", "data-param": "sub" });
    (window as unknown as { flox: (s: string) => boolean }).flox("lead");
    expect(beacon).toHaveBeenCalledWith("https://t.example/c/01NET?click_id=01OTHER&status=lead");
  });

  it("sends nothing without a click id or a source", () => {
    run({ "data-source": "01NET", "data-event": "purchase" });
    window.history.replaceState({}, "", "/?click_id=01CLICK");
    run({ "data-event": "purchase" });
    expect(beacon).not.toHaveBeenCalled();
  });

  // Audit UNI-4: on a Shopify storefront the click goes on the cart as an
  // attribute, once per click and session; elsewhere nothing is posted.
  it("puts the click on a Shopify cart once", async () => {
    const fetchMock = vi.fn(async () => new Response("{}", { status: 200 }));
    Object.defineProperty(window, "fetch", { configurable: true, value: fetchMock });
    window.history.replaceState({}, "", "/?click_id=01SHOP");
    run({});
    expect(fetchMock).not.toHaveBeenCalled();

    (window as unknown as { Shopify: unknown }).Shopify = { shop: "my-shop.myshopify.com", routes: { root: "/en/" } };
    run({});
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe("/en/cart/update.js");
    expect(JSON.parse(init.body as string)).toEqual({ attributes: { flox_click_id: "01SHOP" } });
    await vi.waitFor(() => expect(sessionStorage.getItem("flox_click_id_cart")).toBe("01SHOP"));

    window.history.replaceState({}, "", "/products/x");
    run({});
    expect(fetchMock).toHaveBeenCalledTimes(1);
    delete (window as unknown as { Shopify?: unknown }).Shopify;
  });
});
