"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { useLandingsStore } from "@/stores/landings";
import { slugify } from "@/lib/utils";
import { landingColumns } from "@/features/landings/landing-columns";
import { LandingFormSheet, type LandingFormValues } from "@/features/landings/landing-form-sheet";
import type { Landing } from "@/lib/mock/landings";

export function LandingList() {
  const landings = useLandingsStore((s) => s.landings);
  const addLanding = useLandingsStore((s) => s.addLanding);
  const updateLanding = useLandingsStore((s) => s.updateLanding);

  const [target, setTarget] = React.useState<Landing | null | undefined>(undefined);

  function handleSubmit(values: LandingFormValues) {
    const input = {
      name: values.name,
      type: values.type,
      status: values.status,
      url: values.type === "internal" ? `https://cdn.floxlink.io/lnd/${slugify(values.name)}` : (values.url ?? ""),
      content: values.type === "internal" ? (values.content ?? "") : "",
    };
    if (target) {
      updateLanding(target.id, input);
      toast("Landing updated", { description: values.name });
    } else {
      addLanding(input);
      toast("Landing created", { description: values.name });
    }
    setTarget(undefined);
  }

  const columns = React.useMemo(() => landingColumns((landing) => setTarget(landing)), []);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Landings</h1>
        <Button onClick={() => setTarget(null)}>
          <PlusIcon className="size-4" />
          New Landing
        </Button>
      </div>

      <DataTable
        columns={columns}
        data={landings}
        searchPlaceholder="Search landings..."
        emptyTitle="No landings yet"
        emptyDescription="Add a landing page to use as the first step of a flow."
        pageSize={10}
      />

      {target !== undefined && (
        <LandingFormSheet
          key={target?.id ?? "new"}
          open
          onOpenChange={(open) => !open && setTarget(undefined)}
          title={target ? `Edit ${target.name}` : "New Landing"}
          submitLabel={target ? "Save changes" : "Create landing"}
          defaultValues={target ?? {}}
          onSubmit={handleSubmit}
        />
      )}
    </div>
  );
}
