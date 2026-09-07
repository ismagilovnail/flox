"use client";

import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { usePermission } from "@/hooks/use-auth";
import { useLanding, useUpdateLanding } from "@/hooks/use-landings";
import { LandingForm, type LandingFormValues } from "@/features/landings/landing-form";
import type { Landing } from "@/lib/api/landings";

function toFormValues(landing: Landing): LandingFormValues {
  return {
    name: landing.name,
    type: landing.type,
    url: landing.url,
    content: landing.content,
    status: landing.status,
  };
}

export function LandingDetailView({ id }: { id: string }) {
  const { t } = useTranslation("landings");
  const canWrite = usePermission("content.write");
  const landingQuery = useLanding(id);
  const updateLanding = useUpdateLanding(id);

  if (landingQuery.isPending) {
    return <LoadingState label={t("list.loading")} />;
  }

  if (landingQuery.isError) {
    return (
      <ErrorState
        title={t("list.loadError")}
        description={landingQuery.error.message}
        onRetry={() => landingQuery.refetch()}
      />
    );
  }

  const landing = landingQuery.data;

  function handleSubmit(values: LandingFormValues) {
    const input = {
      name: values.name,
      type: values.type,
      status: values.status,
      url: values.type === "external" ? (values.url ?? "") : undefined,
      content: values.type === "internal" ? (values.content ?? "") : "",
    };
    updateLanding.mutate(input, {
      onSuccess: () => toast(t("toast.updated"), { description: values.name }),
      onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
    });
  }

  return (
    <LandingForm
      title={t("form.titleEdit", { name: landing.name })}
      defaultValues={toFormValues(landing)}
      submitLabel={t("form.submitEdit")}
      onSubmit={handleSubmit}
      disabled={!canWrite}
    />
  );
}
