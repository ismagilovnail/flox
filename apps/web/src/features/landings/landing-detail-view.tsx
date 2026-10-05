"use client";

import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { usePermission } from "@/hooks/use-auth";
import { useLanding, useReplaceLandingArchive, useUpdateLanding } from "@/hooks/use-landings";
import { LandingForm, type LandingFormValues } from "@/features/landings/landing-form";
import { toastUploadWarnings } from "@/features/landings/upload-warnings";
import type { Landing } from "@/lib/api/landings";

function toFormValues(landing: Landing): LandingFormValues {
  return {
    name: landing.name,
    title: landing.title,
    logoUrl: landing.logoUrl,
    type: landing.type,
    url: landing.type === "external" ? landing.url : "",
    archive: null,
    status: landing.status,
  };
}

export function LandingDetailView({ id }: { id: string }) {
  const { t } = useTranslation("landings");
  const canWrite = usePermission("content.write");
  const landingQuery = useLanding(id);
  const updateLanding = useUpdateLanding(id);
  const replaceArchive = useReplaceLandingArchive(id);

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

  async function handleSubmit(values: LandingFormValues) {
    try {
      await updateLanding.mutateAsync({
        name: values.name,
        title: values.title,
        logoUrl: values.logoUrl,
        status: values.status,
        ...(values.type === "external" ? { url: values.url ?? "" } : {}),
      });
      if (values.type === "internal" && values.archive) {
        const { warnings } = await replaceArchive.mutateAsync(values.archive);
        toast(t("toast.archiveReplaced"), { description: values.name });
        toastUploadWarnings(t, warnings);
        return;
      }
      toast(t("toast.updated"), { description: values.name });
    } catch (err) {
      toast.error(t("toast.updateError"), { description: (err as Error).message });
    }
  }

  return (
    <LandingForm
      title={t("form.titleEdit", { name: landing.name })}
      landing={landing}
      defaultValues={toFormValues(landing)}
      submitLabel={replaceArchive.isPending ? t("form.uploading") : t("form.submitEdit")}
      onSubmit={handleSubmit}
      disabled={!canWrite}
      submitting={updateLanding.isPending || replaceArchive.isPending}
    />
  );
}
