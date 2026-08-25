"use client";

import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { LinkIcon, RefreshCwIcon, Unlink2Icon } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Mono } from "@/components/ui/typography";
import {
  useAdAccountConnection,
  useConnectAdAccount,
  useDisconnectAdAccount,
  useSyncAdAccount,
} from "@/hooks/use-ad-account-connection";
import type { CostIntegration } from "@/lib/api/traffic-sources";

function buildConnectSchema(t: TFunction) {
  return z.object({
    adAccountId: z.string().min(1, t("connection.validation.adAccountIdRequired", { ns: "trafficSources" })).max(100),
    accessToken: z.string().min(1, t("connection.validation.accessTokenRequired", { ns: "trafficSources" })).max(500),
  });
}

type ConnectFormValues = z.infer<ReturnType<typeof buildConnectSchema>>;

/** Shown inside SourceFormSheet, in edit mode only, whenever the live
 * (not-yet-saved) costIntegration value is facebook_ads/tiktok_ads —
 * connecting/disconnecting is its own action against its own endpoint
 * (/traffic-sources/{id}/connection), independent of the surrounding
 * form's own "Save changes" submit. No real Facebook/TikTok API call
 * happens anywhere yet (confirmed via AskUserQuestion before this phase):
 * this only stores the credential an eventual sync phase will use. */
