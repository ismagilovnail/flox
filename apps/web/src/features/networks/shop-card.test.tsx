import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import type { Network, ShopConnection, ShopPlatform } from "@/lib/api/networks";
import { ApiError } from "@/lib/api/client";

const empty = (platform: ShopPlatform): ShopConnection => ({
  connected: false,
  shopDomain: "",
  secretPreview: "",
  webhookUrl: `https://t.example/${platform}/01NET`,
  available: true,
});
let shops: Record<ShopPlatform, ShopConnection>;
const mutate = { shopify: vi.fn(), woocommerce: vi.fn(), tilda: vi.fn() };
const remove = { shopify: vi.fn(), woocommerce: vi.fn(), tilda: vi.fn() };
vi.mock("@/hooks/use-networks", () => ({
  useShop: (_id: string, p: ShopPlatform) => ({ isLoading: false, isError: false, error: null, data: shops[p] }),
  useUpdateShop: (_id: string, p: ShopPlatform) => ({ mutate: mutate[p], isPending: false }),
  useDeleteShop: (_id: string, p: ShopPlatform) => ({ mutate: remove[p], isPending: false }),
}));

const { ShopCard, floxScriptTag, newApiKey, wooCheckoutSnippet } = await import("@/features/networks/shop-card");

const network = { id: "01NET", incomingPostbackUrl: "https://t.example/postback/01NET", templateKey: "own_site" } as Network;

// Audit UNI-4 parts 3-5: one card, a tab per platform. Connecting a store
// needs its signing key; the tab gives the webhook URL, the site tag (and
// for WooCommerce the checkout snippet); disconnecting asks first.
describe("ShopCard", () => {
  beforeEach(() => {
    shops = { shopify: empty("shopify"), woocommerce: empty("woocommerce"), tilda: empty("tilda") };
    for (const f of [...Object.values(mutate), ...Object.values(remove)]) f.mockReset();
  });

  it("connects a Shopify store with the domain and the signing key, and shows field errors", () => {
    render(<ShopCard network={network} canWrite />);
    expect(screen.getByText("https://t.example/shopify/01NET")).toBeInTheDocument();
    expect(screen.getByText(floxScriptTag("https://t.example"))).toBeInTheDocument();
    const connect = screen.getByRole("button", { name: "Connect" }) as HTMLButtonElement;
    fireEvent.change(screen.getByLabelText("Store domain"), { target: { value: "my-shop.myshopify.com" } });
    expect(connect.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Webhook signing key"), { target: { value: "shpss_key" } });
    fireEvent.click(connect);
    expect(mutate.shopify.mock.calls[0][0]).toEqual({ shopDomain: "my-shop.myshopify.com", secret: "shpss_key" });
    const onError = mutate.shopify.mock.calls[0][1].onError as (err: Error) => void;
    act(() => onError(new ApiError(400, "validation", "invalid", { shopDomain: "the store's myshopify.com domain" })));
    expect(screen.getByText("the store's myshopify.com domain")).toBeInTheDocument();
  });

  it("keeps the stored key and disconnects after confirming", () => {
    shops.shopify = { ...shops.shopify, connected: true, shopDomain: "my-shop.myshopify.com", secretPreview: "_key" };
    render(<ShopCard network={network} canWrite />);
    expect((screen.getByLabelText("Webhook signing key") as HTMLInputElement).placeholder).toContain("_key");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(mutate.shopify.mock.calls[0][0]).toEqual({ shopDomain: "my-shop.myshopify.com", secret: "" });
    fireEvent.click(screen.getByRole("button", { name: /Disconnect/ }));
    expect(remove.shopify).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Disconnect the store" }));
    expect(remove.shopify).toHaveBeenCalledTimes(1);
  });

  it("opens on the connected WooCommerce store, with its webhook URL and the checkout snippet", () => {
    shops.woocommerce = { ...shops.woocommerce, connected: true, shopDomain: "shop.example.com", secretPreview: "cret" };
    render(<ShopCard network={network} canWrite />);
    expect(screen.getByRole("tab", { name: /WooCommerce/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("https://t.example/woocommerce/01NET")).toBeInTheDocument();
    expect(screen.getByText((_, el) => el?.tagName === "PRE" && el.textContent === wooCheckoutSnippet)).toBeInTheDocument();
    expect(wooCheckoutSnippet).toContain("$_COOKIE['flox_click_id']");
    fireEvent.change(screen.getByLabelText("Site address"), { target: { value: "example.com/shop" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(mutate.woocommerce.mock.calls[0][0]).toEqual({ shopDomain: "example.com/shop", secret: "" });
    expect(mutate.shopify).not.toHaveBeenCalled();
  });

  it("connects a Tilda site with a generated API key, the address optional", () => {
    render(<ShopCard network={network} canWrite />);
    fireEvent.mouseDown(screen.getByRole("tab", { name: "Tilda" }));
    expect(screen.getByText("https://t.example/tilda/01NET")).toBeInTheDocument();
    const key = screen.getByLabelText("API key") as HTMLInputElement;
    expect(key.type).toBe("text");
    fireEvent.click(screen.getByRole("button", { name: "Generate a key" }));
    expect(key.value).toMatch(/^[0-9a-f]{48}$/);
    fireEvent.click(screen.getByRole("button", { name: "Connect" }));
    expect(mutate.tilda.mock.calls[0][0]).toEqual({ shopDomain: "", secret: key.value });
    expect(newApiKey()).not.toBe(key.value);
  });

  it("says when the installation cannot connect stores", () => {
    shops.shopify = { ...shops.shopify, available: false };
    render(<ShopCard network={network} canWrite />);
    expect(screen.getByText(/SHOP_WEBHOOK_ENCRYPTION_KEY/)).toBeInTheDocument();
    expect(screen.queryByLabelText("Store domain")).not.toBeInTheDocument();
  });
});
