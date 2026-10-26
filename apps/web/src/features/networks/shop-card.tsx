"use client";

import * as React from "react";
import { CopyIcon, Unlink2Icon } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Mono } from "@/components/ui/typography";
import { trackerOrigin } from "@/features/networks/browser-conversions-card";
import { useDeleteShop, useShop, useUpdateShop } from "@/hooks/use-networks";
import type { Network, ShopConnection, ShopPlatform } from "@/lib/api/networks";
import { ApiError } from "@/lib/api/client";

type Field = "shopDomain" | "secret";

/** The tag a store adds to every page: it keeps the click (a first-party
 * cookie; on Shopify it also puts it on the cart, which the order keeps). */
export function floxScriptTag(origin: string): string {
  return `<script src="${origin}/flox.js" async></script>`;
}

/** The PHP a WooCommerce store adds (Code Snippets or the theme's
 * functions.php) so the order carries the click flox.js keeps — classic
 * and block checkout (docs/shop-connections.md). */
export const wooCheckoutSnippet = `// FLOX: keep the click flox.js remembered on the order.
function flox_click_to_order( $order ) {
\tif ( empty( $_COOKIE['flox_click_id'] ) || $order->get_meta( 'flox_click_id' ) ) {
\t\treturn;
\t}
\t$order->update_meta_data( 'flox_click_id', sanitize_text_field( wp_unslash( $_COOKIE['flox_click_id'] ) ) );
\t$order->save();
}
add_action( 'woocommerce_checkout_order_created', 'flox_click_to_order' );
add_action( 'woocommerce_store_api_checkout_order_processed', 'flox_click_to_order' );`;

const platforms: ShopPlatform[] = ["shopify", "woocommerce", "tilda"];

/** A Tilda receiver's API key is the owner's to choose: FLOX offers one
 * (it has to be copied into Tilda, so that field is shown in clear). */
export function newApiKey(): string {
  const bytes = new Uint8Array(24);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

/** An own site that is an online store reports its orders by webhook
 * (audit UNI-4, docs/shop-connections.md): paid orders become purchases or
 * repeat purchases, refunds take back what was paid back. One tab per
 * platform; the store signs each webhook, FLOX keeps the key encrypted and
 * shows only its end. */
export function ShopCard({ network, canWrite }: { network: Network; canWrite: boolean }) {
  const { t } = useTranslation("networks");
  const shopify = useShop(network.id, "shopify");
  const woo = useShop(network.id, "woocommerce");
  const tildaSite = useShop(network.id, "tilda");
  const queries: Record<ShopPlatform, ShopQuery> = { shopify, woocommerce: woo, tilda: tildaSite };
  const connected = (p: ShopPlatform) => !!queries[p].data?.connected;
  const [tab, setTab] = React.useState<ShopPlatform | null>(null);
  const current = tab ?? platforms.find(connected) ?? "shopify";

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("shop.title")}</CardTitle>
        <CardDescription>{t("shop.description")}</CardDescription>
      </CardHeader>
      <CardContent>
        <Tabs value={current} onValueChange={(v) => setTab(v as ShopPlatform)}>
          <TabsList>
            {platforms.map((p) => (
              <TabsTrigger key={p} value={p}>
                {t(`${p}.tab`)}
                {connected(p) && (
                  <Badge variant="success" className="ml-1.5">
                    {t("shop.connected")}
                  </Badge>
                )}
              </TabsTrigger>
            ))}
          </TabsList>
          {platforms.map((p) => (
            <TabsContent key={p} value={p} className="pt-3">
              <ShopPanel network={network} platform={p} canWrite={canWrite} query={queries[p]} />
            </TabsContent>
          ))}
        </Tabs>
      </CardContent>
    </Card>
  );
}

type ShopQuery = { isLoading: boolean; isError: boolean; error: Error | null; data?: ShopConnection };

