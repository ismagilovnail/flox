"use client";

import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useNetwork, useUpdateNetwork } from "@/hooks/use-networks";
import { usePermission } from "@/hooks/use-auth";
import { NetworkForm, type NetworkFormValues } from "@/features/networks/network-form";

export function NetworkDetailView({ id }: { id: string }) {
  const { t } = useTranslation("networks");
  const canWrite = usePermission("offer.write");
  const networkQuery = useNetwork(id);
  const updateNetwork = useUpdateNetwork(id);

  if (networkQuery.isPending) {
    return <LoadingState label={t("list.loading")} />;
  }

  if (networkQuery.isError) {
    return (
      <ErrorState
        title={t("list.loadError")}
        description={networkQuery.error.message}
        onRetry={() => networkQuery.refetch()}
      />
    );
  }

  const network = networkQuery.data;

  function handleSubmit(values: NetworkFormValues) {
    updateNetwork.mutate(
      { name: values.name, postbackUrl: values.postbackUrl, acceptDuplicates: values.acceptDuplicates, status: values.status },
      {
        onSuccess: () => toast(t("toast.updated"), { description: values.name }),
        onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
      },
    );
  }

  return (
    <NetworkForm
      title={t("form.titleEdit", { name: network.name })}
      defaultValues={{
        name: network.name,
        postbackUrl: network.postbackUrl,
        acceptDuplicates: network.acceptDuplicates,
        status: network.status,
      }}
      submitLabel={t("form.submitEdit")}
      showStatus
      onSubmit={handleSubmit}
      disabled={!canWrite}
    />
  );
}
