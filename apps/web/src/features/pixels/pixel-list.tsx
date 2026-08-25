"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useCreatePixel, usePixels, useUpdatePixel } from "@/hooks/use-pixels";
import { pixelColumns } from "@/features/pixels/pixel-columns";
import { PixelFormSheet, type PixelFormValues } from "@/features/pixels/pixel-form-sheet";
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

export function PixelList() {
  const { t } = useTranslation(["pixels", "common"]);
  const pixelsQuery = usePixels();

  const [target, setTarget] = React.useState<Pixel | null | undefined>(undefined);

  const columns = React.useMemo(() => pixelColumns(t, (pixel) => setTarget(pixel)), [t]);

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title", { ns: "pixels" })}</h1>
      <Button onClick={() => setTarget(null)}>
        <PlusIcon className="size-4" />
        {t("list.newButton", { ns: "pixels" })}
      </Button>
    </div>
  );

  if (pixelsQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <LoadingState label={t("list.loading", { ns: "pixels" })} />
      </div>
    );
  }

  if (pixelsQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState
          title={t("list.loadError", { ns: "pixels" })}
          description={pixelsQuery.error.message}
          onRetry={() => pixelsQuery.refetch()}
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {header}

      <DataTable
        columns={columns}
        data={pixelsQuery.data?.pixels ?? []}
        searchPlaceholder={t("list.searchPlaceholder", { ns: "pixels" })}
        emptyTitle={t("list.emptyTitle", { ns: "pixels" })}
        emptyDescription={t("list.emptyDescription", { ns: "pixels" })}
        pageSize={10}
      />

      {target !== undefined && (
        <PixelFormDialog key={target?.id ?? "new"} target={target} onClose={() => setTarget(undefined)} />
      )}
    </div>
  );
}

function PixelFormDialog({ target, onClose }: { target: Pixel | null; onClose: () => void }) {
  const { t } = useTranslation("pixels");
  const createPixel = useCreatePixel();
  const updatePixel = useUpdatePixel(target?.id ?? "");

  function handleSubmit(values: PixelFormValues) {
    if (target) {
      updatePixel.mutate(values, {
        onSuccess: () => {
          toast(t("toast.updated"), { description: values.name });
          onClose();
        },
        onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
      });
    } else {
      createPixel.mutate(values, {
        onSuccess: () => {
          toast(t("toast.created"), { description: values.name });
          onClose();
        },
        onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
      });
    }
  }

  return (
    <PixelFormSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={target ? t("form.titleEdit", { name: target.name }) : t("form.titleNew")}
      submitLabel={target ? t("form.submitEdit") : t("form.submitCreate")}
      defaultValues={target ? toFormValues(target) : {}}
      onSubmit={handleSubmit}
    />
  );
}
