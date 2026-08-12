"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { usePixelsStore } from "@/stores/pixels";
import { pixelColumns } from "@/features/pixels/pixel-columns";
import { PixelFormSheet, type PixelFormValues } from "@/features/pixels/pixel-form-sheet";
import type { Pixel } from "@/lib/mock/pixels";

export function PixelList() {
  const pixels = usePixelsStore((s) => s.pixels);
  const addPixel = usePixelsStore((s) => s.addPixel);
  const updatePixel = usePixelsStore((s) => s.updatePixel);

  const [target, setTarget] = React.useState<Pixel | null | undefined>(undefined);

  function handleSubmit(values: PixelFormValues) {
    if (target) {
      updatePixel(target.id, values);
      toast("Pixel updated", { description: values.name });
    } else {
      addPixel(values);
      toast("Pixel created", { description: values.name });
    }
    setTarget(undefined);
  }

  const columns = React.useMemo(() => pixelColumns((pixel) => setTarget(pixel)), []);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Pixels</h1>
        <Button onClick={() => setTarget(null)}>
          <PlusIcon className="size-4" />
          New Pixel
        </Button>
      </div>

      <DataTable
        columns={columns}
        data={pixels}
        searchPlaceholder="Search pixels..."
        emptyTitle="No pixels yet"
        emptyDescription="Add a conversion pixel to feed ad-platform optimization."
        pageSize={10}
      />

      {target !== undefined && (
        <PixelFormSheet
          key={target?.id ?? "new"}
          open
          onOpenChange={(open) => !open && setTarget(undefined)}
          title={target ? `Edit ${target.name}` : "New Pixel"}
          submitLabel={target ? "Save changes" : "Create pixel"}
          defaultValues={target ?? {}}
          onSubmit={handleSubmit}
        />
      )}
    </div>
  );
}
