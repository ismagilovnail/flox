"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { PermissionDeniedState } from "@/components/ui/permission-denied-state";
import { usePermission } from "@/hooks/use-auth";
import { useCreateTrafficSource } from "@/hooks/use-traffic-sources";
import { SourceForm, type SourceFormValues } from "@/features/traffic-sources/source-form";

export function NewSourceView() {
  const { t } = useTranslation("trafficSources");
  const router = useRouter();
  const canWrite = usePermission("source.write");
  const createSource = useCreateTrafficSource();

  function handleSubmit(values: SourceFormValues) {
    createSource.mutate(
      {
        name: values.name,
        type: values.type,
        trackingTemplate: values.trackingTemplate,
        costIntegration: values.costIntegration,
      },
      {
        onSuccess: (created) => {
          toast(t("toast.created"), { description: values.name });
          router.push(`/traffic-sources/${created.id}`);
        },
        onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
      },
    );
  }

  if (!canWrite) {
    return <PermissionDeniedState />;
  }

  return (
    <SourceForm
      title={t("form.titleNew")}
      defaultValues={{}}
      submitLabel={t("form.submitCreate")}
      onSubmit={handleSubmit}
    />
  );
}