function ShopPanel({ network, platform, canWrite, query }: { network: Network; platform: ShopPlatform; canWrite: boolean; query: ShopQuery }) {
  const { t } = useTranslation("networks");
  const update = useUpdateShop(network.id, platform);
  const remove = useDeleteShop(network.id, platform);
  const [domain, setDomain] = React.useState<string | null>(null);
  const [secret, setSecret] = React.useState("");
  const [errors, setErrors] = React.useState<Partial<Record<Field, string>>>({});
  const [confirming, setConfirming] = React.useState(false);
  const origin = trackerOrigin(network.incomingPostbackUrl);
  const id = (field: string) => `${platform}-${field}`;

  const data = query.data;
  const shopDomain = domain ?? data?.shopDomain ?? "";

  function save() {
    update.mutate(
      { shopDomain, secret },
      {
        onSuccess: () => {
          setDomain(null);
          setSecret("");
          setErrors({});
          toast(t(`${platform}.saved`));
        },
        onError: (err) => {
          if (err instanceof ApiError && err.fields) setErrors(err.fields as Partial<Record<Field, string>>);
          toast.error(t("shop.saveError"), { description: err.message });
        },
      },
    );
  }

  function disconnect() {
    remove.mutate(undefined, {
      onSuccess: () => {
        setConfirming(false);
        setDomain(null);
        toast(t(`${platform}.disconnected`));
      },
      onError: (err) => toast.error(t("shop.disconnectError"), { description: err.message }),
    });
  }

  function copy(value: string) {
    navigator.clipboard.writeText(value).then(
      () => toast(t("browser.copied")),
      () => toast.error(t("incoming.copyFailed")),
    );
  }

  if (query.isLoading) return <p className="text-xs text-muted-foreground">{t("shop.loading")}</p>;
  if (query.isError)
    return (
      <p className="text-xs text-danger">
        {t("shop.loadError")} {query.error?.message}
      </p>
    );
  if (!data) return null;
  if (!data.available) return <p className="text-xs text-warning">{t("shop.unavailable")}</p>;

  return (
    <div className="flex flex-col gap-4">
      <p className="text-xs text-muted-foreground">{t(`${platform}.description`)}</p>
      <fieldset disabled={!canWrite || update.isPending} className="contents">
        <div className="grid gap-2 sm:grid-cols-2">
          <div className="grid gap-1.5">
            <Label htmlFor={id("domain")}>{t(`${platform}.domainLabel`)}</Label>
            <Input
              id={id("domain")}
              className="font-mono text-xs"
              placeholder={t(`${platform}.domainPlaceholder`)}
              value={shopDomain}
              onChange={(e) => setDomain(e.target.value)}
              aria-invalid={!!errors.shopDomain}
            />
            {errors.shopDomain && <p className="text-xs text-danger">{errors.shopDomain}</p>}
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor={id("secret")}>{t(`${platform}.secretLabel`)}</Label>
            <Input
              id={id("secret")}
              type={platform === "tilda" ? "text" : "password"}
              autoComplete="off"
              className="font-mono text-xs"
              placeholder={data.connected ? t("shop.secretKeep", { last4: data.secretPreview }) : ""}
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
              aria-invalid={!!errors.secret}
            />
            {errors.secret && <p className="text-xs text-danger">{errors.secret}</p>}
            {platform === "tilda" && canWrite && (
              <Button type="button" size="sm" variant="outline" className="justify-self-start" onClick={() => setSecret(newApiKey())}>
                {t("tilda.generate")}
              </Button>
            )}
          </div>
        </div>
        <p className="text-xs text-muted-foreground">{t(`${platform}.secretHint`)}</p>
        {canWrite && (
          <div className="flex flex-wrap gap-2">
            <Button type="button" size="sm" onClick={save} disabled={update.isPending || (!data.connected && !secret.trim())}>
              {data.connected ? t("shop.save") : t("shop.connect")}
            </Button>
            {data.connected &&
              (confirming ? (
                <>
                  <Button type="button" size="sm" variant="outline" onClick={() => setConfirming(false)}>
                    {t("shop.cancel")}
                  </Button>
                  <Button type="button" size="sm" variant="destructive" onClick={disconnect} disabled={remove.isPending}>
                    {t("shop.confirmDisconnect")}
                  </Button>
                </>
              ) : (
                <Button type="button" size="sm" variant="outline" onClick={() => setConfirming(true)}>
                  <Unlink2Icon className="size-3.5" /> {t("shop.disconnect")}
                </Button>
              ))}
          </div>
        )}
      </fieldset>

      {data.webhookUrl && (
        <CopyBlock label={t("shop.webhookUrl")} copyLabel={t("shop.copyWebhook")} value={data.webhookUrl} hint={t(`${platform}.webhookHint`)} onCopy={copy} />
      )}
      {origin && (
        <CopyBlock label={t(`${platform}.theme`)} copyLabel={t("shop.copyTheme")} value={floxScriptTag(origin)} hint={t(`${platform}.themeHint`)} onCopy={copy} />
      )}
      {platform === "woocommerce" && (
        <CopyBlock label={t("woocommerce.checkout")} copyLabel={t("woocommerce.copyCheckout")} value={wooCheckoutSnippet} hint={t("woocommerce.checkoutHint")} onCopy={copy} pre />
      )}
      <p className="text-xs text-warning">{t(`${platform}.limits`)}</p>
    </div>
  );
}

function CopyBlock({ label, copyLabel, value, hint, onCopy, pre }: { label: string; copyLabel: string; value: string; hint: string; onCopy: (v: string) => void; pre?: boolean }) {
  return (
    <div className="grid gap-1.5">
      <p className="text-xs font-medium">{label}</p>
      <div className="flex items-start gap-2 rounded-md border border-border bg-muted px-3 py-2">
        {pre ? (
          <pre className="min-w-0 flex-1 overflow-x-auto font-mono text-xs whitespace-pre">{value}</pre>
        ) : (
          <Mono className="min-w-0 flex-1 text-xs break-all">{value}</Mono>
        )}
        <IconButton aria-label={copyLabel} size="icon-sm" onClick={() => onCopy(value)}>
          <CopyIcon className="size-3.5" />
        </IconButton>
      </div>
      <p className="text-xs text-muted-foreground">{hint}</p>
    </div>
  );
}
