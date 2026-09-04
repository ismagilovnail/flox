"use client";

import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useOffer, useUpdateOffer } from "@/hooks/use-offers";
import { useNetworks } from "@/hooks/use-networks";
import { OfferForm, type OfferFormValues } from "@/features/offers/offer-form";
import type { Offer } from "@/lib/api/offers";

function toFormValues(offer: Offer): OfferFormValues {
  return {
    networkId: offer.networkId,
    name: offer.name,
    countries: offer.countries,
    payout: offer.payout,
    currency: offer.currency,
    cap: offer.cap === null ? "" : String(offer.cap),
    status: offer.status,
    links: offer.links,
  };
}

export function OfferDetailView({ id }: { id: string }) {
  const { t } = useTranslation("offers");
  const offerQuery = useOffer(id);
  const networksQuery = useNetworks();
  const updateOffer = useUpdateOffer(id);

  if (offerQuery.isPending || networksQuery.isPending) {
    return <LoadingState label={t("list.loading")} />;
  }

  if (offerQuery.isError) {
    return (
      <ErrorState title={t("list.loadError")} description={offerQuery.error.message} onRetry={() => offerQuery.refetch()} />
    );
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

  const offer = offerQuery.data;

  function handleSubmit(values: OfferFormValues) {
    const cap = values.cap.trim() === "" ? null : Number(values.cap);
    const links = values.links.map((l) => ({ label: l.label, url: l.url }));

    updateOffer.mutate(
      {
        networkId: values.networkId,
        name: values.name,
        countries: values.countries,
        payout: values.payout,
        currency: values.currency,
        cap,
        status: values.status,
        links,
      },
      {
        onSuccess: () => toast(t("toast.updated"), { description: values.name }),
        onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
      },
    );
  }

  return (
    <OfferForm
      title={t("form.titleEdit", { name: offer.name })}
      defaultValues={toFormValues(offer)}
      networks={networksQuery.data.networks}
      submitLabel={t("form.submitEdit")}
      showStatus
      onSubmit={handleSubmit}
    />
  );
}
