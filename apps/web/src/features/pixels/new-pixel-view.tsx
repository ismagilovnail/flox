"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { PermissionDeniedState } from "@/components/ui/permission-denied-state";
import { usePermission } from "@/hooks/use-auth";
import { useCreatePixel } from "@/hooks/use-pixels";
import { PixelForm, toPixelInput, type PixelFormValues } from "@/features/pixels/pixel-form";

export function NewPixelView() {
  const { t } = useTranslation("pixels");
  const router = useRouter();
  const canWrite = usePermission("content.write");
  const createPixel = useCreatePixel();

  function handleSubmit(values: PixelFormValues) {
    // A new pixel starts active; the status field applies once it exists.
    const input = toPixelInput(values);
    delete input.status;
    createPixel.mutate(
      input,
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
