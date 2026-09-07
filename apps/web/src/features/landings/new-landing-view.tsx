"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { PermissionDeniedState } from "@/components/ui/permission-denied-state";
import { usePermission } from "@/hooks/use-auth";
import { useCreateLanding } from "@/hooks/use-landings";
import { LandingForm, type LandingFormValues } from "@/features/landings/landing-form";

export function NewLandingView() {
  const { t } = useTranslation("landings");
  const router = useRouter();
  const canWrite = usePermission("content.write");
  const createLanding = useCreateLanding();

  function handleSubmit(values: LandingFormValues) {
    const input = {
      name: values.name,
      type: values.type,
      status: values.status,
      url: values.type === "external" ? (values.url ?? "") : undefined,
      content: values.type === "internal" ? (values.content ?? "") : "",
    };
    createLanding.mutate(input, {
      onSuccess: (created) => {
        toast(t("toast.created"), { description: values.name });
        router.push(`/landings/${created.id}`);
      },
      onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
    });
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
      submitLabel={t("form.submitCreate")}
      onSubmit={handleSubmit}
    />
  );
}
