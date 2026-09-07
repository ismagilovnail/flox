"use client";

import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { usePermission } from "@/hooks/use-auth";
import { usePixel, useUpdatePixel } from "@/hooks/use-pixels";
import { PixelForm, type PixelFormValues } from "@/features/pixels/pixel-form";
import type { Pixel } from "@/lib/api/pixels";

function toFormValues(pixel: Pixel): PixelFormValues {
  return {
    name: pixel.name,
    provider: pixel.provider,
    pixelId: pixel.pixelId,
    events: pixel.events,
    status: pixel.status,
  };
}

export function PixelDetailView({ id }: { id: string }) {
  const { t } = useTranslation("pixels");
  const canWrite = usePermission("content.write");
  const pixelQuery = usePixel(id);
  const updatePixel = useUpdatePixel(id);

  if (pixelQuery.isPending) {
    return <LoadingState label={t("list.loading")} />;
  }

  if (pixelQuery.isError) {
    return (
      <ErrorState title={t("list.loadError")} description={pixelQuery.error.message} onRetry={() => pixelQuery.refetch()} />
    );
  }

  const pixel = pixelQuery.data;

  function handleSubmit(values: PixelFormValues) {
    updatePixel.mutate(values, {
      onSuccess: () => toast(t("toast.updated"), { description: values.name }),
      onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
    });
  }

  return (
    <PixelForm
      title={t("form.titleEdit", { name: pixel.name })}
      defaultValues={toFormValues(pixel)}
      submitLabel={t("form.submitEdit")}
      onSubmit={handleSubmit}
      disabled={!canWrite}
    />
  );
}
