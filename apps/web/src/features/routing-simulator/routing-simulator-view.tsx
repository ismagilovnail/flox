"use client";

import * as React from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation("routingSimulator");
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
      toast.error(t("view.simulateError"), { description: err instanceof Error ? err.message : undefined });
    } finally {
      setIsSimulating(false);
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h2 className="text-sm font-medium">{t("view.title")}</h2>
        <p className="text-xs text-muted-foreground">{t("view.description")}</p>
      </div>

      <SimulatorForm request={request} onChange={handleFieldChange} onSimulate={handleSimulate} isSimulating={isSimulating} />

      {result ? (
        <SimulatorResult result={result} campaignName={campaignName} />
      ) : (
        <EmptyState title={t("view.emptyTitle")} description={t("view.emptyDescription")} />
      )}
    </div>
  );
}
