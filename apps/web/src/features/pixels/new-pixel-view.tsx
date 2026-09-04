"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { useCreatePixel } from "@/hooks/use-pixels";
import { PixelForm, type PixelFormValues } from "@/features/pixels/pixel-form";

export function NewPixelView() {
  const { t } = useTranslation("pixels");
  const router = useRouter();
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

  return (
    <PixelForm title={t("form.titleNew")} defaultValues={{}} submitLabel={t("form.submitCreate")} onSubmit={handleSubmit} />
  );
}
