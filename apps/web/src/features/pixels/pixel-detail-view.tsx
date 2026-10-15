"use client";

import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { usePermission } from "@/hooks/use-auth";
import { usePixel, useUpdatePixel } from "@/hooks/use-pixels";
import { PixelForm, toPixelInput, type PixelFormValues } from "@/features/pixels/pixel-form";
import { PixelDeliveryLog } from "@/features/pixels/pixel-delivery-log";
import { PixelTestEvent } from "@/features/pixels/pixel-test-event";
import { PIXEL_EVENT_TYPES, type Pixel, type PixelEventType } from "@/lib/api/pixels";

function toFormValues(pixel: Pixel): PixelFormValues {
  return {
    name: pixel.name,
    provider: pixel.provider,
    pixelId: pixel.pixelId,
    // Events no longer offered (TG_JOIN, NOTIFICATION_SUBSCRIBE: nothing
    // produces them) drop out on the next save.
    events: pixel.events.filter((e): e is PixelEventType => (PIXEL_EVENT_TYPES as readonly string[]).includes(e)),
    accessToken: "",
    clearAccessToken: false,
    testEventCode: pixel.testEventCode,
    url: pixel.url,
    eventNames: pixel.eventNames as Record<string, string>,
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
    updatePixel.mutate(toPixelInput(values), {
      onSuccess: () => toast(t("toast.updated"), { description: values.name }),
      onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <PixelForm
        // Re-mount after a save so the token field reflects what is stored.
        key={pixel.updatedAt}
        title={t("form.titleEdit", { name: pixel.name })}
        defaultValues={toFormValues(pixel)}
        pixel={pixel}
        submitLabel={t("form.submitEdit")}
        onSubmit={handleSubmit}
        disabled={!canWrite}
      />
      <PixelTestEvent key={`test-event-${pixel.updatedAt}`} pixel={pixel} disabled={!canWrite} />
      <PixelDeliveryLog pixelId={pixel.id} />
    </div>
  );
}
