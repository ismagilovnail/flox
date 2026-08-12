"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { useDomainsStore } from "@/stores/domains";
import { domainColumns } from "@/features/domains/domain-columns";
import { DomainFormSheet, type DomainFormValues } from "@/features/domains/domain-form-sheet";
import type { Domain } from "@/lib/mock/domains";

export function DomainList() {
  const domains = useDomainsStore((s) => s.domains);
  const addDomain = useDomainsStore((s) => s.addDomain);
  const updateDomain = useDomainsStore((s) => s.updateDomain);

  const [target, setTarget] = React.useState<Domain | null | undefined>(undefined);

  function handleSubmit(values: DomainFormValues) {
    if (target) {
      updateDomain(target.id, values);
      toast("Domain updated", { description: values.domain });
    } else {
      addDomain(values);
      toast("Domain added", { description: values.domain });
    }
    setTarget(undefined);
  }

  const columns = React.useMemo(() => domainColumns((domain) => setTarget(domain)), []);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Domains</h1>
        <Button onClick={() => setTarget(null)}>
          <PlusIcon className="size-4" />
          New Domain
        </Button>
      </div>

      <DataTable
        columns={columns}
        data={domains}
        searchPlaceholder="Search domains..."
        emptyTitle="No domains yet"
        emptyDescription="Park a domain for tracking links, PWA installs, or fallback destinations."
        pageSize={10}
      />

      {target !== undefined && (
        <DomainFormSheet
          key={target?.id ?? "new"}
          open
          onOpenChange={(open) => !open && setTarget(undefined)}
          title={target ? `Edit ${target.domain}` : "New Domain"}
          submitLabel={target ? "Save changes" : "Add domain"}
          defaultValues={target ?? {}}
          onSubmit={handleSubmit}
        />
      )}
    </div>
  );
}
