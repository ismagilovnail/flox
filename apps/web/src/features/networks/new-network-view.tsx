"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { PermissionDeniedState } from "@/components/ui/permission-denied-state";
import { usePermission } from "@/hooks/use-auth";
import { useCreateNetwork } from "@/hooks/use-networks";
import { NetworkForm, type NetworkFormValues } from "@/features/networks/network-form";
import { PostbackSecretDialog } from "@/features/networks/network-list";

export function NewNetworkView() {
  const { t } = useTranslation("networks");
  const router = useRouter();
  const canWrite = usePermission("offer.write");
  const createNetwork = useCreateNetwork();
  // Set only right after a successful create — postbackSecret is the ONE
  // time it's ever in a response (§54/Phase 30); showing it here instead
  // of navigating straight away is the only chance the operator gets to
  // copy it.
  const [revealed, setRevealed] = React.useState<{ networkId: string; secret: string } | null>(null);

  function handleSubmit(values: NetworkFormValues) {
    createNetwork.mutate(
      { name: values.name, postbackUrl: values.postbackUrl, acceptDuplicates: values.acceptDuplicates },
      {
        onSuccess: (created) => {
          toast(t("toast.created"), { description: values.name });
          setRevealed({ networkId: created.id, secret: created.postbackSecret });
        },
        onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
      },
    );
  }

  if (!canWrite) {
    return <PermissionDeniedState />;
  }

  if (revealed) {
    return (
      <PostbackSecretDialog
        networkId={revealed.networkId}
        secret={revealed.secret}
        onDone={() => router.push("/networks")}
      />
    );
  }

  return (
    <NetworkForm title={t("form.titleNew")} defaultValues={{}} submitLabel={t("form.submitCreate")} onSubmit={handleSubmit} />
  );
}
