"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon, XIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { ErrorState } from "@/components/ui/error-state";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { LoadingState } from "@/components/ui/loading-state";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Mono } from "@/components/ui/typography";
import { useNetworks } from "@/hooks/use-networks";
import { useCreateEventMapping, useDeleteEventMapping, useEventMappings } from "@/hooks/use-event-mappings";
import { CPA_STATUSES, CPA_STATUS_I18N_KEY, type CpaStatus } from "@/lib/api/conversions";
import type { EventMapping } from "@/lib/api/event-mappings";
import type { Network } from "@/lib/api/networks";

const STATUS_VARIANT: Record<CpaStatus, "warning" | "success" | "danger" | "secondary"> = {
  CPA_HOLD: "warning",
  CPA_ACCEPT: "success",
  CPA_REDEP: "success",
  CPA_DECLINE: "danger",
  CPA_TRASH: "secondary",
};

function NetworkMappingCard({ network, mappings, t }: { network: Network; mappings: EventMapping[]; t: TFunction }) {
  const createMapping = useCreateEventMapping();
  const deleteMapping = useDeleteEventMapping();

  const [networkStatus, setNetworkStatus] = React.useState("");
  const [floxStatus, setFloxStatus] = React.useState<CpaStatus>("CPA_HOLD");

  function handleAdd() {
    const trimmed = networkStatus.trim();
    if (!trimmed) return;
    createMapping.mutate(
      { networkId: network.id, networkStatus: trimmed, floxStatus },
      {
        onSuccess: () => {
          toast(t("eventMapping.toast.added"), {
            description: t("eventMapping.toast.addedDescription", {
              status: trimmed,
              floxStatus: t(CPA_STATUS_I18N_KEY[floxStatus], { ns: "conversions" }),
            }),
          });
          setNetworkStatus("");
        },
        onError: (err) => toast.error(t("eventMapping.toast.addError"), { description: err.message }),
      },
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">{network.name}</CardTitle>
        <CardDescription>{t("eventMapping.cardDescription")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        {mappings.map((mapping) => (
          <div key={mapping.id} className="flex items-center gap-2">
            <Mono className="w-32 shrink-0 truncate text-xs">{mapping.networkStatus}</Mono>
            <span className="text-xs text-muted-foreground">→</span>
            <Badge variant={STATUS_VARIANT[mapping.floxStatus]}>
              {t(CPA_STATUS_I18N_KEY[mapping.floxStatus], { ns: "conversions" })}
            </Badge>
            <IconButton
              aria-label={t("eventMapping.removeAria", { status: mapping.networkStatus })}
              size="icon-sm"
              className="ml-auto"
              disabled={deleteMapping.isPending}
              onClick={() =>
                deleteMapping.mutate(mapping.id, {
                  onError: (err) => toast.error(t("eventMapping.toast.removeError"), { description: err.message }),
                })
              }
            >
              <XIcon className="size-3.5" />
            </IconButton>
          </div>
        ))}
        {mappings.length === 0 && <p className="text-xs text-muted-foreground">{t("eventMapping.emptyMappings")}</p>}

        <div className="mt-1 flex items-center gap-2">
          <Input
            value={networkStatus}
            onChange={(e) => setNetworkStatus(e.target.value)}
            placeholder={t("eventMapping.statusPlaceholder")}
            className="h-8 w-32 font-mono text-xs"
          />
          <span className="text-xs text-muted-foreground">→</span>
          <Select value={floxStatus} onValueChange={(v) => setFloxStatus(v as CpaStatus)}>
            <SelectTrigger size="sm" className="w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {CPA_STATUSES.map((s) => (
                <SelectItem key={s} value={s}>
                  {t(CPA_STATUS_I18N_KEY[s], { ns: "conversions" })}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={handleAdd}
            disabled={!networkStatus.trim() || createMapping.isPending}
          >
            <PlusIcon className="size-3.5" /> {t("eventMapping.addButton")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

export function EventMappingPanel() {
  const { t } = useTranslation(["postbacks", "conversions"]);
  const networksQuery = useNetworks();
  const mappingsQuery = useEventMappings();

  if (networksQuery.isPending || mappingsQuery.isPending) {
    return <LoadingState label={t("eventMapping.loading")} />;
  }

  if (networksQuery.isError) {
    return (
      <ErrorState
        title={t("eventMapping.loadNetworksError")}
        description={networksQuery.error.message}
        onRetry={() => networksQuery.refetch()}
      />
    );
  }
  if (mappingsQuery.isError) {
    return (
      <ErrorState
        title={t("eventMapping.loadMappingsError")}
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
        <AlertDescription>{t("eventMapping.description")}</AlertDescription>
      </Alert>
      <div className="flex flex-col gap-3">
        {networks.map((network) => (
          <NetworkMappingCard
            key={network.id}
            network={network}
            mappings={mappings.filter((m) => m.networkId === network.id)}
            t={t}
          />
        ))}
      </div>
      {networks.length === 0 && <p className="text-sm text-muted-foreground">{t("eventMapping.noNetworks")}</p>}
    </div>
  );
}
