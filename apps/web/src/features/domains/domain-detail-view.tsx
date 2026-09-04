"use client";

import { toast } from "sonner";

import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useDomain, useUpdateDomain } from "@/hooks/use-domains";
import { DomainForm, type DomainFormValues } from "@/features/domains/domain-form";

export function DomainDetailView({ id }: { id: string }) {
  const domainQuery = useDomain(id);
  const updateDomain = useUpdateDomain();

  if (domainQuery.isPending) {
    return <LoadingState label="Loading domain..." />;
  }

  if (domainQuery.isError) {
    return (
      <ErrorState title="Couldn't load domain" description={domainQuery.error.message} onRetry={() => domainQuery.refetch()} />
    );
  }

  const domain = domainQuery.data;

  function handleSubmit(values: DomainFormValues) {
    updateDomain.mutate(
      { id, input: values },
      {
        onSuccess: () => toast("Domain updated", { description: values.domain }),
        onError: (err) => toast.error("Couldn't update domain", { description: err.message }),
      },
    );
  }

  return (
    <DomainForm
      title={`Edit ${domain.domain}`}
      defaultValues={domain}
      submitLabel="Save changes"
      onSubmit={handleSubmit}
    />
  );
}
