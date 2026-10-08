"use client";

import * as React from "react";
import Link from "next/link";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { ArrowRightIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription, CardAction } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Caption } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useConnectIntegration, useDisconnectIntegration, useIntegrations, useVerifyIntegration } from "@/hooks/use-integrations";
import {
  AD_PLATFORM_LABELS,
  CREDENTIAL_PROVIDERS,
  type CredentialProvider,
  type IntegrationStatus,
} from "@/lib/integrations";

const STATUS_VARIANT: Record<IntegrationStatus, "success" | "outline" | "danger"> = {
  connected: "success",
  not_connected: "outline",
  error: "danger",
};

const AD_PLATFORM_ORDER = ["facebook_ads", "tiktok_ads", "google_ads"];

export function IntegrationsPanel() {
  const { t } = useTranslation(["settings", "common"]);
  const integrationsQuery = useIntegrations();
  const labelOf = (provider: CredentialProvider | null) => CREDENTIAL_PROVIDERS.find((p) => p.provider === provider)?.label ?? "";
  const connectMutation = useConnectIntegration();
  const verifyMutation = useVerifyIntegration();
  const disconnectMutation = useDisconnectIntegration();

  const [connectTarget, setConnectTarget] = React.useState<CredentialProvider | null>(null);
  const [disconnectTarget, setDisconnectTarget] = React.useState<CredentialProvider | null>(null);
  const [fields, setFields] = React.useState<Record<string, string>>({});

  const meta = CREDENTIAL_PROVIDERS.find((p) => p.provider === connectTarget);

  function openConnect(provider: CredentialProvider) {
    setFields({});
    setConnectTarget(provider);
  }

  function submitConnect(e: React.FormEvent) {
    e.preventDefault();
    if (!connectTarget) return;
    connectMutation.mutate(
      { provider: connectTarget, fields },
      {
        onSuccess: () => {
          toast(t("integrations.connected", { name: labelOf(connectTarget) }));
          setConnectTarget(null);
        },
        onError: (err) => toast.error(t("integrations.connectError"), { description: err.message }),
      },
    );
  }

  function handleVerify(provider: CredentialProvider) {
    verifyMutation.mutate(provider, {
      onSuccess: (result) =>
        result.status === "connected"
          ? toast(t("integrations.verified"))
          : toast.error(t("integrations.verifyFailed"), { description: result.lastError ?? undefined }),
      onError: (err) => toast.error(t("integrations.verifyError"), { description: err.message }),
    });
  }

  function handleDisconnect() {
    if (!disconnectTarget) return;
    const target = disconnectTarget;
    disconnectMutation.mutate(target, {
      onSuccess: () => toast(t("integrations.disconnected", { name: labelOf(target) })),
      onError: (err) => toast.error(t("integrations.disconnectError"), { description: err.message }),
    });
    setDisconnectTarget(null);
  }

  if (integrationsQuery.isPending) {
    return <LoadingState label={t("integrations.loading")} />;
  }
  if (integrationsQuery.isError) {
    return (
      <ErrorState
        title={t("integrations.loadError")}
        description={integrationsQuery.error.message}
        onRetry={() => integrationsQuery.refetch()}
      />
    );
  }

  const byIntegration = new Map(integrationsQuery.data.integrations.map((i) => [i.provider, i]));
  const byAdPlatform = new Map(integrationsQuery.data.adPlatforms.map((p) => [p.provider, p]));

  return (
    <div className="grid gap-3 sm:grid-cols-2">
      {AD_PLATFORM_ORDER.map((provider) => {
        const platform = byAdPlatform.get(provider);
        const connectedCount = platform?.connectedCount ?? 0;
        const totalCount = platform?.totalCount ?? 0;
        return (
          <Card key={provider}>
            <CardHeader>
              <CardTitle className="text-sm">{AD_PLATFORM_LABELS[provider]}</CardTitle>
              <CardDescription>{t("integrations.adPlatformDescription")}</CardDescription>
              <CardAction>
                <Badge variant={connectedCount > 0 ? "success" : "outline"}>
                  {connectedCount > 0 ? t("integrations.status.connected") : t("integrations.status.not_connected")}
                </Badge>
              </CardAction>
            </CardHeader>
            <CardContent className="flex items-center justify-between">
              <Caption>
                {totalCount === 0
                  ? t("integrations.noSources")
                  : t("integrations.sourcesConnected", { connected: connectedCount, count: totalCount })}
              </Caption>
              <Button asChild size="sm" variant="outline">
                <Link href="/traffic-sources">
                  {t("integrations.manage")} <ArrowRightIcon className="size-3.5" />
                </Link>
              </Button>
            </CardContent>
          </Card>
        );
      })}

      {CREDENTIAL_PROVIDERS.map((p) => {
        const integration = byIntegration.get(p.provider);
        const status: IntegrationStatus = integration?.status ?? "not_connected";
        const connected = status !== "not_connected";
        return (
          <Card key={p.provider}>
            <CardHeader>
              <CardTitle className="text-sm">{p.label}</CardTitle>
              <CardDescription>{t(`integrations.providers.${p.provider}`)}</CardDescription>
              <CardAction>
                <Badge variant={STATUS_VARIANT[status]}>{t(`integrations.status.${status}`)}</Badge>
              </CardAction>
            </CardHeader>
            <CardContent className="flex flex-col gap-2">
              <div className="flex items-center justify-between">
                {integration?.lastVerifiedAt ? (
                  <Caption>
                    {t("integrations.verifiedAt")} <RelativeTime date={integration.lastVerifiedAt} className="inline" />
                  </Caption>
                ) : (
                  <Caption>{t("integrations.status.not_connected")}</Caption>
                )}
                <div className="flex gap-2">
                  {connected && (
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => handleVerify(p.provider)}
                      disabled={verifyMutation.isPending}
                    >
                      {t("integrations.verify")}
                    </Button>
                  )}
                  <Button
                    size="sm"
                    variant={connected ? "outline" : "default"}
                    onClick={() => (connected ? setDisconnectTarget(p.provider) : openConnect(p.provider))}
                  >
                    {connected ? t("integrations.disconnect") : t("integrations.connect")}
                  </Button>
                </div>
              </div>
              {status === "error" && integration?.lastError && (
                <p className="text-xs text-danger">{integration.lastError}</p>
              )}
            </CardContent>
          </Card>
        );
      })}

      <Dialog open={!!connectTarget} onOpenChange={(open) => !open && setConnectTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("integrations.connectTitle", { name: meta?.label })}</DialogTitle>
            <DialogDescription>{t("integrations.connectDescription")}</DialogDescription>
          </DialogHeader>
          <form onSubmit={submitConnect} className="flex flex-col gap-4">
            {meta?.fields.map((field) => (
              <div key={field.name} className="grid gap-1.5">
                <Label htmlFor={`integration-${field.name}`}>{t(`integrations.fields.${field.name}`)}</Label>
                <Input
                  id={`integration-${field.name}`}
                  type={field.type}
                  placeholder={field.placeholder}
                  value={fields[field.name] ?? ""}
                  onChange={(e) => setFields((f) => ({ ...f, [field.name]: e.target.value }))}
                  required
                />
              </div>
            ))}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setConnectTarget(null)}>
                {t("common:actions.cancel")}
              </Button>
              <Button type="submit" disabled={connectMutation.isPending}>
                {t("integrations.connect")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={!!disconnectTarget} onOpenChange={(open) => !open && setDisconnectTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("integrations.disconnectTitle", { name: labelOf(disconnectTarget) })}</DialogTitle>
            <DialogDescription>{t("integrations.disconnectDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDisconnectTarget(null)}>
              {t("common:actions.cancel")}
            </Button>
            <Button variant="destructive" onClick={handleDisconnect}>
              {t("integrations.disconnect")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
