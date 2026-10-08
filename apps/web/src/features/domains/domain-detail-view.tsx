"use client";

import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { usePermission } from "@/hooks/use-auth";
import { useDomain, useUpdateDomain } from "@/hooks/use-domains";
import { DomainForm, type DomainFormValues } from "@/features/domains/domain-form";
import { DomainAutomationCard } from "@/features/domains/domain-automation-card";

export function DomainDetailView({ id }: { id: string }) {
  const { t } = useTranslation("domains");
  const canWrite = usePermission("settings.write");
  const domainQuery = useDomain(id);
  const updateDomain = useUpdateDomain();

  if (domainQuery.isPending) {
    return <LoadingState label={t("form.loading")} />;
  }

  if (domainQuery.isError) {
    return (
      <ErrorState title={t("form.loadError")} description={domainQuery.error.message} onRetry={() => domainQuery.refetch()} />
    );
  }

  const domain = domainQuery.data;

  function handleSubmit(values: DomainFormValues) {
    updateDomain.mutate(
      { id, input: values },
      {
        onSuccess: () => toast(t("form.updated"), { description: values.domain }),
        onError: (err) => toast.error(t("form.updateError"), { description: err.message }),
      },
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <DomainForm
        title={t("form.editTitle", { domain: domain.domain })}
        defaultValues={domain}
        submitLabel={t("form.save")}
        onSubmit={handleSubmit}
        disabled={!canWrite}
      />
      <DomainAutomationCard domain={domain} />
    </div>
  );
}
