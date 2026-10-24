"use client";

import * as React from "react";
import { CopyIcon, Unlink2Icon } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Mono } from "@/components/ui/typography";
import { trackerOrigin } from "@/features/networks/browser-conversions-card";
import { useDeleteShopify, useShopify, useUpdateShopify } from "@/hooks/use-networks";
import type { Network } from "@/lib/api/networks";
import { ApiError } from "@/lib/api/client";

type Field = "shopDomain" | "secret";

/** The theme tag a Shopify store adds: it puts the click on the cart, the
 * order keeps it, the order webhook brings it back. */
export function shopifySnippet(origin: string): string {
  return `<script src="${origin}/flox.js" async></script>`;
}

/** An own site that is a Shopify store reports its orders by webhook (audit
 * UNI-4 part 3, docs/shop-connections.md): paid orders become purchases or
 * repeat purchases, refunds take back what was paid back. The store signs
 * each webhook; FLOX keeps the key encrypted and shows only its end. */
export function ShopifyCard({ network, canWrite }: { network: Network; canWrite: boolean }) {
  const { t } = useTranslation("networks");
  const query = useShopify(network.id);
  const update = useUpdateShopify(network.id);
  const remove = useDeleteShopify(network.id);
  const [domain, setDomain] = React.useState<string | null>(null);
  const [secret, setSecret] = React.useState("");
  const [errors, setErrors] = React.useState<Partial<Record<Field, string>>>({});
  const [confirming, setConfirming] = React.useState(false);
  const origin = trackerOrigin(network.incomingPostbackUrl);

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
          toast(t("shopify.saved"));
        },
        onError: (err) => {
          if (err instanceof ApiError && err.fields) setErrors(err.fields as Partial<Record<Field, string>>);
          toast.error(t("shopify.saveError"), { description: err.message });
        },
      },
    );
  }

  function disconnect() {
    remove.mutate(undefined, {
      onSuccess: () => {
        setConfirming(false);
        setDomain(null);
        toast(t("shopify.disconnected"));
      },
      onError: (err) => toast.error(t("shopify.disconnectError"), { description: err.message }),
    });
  }

  function copy(value: string) {
    navigator.clipboard.writeText(value).then(
      () => toast(t("browser.copied")),
      () => toast.error(t("incoming.copyFailed")),
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("shopify.title")}</CardTitle>
        <CardDescription>{t("shopify.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {query.isLoading && <p className="text-xs text-muted-foreground">{t("shopify.loading")}</p>}
        {query.isError && (
          <p className="text-xs text-danger">
            {t("shopify.loadError")} {query.error.message}
          </p>
        )}
        {data && !data.available && <p className="text-xs text-warning">{t("shopify.unavailable")}</p>}
        {data && data.available && (
          <>
            <fieldset disabled={!canWrite || update.isPending} className="contents">
              <div className="grid gap-2 sm:grid-cols-2">
                <div className="grid gap-1.5">
                  <Label htmlFor="shopify-domain">{t("shopify.domainLabel")}</Label>
                  <Input
                    id="shopify-domain"
                    className="font-mono text-xs"
                    placeholder="my-shop.myshopify.com"
                    value={shopDomain}
                    onChange={(e) => setDomain(e.target.value)}
                    aria-invalid={!!errors.shopDomain}
                  />
                  {errors.shopDomain && <p className="text-xs text-danger">{errors.shopDomain}</p>}
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="shopify-secret">{t("shopify.secretLabel")}</Label>
                  <Input
                    id="shopify-secret"
                    type="password"
                    autoComplete="off"
                    className="font-mono text-xs"
                    placeholder={data.connected ? t("shopify.secretKeep", { last4: data.secretPreview }) : ""}
                    value={secret}
                    onChange={(e) => setSecret(e.target.value)}
                    aria-invalid={!!errors.secret}
                  />
                  {errors.secret && <p className="text-xs text-danger">{errors.secret}</p>}
                </div>
              </div>
              <p className="text-xs text-muted-foreground">{t("shopify.secretHint")}</p>
              {canWrite && (
                <div className="flex flex-wrap gap-2">
                  <Button type="button" size="sm" onClick={save} disabled={update.isPending || (!data.connected && !secret.trim())}>
                    {data.connected ? t("shopify.save") : t("shopify.connect")}
                  </Button>
                  {data.connected &&
                    (confirming ? (
                      <>
                        <Button type="button" size="sm" variant="outline" onClick={() => setConfirming(false)}>
                          {t("shopify.cancel")}
                        </Button>
                        <Button type="button" size="sm" variant="destructive" onClick={disconnect} disabled={remove.isPending}>
                          {t("shopify.confirmDisconnect")}
                        </Button>
                      </>
                    ) : (
                      <Button type="button" size="sm" variant="outline" onClick={() => setConfirming(true)}>
                        <Unlink2Icon className="size-3.5" /> {t("shopify.disconnect")}
                      </Button>
                    ))}
                </div>
              )}
            </fieldset>

            {data.webhookUrl && (
              <div className="grid gap-1.5">
                <p className="text-xs font-medium">{t("shopify.webhookUrl")}</p>
                <div className="flex items-start gap-2 rounded-md border border-border bg-muted px-3 py-2">
                  <Mono className="min-w-0 flex-1 text-xs break-all">{data.webhookUrl}</Mono>
                  <IconButton aria-label={t("shopify.copyWebhook")} size="icon-sm" onClick={() => copy(data.webhookUrl)}>
                    <CopyIcon className="size-3.5" />
                  </IconButton>
                </div>
                <p className="text-xs text-muted-foreground">{t("shopify.webhookHint")}</p>
              </div>
            )}
            {origin && (
              <div className="grid gap-1.5">
                <p className="text-xs font-medium">{t("shopify.theme")}</p>
                <div className="flex items-start gap-2 rounded-md border border-border bg-muted px-3 py-2">
                  <Mono className="min-w-0 flex-1 text-xs break-all">{shopifySnippet(origin)}</Mono>
                  <IconButton aria-label={t("shopify.copyTheme")} size="icon-sm" onClick={() => copy(shopifySnippet(origin))}>
                    <CopyIcon className="size-3.5" />
                  </IconButton>
                </div>
                <p className="text-xs text-muted-foreground">{t("shopify.themeHint")}</p>
              </div>
            )}
            <p className="text-xs text-warning">{t("shopify.limits")}</p>
          </>
        )}
      </CardContent>
    </Card>
  );
}
