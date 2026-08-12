"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon, XIcon } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Mono } from "@/components/ui/typography";
import { useNetworksStore } from "@/stores/networks";
import { useEventMappingsStore } from "@/stores/event-mappings";
import { CPA_STATUSES, type CpaStatus } from "@/lib/mock/conversions";

const STATUS_VARIANT: Record<CpaStatus, "warning" | "success" | "danger" | "secondary"> = {
  CPA_HOLD: "warning",
  CPA_ACCEPT: "success",
  CPA_REDEP: "success",
  CPA_DECLINE: "danger",
  CPA_TRASH: "secondary",
};

function NetworkMappingCard({ networkId, networkName }: { networkId: string; networkName: string }) {
  // Select the raw array and filter locally — calling a store method that returns a
  // freshly-.filter()'d array AS the selector breaks useSyncExternalStore's snapshot-
  // stability check (new reference every read) and causes an infinite render loop.
  const allMappings = useEventMappingsStore((s) => s.mappings);
  const mappings = React.useMemo(
    () => allMappings.filter((m) => m.networkId === networkId),
    [allMappings, networkId],
  );
  const addMapping = useEventMappingsStore((s) => s.addMapping);
  const removeMapping = useEventMappingsStore((s) => s.removeMapping);

  const [networkStatus, setNetworkStatus] = React.useState("");
  const [floxStatus, setFloxStatus] = React.useState<CpaStatus>("CPA_HOLD");

  function handleAdd() {
    const trimmed = networkStatus.trim();
    if (!trimmed) return;
    addMapping({ networkId, networkStatus: trimmed, floxStatus });
    toast("Mapping added", { description: `${trimmed} → ${floxStatus}` });
    setNetworkStatus("");
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">{networkName}</CardTitle>
        <CardDescription>Raw status string this network sends → FLOX status.</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        {mappings.map((mapping) => (
          <div key={mapping.id} className="flex items-center gap-2">
            <Mono className="w-32 shrink-0 truncate text-xs">{mapping.networkStatus}</Mono>
            <span className="text-xs text-muted-foreground">→</span>
            <Badge variant={STATUS_VARIANT[mapping.floxStatus]}>{mapping.floxStatus.replace("CPA_", "")}</Badge>
            <IconButton
              aria-label={`Remove mapping ${mapping.networkStatus}`}
              size="icon-sm"
              className="ml-auto"
              onClick={() => removeMapping(mapping.id)}
            >
              <XIcon className="size-3.5" />
            </IconButton>
          </div>
        ))}
        {mappings.length === 0 && <p className="text-xs text-muted-foreground">No mappings yet — unmapped statuses fall through unrecognized.</p>}

        <div className="mt-1 flex items-center gap-2">
          <Input
            value={networkStatus}
            onChange={(e) => setNetworkStatus(e.target.value)}
            placeholder="e.g. ftd"
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
                  {s}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button type="button" variant="outline" size="sm" onClick={handleAdd} disabled={!networkStatus.trim()}>
            <PlusIcon className="size-3.5" /> Add
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

export function EventMappingPanel() {
  const networks = useNetworksStore((s) => s.networks);

  return (
    <div className="flex flex-col gap-4">
      <Alert>
        <AlertDescription>
          The Conversion Engine (Phase 23) uses this table to translate a network&apos;s own status strings into the
          canonical CPA_HOLD / CPA_ACCEPT / CPA_REDEP / CPA_DECLINE / CPA_TRASH enum (§43).
        </AlertDescription>
      </Alert>
      <div className="flex flex-col gap-3">
        {networks.map((network) => (
          <NetworkMappingCard key={network.id} networkId={network.id} networkName={network.name} />
        ))}
      </div>
    </div>
  );
}