export function AdAccountConnectionSection({
  trafficSourceId,
  provider,
}: {
  trafficSourceId: string;
  provider: Extract<CostIntegration, "facebook_ads" | "tiktok_ads">;
}) {
  const { t } = useTranslation(["trafficSources", "common"]);
  const connectionQuery = useAdAccountConnection(trafficSourceId);
  const connect = useConnectAdAccount(trafficSourceId);
  const disconnect = useDisconnectAdAccount(trafficSourceId);
  const sync = useSyncAdAccount(trafficSourceId);

  const form = useForm<ConnectFormValues>({
    resolver: zodResolver(buildConnectSchema(t)),
    defaultValues: { adAccountId: "", accessToken: "" },
  });

  function submit(values: ConnectFormValues) {
    connect.mutate(values, {
      onSuccess: () => {
        toast(t("connection.toast.connected", { ns: "trafficSources" }));
        form.reset();
      },
      onError: (err) => toast.error(t("connection.toast.connectError", { ns: "trafficSources" }), { description: err.message }),
    });
  }

  function handleDisconnect() {
    disconnect.mutate(undefined, {
      onSuccess: () => toast(t("connection.toast.disconnected", { ns: "trafficSources" })),
      onError: (err) => toast.error(t("connection.toast.disconnectError", { ns: "trafficSources" }), { description: err.message }),
    });
  }

  function handleSync() {
    sync.mutate(undefined, {
      onSuccess: (result) =>
        toast(t("connection.toast.synced", { ns: "trafficSources" }), {
          description: t("connection.syncResult.entriesWritten", { ns: "trafficSources" }) + `: ${result.entriesWritten}`,
        }),
      onError: (err) => toast.error(t("connection.toast.syncError", { ns: "trafficSources" }), { description: err.message }),
    });
  }

  return (
    <div className="flex flex-col gap-2 rounded-lg border border-border p-3">
      <div className="flex items-center gap-2">
        <span className="text-sm font-medium">{t("connection.title", { ns: "trafficSources" })}</span>
        <Badge variant="outline">{t(`costIntegration.${provider === "facebook_ads" ? "facebookAds" : "tiktokAds"}`, { ns: "trafficSources" })}</Badge>
      </div>
      <p className="text-xs text-muted-foreground">{t("connection.description", { ns: "trafficSources" })}</p>

      {connectionQuery.isLoading ? (
        <p className="text-xs text-muted-foreground">{t("connection.loading", { ns: "trafficSources" })}</p>
      ) : connectionQuery.data ? (
        <div className="flex flex-col gap-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <LinkIcon className="size-3.5 text-success" />
              <Mono className="text-xs">{connectionQuery.data.adAccountId}</Mono>
              <span className="text-xs text-muted-foreground">
                {t("connection.tokenPreview", { ns: "trafficSources", last4: connectionQuery.data.tokenPreview })}
              </span>
            </div>
            <div className="flex items-center gap-2">
              <Button type="button" variant="outline" size="sm" onClick={handleSync} disabled={sync.isPending}>
                <RefreshCwIcon className={`size-3.5 ${sync.isPending ? "animate-spin" : ""}`} />
                {sync.isPending
                  ? t("connection.syncing", { ns: "trafficSources" })
                  : t("connection.syncNow", { ns: "trafficSources" })}
              </Button>
              <Button type="button" variant="outline" size="sm" onClick={handleDisconnect} disabled={disconnect.isPending}>
                <Unlink2Icon className="size-3.5" /> {t("connection.disconnect", { ns: "trafficSources" })}
              </Button>
            </div>
          </div>

          {sync.data && (
            <div className="flex flex-col gap-1.5 rounded-md border border-border bg-muted/30 p-2.5">
              <span className="text-xs font-medium">{t("connection.syncResult.title", { ns: "trafficSources" })}</span>
              <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                <span>
                  {t("connection.syncResult.recordsFetched", { ns: "trafficSources" })}: {sync.data.recordsFetched}
                </span>
                <span>
                  {t("connection.syncResult.entriesWritten", { ns: "trafficSources" })}: {sync.data.entriesWritten}
                </span>
              </div>
              {sync.data.unmatchedExternalCampaignIds && sync.data.unmatchedExternalCampaignIds.length > 0 && (
                <div className="flex flex-col gap-1">
                  <span className="text-xs text-muted-foreground">
                    {t("connection.syncResult.unmatchedTitle", { ns: "trafficSources" })}
                  </span>
                  <div className="flex flex-wrap gap-1">
                    {sync.data.unmatchedExternalCampaignIds.map((id) => (
                      <Badge key={id} variant="outline" className="font-mono text-[11px]">
                        {id}
                      </Badge>
                    ))}
                    {sync.data.unmatchedExternalCampaignIdsTruncated && (
                      <span className="text-xs text-muted-foreground">
                        {t("connection.syncResult.unmatchedTruncated", { ns: "trafficSources" })}
                      </span>
                    )}
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {t("connection.syncResult.unmatchedHint", { ns: "trafficSources" })}
                  </p>
                </div>
              )}
            </div>
          )}
        </div>
      ) : (
        // A <form> here (rather than this <div>) would nest inside
        // SourceFormSheet's own outer <form> — HTML forbids nesting
        // forms, and Chrome's actual behavior for it isn't "the inner
        // form is ignored": the inner submit button's default GET
        // request fires against the outer form instead, sending every
        // field — including the access token — as a URL query string.
        // Reproduced live during this phase's manual verification.
        // handleSubmit is invoked directly from the button's onClick
        // instead, with no <form>/onSubmit anywhere in this subtree, so
        // there's no submit event for either form to react to.
        <div
          className="flex flex-col gap-2 sm:flex-row sm:items-end"
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              void form.handleSubmit(submit)();
            }
          }}
        >
          <div className="grid flex-1 gap-1.5">
            <Label htmlFor="conn-account">{t("connection.adAccountIdLabel", { ns: "trafficSources" })}</Label>
            <Input
              id="conn-account"
              placeholder={t("connection.adAccountIdPlaceholder", { ns: "trafficSources" })}
              className="font-mono text-xs"
              {...form.register("adAccountId")}
              aria-invalid={!!form.formState.errors.adAccountId}
            />
            {form.formState.errors.adAccountId && (
              <p className="text-xs text-danger">{form.formState.errors.adAccountId.message}</p>
            )}
          </div>
          <div className="grid flex-1 gap-1.5">
            <Label htmlFor="conn-token">{t("connection.accessTokenLabel", { ns: "trafficSources" })}</Label>
            <Input
              id="conn-token"
              type="password"
              placeholder={t("connection.accessTokenPlaceholder", { ns: "trafficSources" })}
              className="font-mono text-xs"
              {...form.register("accessToken")}
              aria-invalid={!!form.formState.errors.accessToken}
            />
            {form.formState.errors.accessToken && (
              <p className="text-xs text-danger">{form.formState.errors.accessToken.message}</p>
            )}
          </div>
          <Button type="button" size="sm" disabled={connect.isPending} onClick={form.handleSubmit(submit)}>
            <LinkIcon className="size-3.5" /> {t("connection.connect", { ns: "trafficSources" })}
          </Button>
        </div>
      )}
    </div>
  );
}
