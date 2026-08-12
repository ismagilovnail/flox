"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { useCustomMetricsStore } from "@/stores/custom-metrics";
import { METRIC_CATALOG, surfaceCanCompute } from "@/lib/mock/custom-metrics-registry";
import { validateFormula } from "@/lib/formula-engine";
import { useCurrentMember } from "@/hooks/use-current-member";
import { customMetricColumns } from "@/features/custom-metrics/custom-metric-columns";
import {
  CustomMetricFormSheet,
  type CustomMetricFormValues,
} from "@/features/custom-metrics/custom-metric-form-sheet";
import type { CustomMetric } from "@/lib/mock/custom-metrics";

function toFormValues(metric: CustomMetric): CustomMetricFormValues {
  return {
    name: metric.name,
    group: metric.group,
    format: metric.format,
    formula: metric.formula,
    targets: metric.targets,
    status: metric.status,
  };
}

export function CustomMetricsList() {
  const metrics = useCustomMetricsStore((s) => s.metrics);
  const addMetric = useCustomMetricsStore((s) => s.addMetric);
  const updateMetric = useCustomMetricsStore((s) => s.updateMetric);
  const { member: currentMember, memberId: CURRENT_USER_MEMBER_ID, isOwnerOrAdmin: canManageAny } = useCurrentMember();

  const [target, setTarget] = React.useState<CustomMetric | null | undefined>(undefined);

  const role = currentMember?.role;
  const canCreate = canManageAny || role === "Manager";
  const canManage = React.useCallback(
    (metric: CustomMetric) => canManageAny || (role === "Manager" && metric.createdByMemberId === CURRENT_USER_MEMBER_ID),
    [canManageAny, role, CURRENT_USER_MEMBER_ID],
  );

  const visibleMetrics = canCreate ? metrics : metrics.filter((m) => m.status === "published" && m.active);

  const existingGroups = React.useMemo(
    () => [...new Set(metrics.map((m) => m.group))].sort((a, b) => a.localeCompare(b)),
    [metrics],
  );

  function handleSubmit(values: CustomMetricFormValues) {
    const validation = validateFormula(values.formula, METRIC_CATALOG);
    const targets = validation.valid
      ? values.targets.filter((t) => surfaceCanCompute(t, validation.usedMetricIds ?? []))
      : [];
    const input = { ...values, targets };

    if (target) {
      updateMetric(target.id, input);
      toast("Metric updated", { description: values.name });
    } else {
      addMetric(input, CURRENT_USER_MEMBER_ID);
      toast("Metric created", { description: values.name });
    }
    setTarget(undefined);
  }

  const columns = React.useMemo(() => customMetricColumns((metric) => setTarget(metric), canManage), [canManage]);

  return (
    <div className="flex flex-col gap-4">
      <Alert>
        <AlertDescription>
          Formulas reference registry metrics by stable id (§30.5, §50) — never raw columns. Owner/Admin manage any
          metric; Manager can create and manage their own; Buyer/Analyst/Viewer only see published, active metrics.
        </AlertDescription>
      </Alert>

      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">{visibleMetrics.length} metrics</p>
        {canCreate && (
          <Button onClick={() => setTarget(null)}>
            <PlusIcon className="size-4" />
            New Custom Metric
          </Button>
        )}
      </div>

      <DataTable
        columns={columns}
        data={visibleMetrics}
        searchPlaceholder="Search metrics..."
        emptyTitle="No custom metrics yet"
        emptyDescription="Build a formula on top of the metrics registry — safe division included."
        pageSize={10}
      />

      {target !== undefined && (
        <CustomMetricFormSheet
          key={target?.id ?? "new"}
          open
          onOpenChange={(open) => !open && setTarget(undefined)}
          title={target ? `Edit ${target.name}` : "New Custom Metric"}
          submitLabel={target ? "Save changes" : "Create metric"}
          defaultValues={target ? toFormValues(target) : {}}
          existingGroups={existingGroups}
          onSubmit={handleSubmit}
        />
      )}
    </div>
  );
}
