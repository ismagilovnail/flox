"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useCreateDomain, useDomains, useUpdateDomain } from "@/hooks/use-domains";
import { domainColumns } from "@/features/domains/domain-columns";
import { DomainFormSheet, type DomainFormValues } from "@/features/domains/domain-form-sheet";
import type { Domain } from "@/lib/api/domains";

export function DomainList() {
  const domainsQuery = useDomains();
  const createDomain = useCreateDomain();
  const updateDomain = useUpdateDomain();

  const [target, setTarget] = React.useState<Domain | null | undefined>(undefined);

  function handleSubmit(values: DomainFormValues) {
    if (target) {
      updateDomain.mutate(
        { id: target.id, input: values },
        {
          onSuccess: () => {
            toast("Domain updated", { description: values.domain });
            setTarget(undefined);
          },
          onError: (err) => toast.error("Couldn't update domain", { description: err.message }),
        },
      );
    } else {
      createDomain.mutate(values, {
        onSuccess: () => {
          toast("Domain added", { description: values.domain });
          setTarget(undefined);
        },
        onError: (err) => toast.error("Couldn't add domain", { description: err.message }),
      });
    }
  }

  const columns = React.useMemo(() => domainColumns((domain) => setTarget(domain)), []);

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">Domains</h1>
      <Button onClick={() => setTarget(null)}>
        <PlusIcon className="size-4" />
        New Domain
      </Button>
    </div>
  );

  if (domainsQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <LoadingState label="Loading domains..." />
      </div>
    );
  }

  if (domainsQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState
          title="Couldn't load domains"
          description={domainsQuery.error.message}
          onRetry={() => domainsQuery.refetch()}
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {header}

      <DataTable
        columns={columns}
        data={domainsQuery.data.domains}
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
