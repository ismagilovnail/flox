"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { useNetworksStore } from "@/stores/networks";
import { networkColumns } from "@/features/networks/network-columns";
import { NetworkFormSheet, type NetworkFormValues } from "@/features/networks/network-form-sheet";
import type { Network } from "@/lib/mock/networks";

export function NetworkList() {
  const networks = useNetworksStore((s) => s.networks);
  const addNetwork = useNetworksStore((s) => s.addNetwork);
  const updateNetwork = useNetworksStore((s) => s.updateNetwork);

  const [target, setTarget] = React.useState<Network | null | undefined>(undefined);

  function handleSubmit(values: NetworkFormValues) {
    if (target) {
      updateNetwork(target.id, values);
      toast("Network updated", { description: values.name });
    } else {
      addNetwork(values);
      toast("Network created", { description: values.name });
    }
    setTarget(undefined);
  }

  const columns = React.useMemo(() => networkColumns((network) => setTarget(network)), []);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Networks</h1>
        <Button onClick={() => setTarget(null)}>
          <PlusIcon className="size-4" />
          New Network
        </Button>
      </div>

      <DataTable
        columns={columns}
        data={networks}
        searchPlaceholder="Search networks..."
        emptyTitle="No networks yet"
        emptyDescription="Add the CPA/CPL networks your offers belong to."
        pageSize={10}
      />

      {target !== undefined && (
        <NetworkFormSheet
          key={target?.id ?? "new"}
          open
          onOpenChange={(open) => !open && setTarget(undefined)}
          title={target ? `Edit ${target.name}` : "New Network"}
          submitLabel={target ? "Save changes" : "Create network"}
          defaultValues={target ?? {}}
          onSubmit={handleSubmit}
        />
      )}
    </div>
  );
}
