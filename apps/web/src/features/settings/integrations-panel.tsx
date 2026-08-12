"use client";

import { toast } from "sonner";
import { formatDistanceToNow } from "date-fns";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription, CardAction } from "@/components/ui/card";
import { Caption } from "@/components/ui/typography";
import { useSettingsStore } from "@/stores/settings";
import type { IntegrationStatus } from "@/lib/mock/settings";

const STATUS_VARIANT: Record<IntegrationStatus, "success" | "outline" | "danger"> = {
  connected: "success",
  not_connected: "outline",
  error: "danger",
};

export function IntegrationsPanel() {
  const integrations = useSettingsStore((s) => s.integrations);
  const setIntegrationStatus = useSettingsStore((s) => s.setIntegrationStatus);

  return (
    <div className="grid gap-3 sm:grid-cols-2">
      {integrations.map((integration) => {
        const connected = integration.status === "connected";
        return (
          <Card key={integration.id}>
            <CardHeader>
              <CardTitle className="text-sm">{integration.label}</CardTitle>
              <CardDescription>{integration.description}</CardDescription>
              <CardAction>
                <Badge variant={STATUS_VARIANT[integration.status]}>{integration.status.replace("_", " ")}</Badge>
              </CardAction>
            </CardHeader>
            <CardContent className="flex items-center justify-between">
              <Caption>
                {integration.connectedAt
                  ? `Connected ${formatDistanceToNow(new Date(integration.connectedAt), { addSuffix: true })}`
                  : "Not connected"}
              </Caption>
              <Button
                size="sm"
                variant={connected ? "outline" : "default"}
                onClick={() => {
                  setIntegrationStatus(integration.id, connected ? "not_connected" : "connected");
                  toast(connected ? `${integration.label} disconnected` : `${integration.label} connected`);
                }}
              >
                {connected ? "Disconnect" : "Connect"}
              </Button>
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}
