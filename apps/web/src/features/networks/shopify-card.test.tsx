import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import type { Network, ShopifyConnection } from "@/lib/api/networks";
import { ApiError } from "@/lib/api/client";

let shop: ShopifyConnection = { connected: false, shopDomain: "", secretPreview: "", webhookUrl: "https://t.example/shopify/01NET", available: true };
const mutate = vi.fn();
const remove = vi.fn();
vi.mock("@/hooks/use-networks", () => ({
  useShopify: () => ({ isLoading: false, isError: false, data: shop }),
  useUpdateShopify: () => ({ mutate, isPending: false }),
  useDeleteShopify: () => ({ mutate: remove, isPending: false }),
}));

const { ShopifyCard, shopifySnippet } = await import("@/features/networks/shopify-card");

const network = { id: "01NET", incomingPostbackUrl: "https://t.example/postback/01NET", templateKey: "own_site" } as Network;

// Audit UNI-4 part 3: connecting a store needs its signing key; the card
// gives the webhook URL and the theme tag; disconnecting asks first.
describe("ShopifyCard", () => {
  beforeEach(() => {
    mutate.mockReset();
    remove.mockReset();
  });

  it("connects with the domain and the signing key, and shows field errors", () => {
    render(<ShopifyCard network={network} canWrite />);
    expect(screen.getByText("https://t.example/shopify/01NET")).toBeInTheDocument();
    expect(screen.getByText(shopifySnippet("https://t.example"))).toBeInTheDocument();
    const connect = screen.getByRole("button", { name: "Connect" }) as HTMLButtonElement;
    fireEvent.change(screen.getByLabelText("Store domain"), { target: { value: "my-shop.myshopify.com" } });
    expect(connect.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Webhook signing key"), { target: { value: "shpss_key" } });
    fireEvent.click(connect);
    expect(mutate.mock.calls[0][0]).toEqual({ shopDomain: "my-shop.myshopify.com", secret: "shpss_key" });
    const onError = mutate.mock.calls[0][1].onError as (err: Error) => void;
    act(() => onError(new ApiError(400, "validation", "invalid", { shopDomain: "the store's myshopify.com domain" })));
    expect(screen.getByText("the store's myshopify.com domain")).toBeInTheDocument();
  });

  it("keeps the stored key and disconnects after confirming", () => {
    shop = { ...shop, connected: true, shopDomain: "my-shop.myshopify.com", secretPreview: "_key" };
    render(<ShopifyCard network={network} canWrite />);
    expect((screen.getByLabelText("Webhook signing key") as HTMLInputElement).placeholder).toContain("_key");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(mutate.mock.calls[0][0]).toEqual({ shopDomain: "my-shop.myshopify.com", secret: "" });
    fireEvent.click(screen.getByRole("button", { name: /Disconnect/ }));
    expect(remove).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Disconnect the store" }));
    expect(remove).toHaveBeenCalledTimes(1);
  });

  it("says when the installation cannot connect stores", () => {
    shop = { ...shop, available: false };
    render(<ShopifyCard network={network} canWrite />);
    expect(screen.getByText(/SHOP_WEBHOOK_ENCRYPTION_KEY/)).toBeInTheDocument();
    expect(screen.queryByLabelText("Store domain")).not.toBeInTheDocument();
  });
});
