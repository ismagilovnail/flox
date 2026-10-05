"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { PermissionDeniedState } from "@/components/ui/permission-denied-state";
import { usePermission } from "@/hooks/use-auth";
import { useCreateLanding, useUploadLanding } from "@/hooks/use-landings";
import { LandingForm, type LandingFormValues } from "@/features/landings/landing-form";
import { toastUploadWarnings } from "@/features/landings/upload-warnings";

export function NewLandingView() {
  const { t } = useTranslation("landings");
  const router = useRouter();
  const canWrite = usePermission("content.write");
  const createLanding = useCreateLanding();
  const uploadLanding = useUploadLanding();

  function handleSubmit(values: LandingFormValues) {
    const status = values.status === "paused" ? "paused" : "active";
    const onError = (err: Error) => toast.error(t("toast.createError"), { description: err.message });

    if (values.type === "external") {
      createLanding.mutate(
        { name: values.name, title: values.title, logoUrl: values.logoUrl, type: "external", url: values.url ?? "", status },
        {
          onSuccess: (created) => {
            toast(t("toast.created"), { description: values.name });
            router.push(`/landings/${created.id}`);
          },
          onError,
        },
      );
      return;
    }

    if (!values.archive) return; // the schema already requires it on create
    uploadLanding.mutate(
      { name: values.name, title: values.title, logoUrl: values.logoUrl, status, archive: values.archive },
      {
        onSuccess: ({ landing, warnings }) => {
          toast(t("toast.created"), { description: values.name });
          toastUploadWarnings(t, warnings);
          router.push(`/landings/${landing.id}`);
        },
        onError,
      },
    );
  }

  if (!canWrite) {
    return (
      <div className="flex flex-col gap-6">
        <h1 className="text-2xl font-semibold tracking-tight">{t("form.titleNew")}</h1>
        <PermissionDeniedState />
      </div>
    );
  }

  return (
    <LandingForm
      title={t("form.titleNew")}
      defaultValues={{}}
      submitLabel={uploadLanding.isPending ? t("form.uploading") : t("form.submitCreate")}
      onSubmit={handleSubmit}
      submitting={createLanding.isPending || uploadLanding.isPending}
    />
  );
}
