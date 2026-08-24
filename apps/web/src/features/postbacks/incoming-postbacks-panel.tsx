"use client";

import { toast } from "sonner";
import { CopyIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { ErrorState } from "@/components/ui/error-state";
import { IconButton } from "@/components/ui/icon-button";
import { LoadingState } from "@/components/ui/loading-state";
import { Mono } from "@/components/ui/typography";
import { useNetworks } from "@/hooks/use-networks";
import { useEventMappings } from "@/hooks/use-event-mappings";

function incomingUrl(networkId: string) {
  return `https://api.floxlink.io/postback/${networkId}?click_id={click_id}&status={status}&revenue={revenue}&currency={currency}`;
}

export function IncomingPostbacksPanel() {
  const { t } = useTranslation("postbacks");
  const networksQuery = useNetworks();
  const mappingsQuery = useEventMappings();

  function copy(url: string) {
    navigator.clipboard.writeText(url);
    toast(t("incoming.copyToastTitle"), { description: url });
  }

  if (networksQuery.isPending || mappingsQuery.isPending) {
    return <LoadingState label={t("incoming.loading")} />;
  }

  if (networksQuery.isError) {
    return (
      <ErrorState
        title={t("incoming.loadNetworksError")}
        description={networksQuery.error.message}
        onRetry={() => networksQuery.refetch()}
      />
    );
  }
  if (mappingsQuery.isError) {
    return (
      <ErrorState
        title={t("incoming.loadMappingsError")}
        description={mappingsQuery.error.message}
        onRetry={() => mappingsQuery.refetch()}
      />
    );
  }

  const networks = networksQuery.data.networks;
  const mappings = mappingsQuery.data.eventMappings;

  return (
    <div className="flex flex-col gap-4">
      <Alert>
        <AlertDescription>{t("incoming.description")}</AlertDescription>
      </Alert>

      <div className="flex flex-col gap-3">
        {networks.map((network) => {
          const mappedCount = mappings.filter((m) => m.networkId === network.id).length;
          const url = incomingUrl(network.id);
          return (
            <Card key={network.id}>
              <CardHeader>
                <CardTitle className="text-sm">{network.name}</CardTitle>
                <CardDescription>
                  <Badge variant={mappedCount > 0 ? "outline" : "warning"}>
                    {t("incoming.mappedCount", { count: mappedCount })}
                  </Badge>
                </CardDescription>
              </CardHeader>
              <CardContent className="flex items-center gap-2">
                <Mono className="min-w-0 flex-1 truncate text-xs">{url}</Mono>
                <IconButton
                  aria-label={t("incoming.copyAria", { name: network.name })}
                  size="icon-sm"
                  onClick={() => copy(url)}
                >
                  <CopyIcon className="size-3.5" />
                </IconButton>
              </CardContent>
            </Card>
          );
        })}
      </div>
      {networks.length === 0 && <p className="text-sm text-muted-foreground">{t("incoming.noNetworks")}</p>}
    </div>
  );
}
