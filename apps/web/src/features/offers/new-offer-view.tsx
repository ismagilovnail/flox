"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty-state";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { PermissionDeniedState } from "@/components/ui/permission-denied-state";
import { usePermission } from "@/hooks/use-auth";
import { useCreateOffer } from "@/hooks/use-offers";
import { useNetworks } from "@/hooks/use-networks";
import { OfferForm, type OfferFormValues } from "@/features/offers/offer-form";

export function NewOfferView() {
  const { t } = useTranslation(["offers", "nav"]);
  const router = useRouter();
  const canWrite = usePermission("offer.write");
  const networksQuery = useNetworks();
  const createOffer = useCreateOffer();

  if (!canWrite) {
    return <PermissionDeniedState />;
  }

  if (networksQuery.isPending) {
    return <LoadingState label={t("list.loading")} />;
  }

  if (networksQuery.isError) {
    return (
      <ErrorState
        title={t("list.loadNetworksError")}
        description={networksQuery.error.message}
        onRetry={() => networksQuery.refetch()}
      />
    );
  }

  const networks = networksQuery.data.networks;

  if (networks.length === 0) {
    return (
      <EmptyState
        title={t("list.emptyTitle")}
        description={t("list.emptyDescriptionNoNetworks")}
        action={
          <Button asChild>
            <Link href="/networks">{t("networks", { ns: "nav" })}</Link>
          </Button>
        }
      />
    );
  }

  function handleSubmit(values: OfferFormValues) {
    const cap = values.cap.trim() === "" ? null : Number(values.cap);
    const links = values.links.map((l) => ({ label: l.label, url: l.url }));

    createOffer.mutate(
      {
        networkId: values.networkId,
        name: values.name,
        countries: values.countries,
        payout: values.payout,
        currency: values.currency,
        cap,
        links,
      },
      {
        onSuccess: () => {
          toast(t("toast.created"), { description: values.name });
          router.push("/offers");
        },
        onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
      },
    );
  }

  return (
    <OfferForm
      title={t("form.titleNew")}
      defaultValues={{}}
      networks={networks}
      submitLabel={t("form.submitCreate")}
      onSubmit={handleSubmit}
    />
  );
}
