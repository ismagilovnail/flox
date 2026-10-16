"use client";

import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useNetwork, useUpdateNetwork } from "@/hooks/use-networks";
import { usePermission } from "@/hooks/use-auth";
import { NetworkForm, type NetworkFormValues } from "@/features/networks/network-form";
import { IncomingPostbackCard } from "@/features/networks/incoming-postback-card";
import { PostbackCheckCard } from "@/features/networks/postback-check-card";
import { NetworkDiagnostics } from "@/features/networks/network-diagnostics";

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
      { name: values.name, postbackUrl: values.postbackUrl, acceptDuplicates: values.acceptDuplicates, currency: values.currency, status: values.status },
      {
        onSuccess: () => toast(t("toast.updated"), { description: values.name }),
        onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
      },
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <NetworkDiagnostics networkId={network.id} />
      <IncomingPostbackCard network={network} canWrite={canWrite} />
      <PostbackCheckCard network={network} />
      <NetworkForm
      title={t("form.titleEdit", { name: network.name })}
      defaultValues={{
        name: network.name,
        postbackUrl: network.postbackUrl,
        acceptDuplicates: network.acceptDuplicates,
        currency: network.currency,
        status: network.status,
      }}
      submitLabel={t("form.submitEdit")}
      showStatus
      onSubmit={handleSubmit}
      disabled={!canWrite}
      />
    </div>
  );
}
