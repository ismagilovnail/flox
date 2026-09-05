"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { useMe } from "@/hooks/use-auth";
import { EMPTY_CUSTOM_METRICS, useCreateCustomMetric, useCustomMetrics, useUpdateCustomMetric } from "@/hooks/use-custom-metrics";
import { customMetricColumns } from "@/features/custom-metrics/custom-metric-columns";
import {
  CustomMetricFormSheet,
  type CustomMetricFormValues,
} from "@/features/custom-metrics/custom-metric-form-sheet";
import type { CustomMetric } from "@/lib/custom-metrics";

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
  const metrics = useCustomMetrics().data?.metrics ?? EMPTY_CUSTOM_METRICS;
  const createMetric = useCreateCustomMetric();
  const updateMetric = useUpdateCustomMetric();
  const me = useMe();

  const [target, setTarget] = React.useState<CustomMetric | null | undefined>(undefined);

  const role = me.data?.role;
  const userId = me.data?.user.id;
  // Owner/Admin manage any metric; Manager creates and manages only its
  // own; Buyer/Analyst/Viewer never see this button at all — same tiers
  // apps/internal/custommetrics.Service actually enforces server-side,
  // mirrored here only to shape the UI (§52: "frontend permissions are
  // only UX").
  const canManageAny = role === "Owner" || role === "Admin";
  const canCreate = canManageAny || role === "Manager";
  const canManage = React.useCallback(
    (metric: CustomMetric) => canManageAny || (role === "Manager" && metric.createdByUserId === userId),
    [canManageAny, role, userId],
  );

  // The server already scopes List() to this exact tier (full list for
  // Owner/Admin/Manager, published+active only otherwise) — no client-
  // side re-filter needed, unlike the old mock store which had no
  // backend to enforce this.
  const visibleMetrics = metrics;

  const existingGroups = React.useMemo(
    () => [...new Set(metrics.map((m) => m.group))].sort((a, b) => a.localeCompare(b)),
    [metrics],
  );

  function handleSubmit(values: CustomMetricFormValues) {
    if (target) {
      updateMetric.mutate(
        { id: target.id, input: values },
        {
          onSuccess: () => toast("Metric updated", { description: values.name }),
          onError: (err) => toast.error("Couldn't update metric", { description: err.message }),
        },
      );
    } else {
      createMetric.mutate(values, {
        onSuccess: () => toast("Metric created", { description: values.name }),
        onError: (err) => toast.error("Couldn't create metric", { description: err.message }),
      });
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
