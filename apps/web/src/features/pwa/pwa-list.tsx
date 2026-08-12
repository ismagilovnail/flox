"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { usePwasStore } from "@/stores/pwas";
import { pwaColumns } from "@/features/pwa/pwa-columns";
import { PwaFormSheet, type PwaFormValues } from "@/features/pwa/pwa-form-sheet";
import type { Pwa } from "@/lib/mock/pwas";

export function PwaList() {
  const pwas = usePwasStore((s) => s.pwas);
  const addPwa = usePwasStore((s) => s.addPwa);
  const updatePwa = usePwasStore((s) => s.updatePwa);

  const [target, setTarget] = React.useState<Pwa | null | undefined>(undefined);

  function handleSubmit(values: PwaFormValues) {
    if (target) {
      updatePwa(target.id, values);
      toast("PWA updated", { description: values.name });
    } else {
      addPwa(values);
      toast("PWA created", { description: values.name });
    }
    setTarget(undefined);
  }

  const columns = React.useMemo(() => pwaColumns((pwa) => setTarget(pwa)), []);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">PWA</h1>
        <Button onClick={() => setTarget(null)}>
          <PlusIcon className="size-4" />
          New PWA
        </Button>
      </div>

      <DataTable
        columns={columns}
        data={pwas}
        searchPlaceholder="Search PWAs..."
        emptyTitle="No PWAs yet"
        emptyDescription="Add a PWA manifest to use as the installable step of a flow."
        pageSize={10}
      />

      {target !== undefined && (
        <PwaFormSheet
          key={target?.id ?? "new"}
          open
          onOpenChange={(open) => !open && setTarget(undefined)}
          title={target ? `Edit ${target.name}` : "New PWA"}
          submitLabel={target ? "Save changes" : "Create PWA"}
          defaultValues={target ?? {}}
          onSubmit={handleSubmit}
        />
      )}
    </div>
  );
}
