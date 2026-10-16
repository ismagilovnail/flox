"use client";

import * as React from "react";
import { toast } from "sonner";
import { ClockIcon, PlusIcon, XIcon } from "lucide-react";
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
import { RelativeTime } from "@/components/ui/relative-time";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Mono } from "@/components/ui/typography";
import { useNetworks } from "@/hooks/use-networks";
import { usePermission } from "@/hooks/use-auth";
import {
  useCreateEventMapping,
  useDeleteEventMapping,
  useEventMappings,
  useWaitingStatuses,
} from "@/hooks/use-event-mappings";
import { CPA_STATUSES, CPA_STATUS_I18N_KEY, type CpaStatus } from "@/lib/api/conversions";
import type { EventMapping, WaitingStatus } from "@/lib/api/event-mappings";
import type { Network } from "@/lib/api/networks";

/** How long a postback waits for its status mapping (Go:
 * conversion.MappingReplayWindow). */
const MAPPING_REPLAY_DAYS = 30;

const STATUS_VARIANT: Record<CpaStatus, "warning" | "success" | "danger" | "secondary"> = {
  CPA_HOLD: "warning",
  CPA_ACCEPT: "success",
  CPA_REDEP: "success",
  CPA_DECLINE: "danger",
  CPA_TRASH: "secondary",
};

/** Statuses this network sent before they were mapped: the postbacks are
 * kept and recorded once the status is (audit 2026-10-15, DATA-4). "Map"
 * puts the status in the form below. */
function WaitingStatuses({
  waiting,
  canWrite,
  onMap,
  t,
}: {
  waiting: WaitingStatus[];
  canWrite: boolean;
  onMap: (rawStatus: string) => void;
  t: TFunction;
}) {
  if (waiting.length === 0) return null;
  return (
    <div className="flex flex-col gap-1.5 rounded-md border border-warning/40 bg-warning/5 p-2">
      <p className="flex items-center gap-1.5 text-xs font-medium">
        <ClockIcon aria-hidden="true" className="size-3.5 text-warning" />
        {t("eventMapping.waiting.title")}
      </p>
      {waiting.map((w) => (
        <div key={w.rawStatus} className="flex items-center gap-2">
          <Mono className="w-32 shrink-0 truncate text-xs">{w.rawStatus}</Mono>
          <span className="text-xs tabular-nums">{t("eventMapping.waiting.row", { count: w.count })}</span>
          <span aria-hidden="true" className="text-xs text-muted-foreground">·</span>
          <RelativeTime date={w.lastAt} />
          {canWrite && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="ml-auto h-7"
              aria-label={t("eventMapping.waiting.mapAria", { status: w.rawStatus })}
              onClick={() => onMap(w.rawStatus)}
            >
              {t("eventMapping.waiting.mapButton")}
            </Button>
          )}
        </div>
      ))}
      <p className="text-xs text-muted-foreground">{t("eventMapping.waiting.hint", { days: MAPPING_REPLAY_DAYS })}</p>
    </div>
  );
}

function NetworkMappingCard({
  network,
  mappings,
  waiting,
  t,
}: {
  network: Network;
  mappings: EventMapping[];
  waiting: WaitingStatus[];
  t: TFunction;
}) {
  const canWrite = usePermission("offer.write");
  const createMapping = useCreateEventMapping();
  const deleteMapping = useDeleteEventMapping();

  const [networkStatus, setNetworkStatus] = React.useState("");
  const [floxStatus, setFloxStatus] = React.useState<CpaStatus>("CPA_HOLD");
  const statusInputRef = React.useRef<HTMLInputElement>(null);

  // A status saved since the last refetch is no longer waiting.
  const mapped = new Set(mappings.map((m) => m.networkStatus.toLowerCase()));
  const stillWaiting = waiting.filter((w) => !mapped.has(w.rawStatus.toLowerCase()));

  function handleMap(rawStatus: string) {
    setNetworkStatus(rawStatus);
    statusInputRef.current?.focus();
  }

  function handleAdd() {
    const trimmed = networkStatus.trim();
    if (!trimmed) return;
    const wasWaiting = stillWaiting.some((w) => w.rawStatus.toLowerCase() === trimmed.toLowerCase());
    createMapping.mutate(
      { networkId: network.id, networkStatus: trimmed, floxStatus },
      {
        onSuccess: () => {
          toast(t("eventMapping.toast.added"), {
            description: t(wasWaiting ? "eventMapping.toast.addedWaiting" : "eventMapping.toast.addedDescription", {
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
        <WaitingStatuses waiting={stillWaiting} canWrite={canWrite} onMap={handleMap} t={t} />
        {mappings.map((mapping) => (
          <div key={mapping.id} className="flex items-center gap-2">
            <Mono className="w-32 shrink-0 truncate text-xs">{mapping.networkStatus}</Mono>
            <span className="text-xs text-muted-foreground">→</span>
            <Badge variant={STATUS_VARIANT[mapping.floxStatus]}>
              {t(CPA_STATUS_I18N_KEY[mapping.floxStatus], { ns: "conversions" })}
            </Badge>
            {canWrite && (
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
            )}
          </div>
        ))}
        {mappings.length === 0 && <p className="text-xs text-muted-foreground">{t("eventMapping.emptyMappings")}</p>}

        {canWrite && (
          <div className="mt-1 flex items-center gap-2">
            <Input
              ref={statusInputRef}
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
        )}
      </CardContent>
    </Card>
  );
}

export function EventMappingPanel() {
  const { t } = useTranslation(["postbacks", "conversions"]);
  const networksQuery = useNetworks();
  const mappingsQuery = useEventMappings();
  // Optional: the mappings work without it, so its failure is a note, not
  // the panel's error state.
  const waitingQuery = useWaitingStatuses();

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
  const waiting = waitingQuery.data?.statuses ?? [];

  return (
    <div className="flex flex-col gap-4">
      <Alert>
        <AlertDescription>{t("eventMapping.description")}</AlertDescription>
      </Alert>
      {waitingQuery.isError && <p className="text-xs text-danger">{t("eventMapping.waiting.loadError")}</p>}
      <div className="flex flex-col gap-3">
        {networks.map((network) => (
          <NetworkMappingCard
            key={network.id}
            network={network}
            mappings={mappings.filter((m) => m.networkId === network.id)}
            waiting={waiting.filter((w) => w.networkId === network.id)}
            t={t}
          />
        ))}
      </div>
      {networks.length === 0 && <p className="text-sm text-muted-foreground">{t("eventMapping.noNetworks")}</p>}
    </div>
  );
}
