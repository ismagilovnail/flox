"use client";

import { toast } from "sonner";
import { CopyIcon } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { IconButton } from "@/components/ui/icon-button";
import { Mono } from "@/components/ui/typography";
import { useNetworksStore } from "@/stores/networks";
import { useEventMappingsStore } from "@/stores/event-mappings";

function incomingUrl(networkId: string) {
  return `https://api.floxlink.io/postback/${networkId}?click_id={click_id}&status={status}&revenue={revenue}&currency={currency}`;
}

export function IncomingPostbacksPanel() {
  const networks = useNetworksStore((s) => s.networks);
  const mappings = useEventMappingsStore((s) => s.mappings);

  function copy(url: string) {
    navigator.clipboard.writeText(url);
    toast("Incoming postback URL copied", { description: url });
  }

  return (
    <div className="flex flex-col gap-4">
      <Alert>
        <AlertDescription>
          Give each network this URL so they can report conversions into FLOX. Dedup applies on (click_id, status)
          — §45 — unless the network has &ldquo;Accept duplicate postbacks&rdquo; on. Raw status strings are
          translated to FLOX statuses via Event Mapping.
        </AlertDescription>
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
                    {mappedCount} status{mappedCount === 1 ? "" : "es"} mapped
                  </Badge>
                </CardDescription>
              </CardHeader>
              <CardContent className="flex items-center gap-2">
                <Mono className="min-w-0 flex-1 truncate text-xs">{url}</Mono>
                <IconButton aria-label={`Copy incoming URL for ${network.name}`} size="icon-sm" onClick={() => copy(url)}>
                  <CopyIcon className="size-3.5" />
                </IconButton>
              </CardContent>
            </Card>
          );
        })}
      </div>
    </div>
  );
}
