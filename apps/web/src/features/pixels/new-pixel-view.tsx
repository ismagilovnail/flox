"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { PermissionDeniedState } from "@/components/ui/permission-denied-state";
import { usePermission } from "@/hooks/use-auth";
import { useCreatePixel } from "@/hooks/use-pixels";
import { PixelForm, type PixelFormValues } from "@/features/pixels/pixel-form";

export function NewPixelView() {
  const { t } = useTranslation("pixels");
  const router = useRouter();
  const canWrite = usePermission("content.write");
  const createPixel = useCreatePixel();

  function handleSubmit(values: PixelFormValues) {
    createPixel.mutate(
      { name: values.name, provider: values.provider, pixelId: values.pixelId, events: values.events },
      {
        onSuccess: (created) => {
          toast(t("toast.created"), { description: values.name });
          router.push(`/pixels/${created.id}`);
        },
        onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
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
    <PixelForm title={t("form.titleNew")} defaultValues={{}} submitLabel={t("form.submitCreate")} onSubmit={handleSubmit} />
  );
}
