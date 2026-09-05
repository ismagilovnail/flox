"use client";

import * as React from "react";
import Link from "next/link";
import { toast } from "sonner";
import { formatDistanceToNow } from "date-fns";
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
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useConnectIntegration, useDisconnectIntegration, useIntegrations, useVerifyIntegration } from "@/hooks/use-integrations";
import {
  AD_PLATFORM_DESCRIPTIONS,
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
  const integrationsQuery = useIntegrations();
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
          toast(`${CREDENTIAL_PROVIDERS.find((p) => p.provider === connectTarget)?.label} connected`);
          setConnectTarget(null);
        },
        onError: (err) => toast.error("Couldn't connect", { description: err.message }),
      },
    );
  }

  function handleVerify(provider: CredentialProvider) {
    verifyMutation.mutate(provider, {
      onSuccess: (result) =>
        result.status === "connected"
          ? toast("Connection verified")
          : toast.error("Verification failed", { description: result.lastError ?? undefined }),
      onError: (err) => toast.error("Couldn't verify connection", { description: err.message }),
    });
  }

  function handleDisconnect() {
    if (!disconnectTarget) return;
    const target = disconnectTarget;
    disconnectMutation.mutate(target, {
      onSuccess: () => toast(`${CREDENTIAL_PROVIDERS.find((p) => p.provider === target)?.label} disconnected`),
      onError: (err) => toast.error("Couldn't disconnect", { description: err.message }),
    });
    setDisconnectTarget(null);
  }

  if (integrationsQuery.isPending) {
    return <LoadingState label="Loading integrations…" />;
  }
  if (integrationsQuery.isError) {
    return (
      <ErrorState
        title="Couldn't load integrations"
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
              <CardDescription>{AD_PLATFORM_DESCRIPTIONS[provider]}</CardDescription>
              <CardAction>
                <Badge variant={connectedCount > 0 ? "success" : "outline"}>
                  {connectedCount > 0 ? "connected" : "not connected"}
                </Badge>
              </CardAction>
            </CardHeader>
            <CardContent className="flex items-center justify-between">
              <Caption>
                {totalCount === 0
                  ? "Not configured on any traffic source yet"
                  : `Connected on ${connectedCount} of ${totalCount} traffic source${totalCount === 1 ? "" : "s"}`}
              </Caption>
              <Button asChild size="sm" variant="outline">
                <Link href="/traffic-sources">
                  Manage <ArrowRightIcon className="size-3.5" />
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
              <CardDescription>{p.description}</CardDescription>
              <CardAction>
                <Badge variant={STATUS_VARIANT[status]}>{status.replace("_", " ")}</Badge>
              </CardAction>
            </CardHeader>
            <CardContent className="flex flex-col gap-2">
              <div className="flex items-center justify-between">
                <Caption>
                  {integration?.lastVerifiedAt
                    ? `Verified ${formatDistanceToNow(new Date(integration.lastVerifiedAt), { addSuffix: true })}`
                    : "Not connected"}
                </Caption>
                <div className="flex gap-2">
                  {connected && (
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => handleVerify(p.provider)}
                      disabled={verifyMutation.isPending}
                    >
                      Verify
                    </Button>
                  )}
                  <Button
                    size="sm"
                    variant={connected ? "outline" : "default"}
                    onClick={() => (connected ? setDisconnectTarget(p.provider) : openConnect(p.provider))}
                  >
                    {connected ? "Disconnect" : "Connect"}
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
            <DialogTitle>Connect {meta?.label}</DialogTitle>
            <DialogDescription>Credentials are encrypted at rest and never shown again after saving.</DialogDescription>
          </DialogHeader>
          <form onSubmit={submitConnect} className="flex flex-col gap-4">
            {meta?.fields.map((field) => (
              <div key={field.name} className="grid gap-1.5">
                <Label htmlFor={`integration-${field.name}`}>{field.label}</Label>
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
                Cancel
              </Button>
              <Button type="submit" disabled={connectMutation.isPending}>
                Connect
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={!!disconnectTarget} onOpenChange={(open) => !open && setDisconnectTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              Disconnect {CREDENTIAL_PROVIDERS.find((p) => p.provider === disconnectTarget)?.label}?
            </DialogTitle>
            <DialogDescription>The stored credential is deleted. You can reconnect at any time.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDisconnectTarget(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDisconnect}>
              Disconnect
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
