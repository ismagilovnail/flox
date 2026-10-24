"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { PermissionDeniedState } from "@/components/ui/permission-denied-state";
import { usePermission } from "@/hooks/use-auth";
import { useCreateNetwork } from "@/hooks/use-networks";
import { useCreateEventMapping } from "@/hooks/use-event-mappings";
import { networkTemplate } from "@/lib/network-templates";
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
  const [revealed, setRevealed] = React.useState<{ name: string; incomingPostbackUrl: string; secret: string; templateKey: string } | null>(null);
  const createEventMapping = useCreateEventMapping();

  function handleSubmit(values: NetworkFormValues) {
    createNetwork.mutate(
      { name: values.name, postbackUrl: values.postbackUrl, acceptDuplicates: values.acceptDuplicates, currency: values.currency, templateKey: values.templateKey },
      {
        onSuccess: async (created) => {
          toast(t("toast.created"), { description: values.name });
          // A platform template's status mappings come with the network
          // (audit INT-3). One that fails is reported; the network stands.
          const mappings = networkTemplate(values.templateKey)?.statusMappings ?? [];
          const results = await Promise.allSettled(
            mappings.map((m) => createEventMapping.mutateAsync({ networkId: created.id, networkStatus: m.networkStatus, floxStatus: m.floxStatus })),
          );
          if (results.some((r) => r.status === "rejected")) toast.error(t("template.mappingsError"));
          setRevealed({ name: created.name, incomingPostbackUrl: created.incomingPostbackUrl, secret: created.postbackSecret, templateKey: values.templateKey });
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
        networkName={revealed.name}
        incomingPostbackUrl={revealed.incomingPostbackUrl}
        secret={revealed.secret}
        templateKey={revealed.templateKey}
        onDone={() => router.push("/networks")}
      />
    );
  }

  return (
    <NetworkForm title={t("form.titleNew")} defaultValues={{}} submitLabel={t("form.submitCreate")} chooseKind onSubmit={handleSubmit} />
  );
}
