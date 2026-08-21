"use client";

import * as React from "react";
import { toast } from "sonner";

import { EmptyState } from "@/components/ui/empty-state";
import { emptySimulateRequest, simulateRoute, type SimulateResult } from "@/lib/api/routing";
import type { FilterField } from "@/lib/filters";
import { SimulatorForm } from "@/features/routing-simulator/simulator-form";
import { SimulatorResult } from "@/features/routing-simulator/simulator-result";

export function RoutingSimulatorView({
  campaignId,
  campaignName,
}: {
  campaignId: string;
  campaignName: string;
}) {
  const [request, setRequest] = React.useState(emptySimulateRequest);
  const [result, setResult] = React.useState<SimulateResult | null>(null);
  const [isSimulating, setIsSimulating] = React.useState(false);

  function handleFieldChange(field: FilterField, value: string) {
    setRequest((prev) => ({ ...prev, [field]: value }));
  }

  async function handleSimulate() {
    setIsSimulating(true);
    try {
      setResult(await simulateRoute(campaignId, request));
    } catch (err) {
      toast.error("Couldn't simulate this request", { description: err instanceof Error ? err.message : undefined });
    } finally {
      setIsSimulating(false);
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h2 className="text-sm font-medium">Simulate incoming traffic</h2>
        <p className="text-xs text-muted-foreground">
          Fill in the request attributes a click could carry and see exactly which Stream Set would match, which
          Flow would be picked, and why — evaluated against this campaign&apos;s current configuration, top-to-bottom
          by priority.
        </p>
      </div>

      <SimulatorForm request={request} onChange={handleFieldChange} onSimulate={handleSimulate} isSimulating={isSimulating} />

      {result ? (
        <SimulatorResult result={result} campaignName={campaignName} />
      ) : (
        <EmptyState
          title="No simulation yet"
          description="Set request attributes above and click Simulate to see the routing decision."
        />
      )}
    </div>
  );
}
