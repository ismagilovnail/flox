"use client";

import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useTrafficSource, useUpdateTrafficSource } from "@/hooks/use-traffic-sources";
import { SourceForm, type SourceFormValues } from "@/features/traffic-sources/source-form";

export function SourceDetailView({ id }: { id: string }) {
  const { t } = useTranslation("trafficSources");
  const sourceQuery = useTrafficSource(id);
  const updateSource = useUpdateTrafficSource(id);

  if (sourceQuery.isPending) {
    return <LoadingState label={t("list.loading")} />;
  }

  if (sourceQuery.isError) {
    return (
      <ErrorState
        title={t("list.loadError")}
        description={sourceQuery.error.message}
        onRetry={() => sourceQuery.refetch()}
      />
    );
  }

  const source = sourceQuery.data;

  function handleSubmit(values: SourceFormValues) {
    updateSource.mutate(
      {
        name: values.name,
        type: values.type,
        trackingTemplate: values.trackingTemplate,
        costIntegration: values.costIntegration,
        status: values.status,
      },
      {
        onSuccess: () => toast(t("toast.updated"), { description: values.name }),
        onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
      },
    );
  }

  return (
    <SourceForm
      title={t("form.titleEdit", { name: source.name })}
      defaultValues={{
        name: source.name,
        type: source.type as SourceFormValues["type"],
        trackingTemplate: source.trackingTemplate,
        costIntegration: source.costIntegration,
        status: source.status,
      }}
      submitLabel={t("form.submitEdit")}
      showStatus
      sourceId={source.id}
      onSubmit={handleSubmit}
    />
  );
}
