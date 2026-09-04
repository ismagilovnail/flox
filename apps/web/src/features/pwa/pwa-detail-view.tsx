"use client";

import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { usePwa, useUpdatePwa } from "@/hooks/use-pwas";
import { PwaForm, type PwaFormValues } from "@/features/pwa/pwa-form";
import { PwaRowActions } from "@/features/pwa/pwa-row-actions";
import type { Pwa } from "@/lib/api/pwa";

function toFormValues(pwa: Pwa): PwaFormValues {
  return {
    name: pwa.name,
    shortName: pwa.shortName,
    themeColor: pwa.themeColor,
    backgroundColor: pwa.backgroundColor,
    iconUrl: pwa.iconUrl,
    startUrl: pwa.startUrl,
    bounceInAppWebview: pwa.bounceInAppWebview,
    status: pwa.status,
    developer: pwa.developer,
    description: pwa.description,
    rating: pwa.rating,
    ratingCount: pwa.ratingCount,
    downloadsLabel: pwa.downloadsLabel,
    sizeMb: pwa.sizeMb,
    screenshots: pwa.screenshots.map((url) => ({ url })),
    whatsNew: pwa.whatsNew,
    ratingStars5: pwa.ratingStars5,
    ratingStars4: pwa.ratingStars4,
    ratingStars3: pwa.ratingStars3,
    ratingStars2: pwa.ratingStars2,
    ratingStars1: pwa.ratingStars1,
    version: pwa.version,
    ageRating: pwa.ageRating,
    category: pwa.category,
    interactiveElements: pwa.interactiveElements,
    containsAds: pwa.containsAds,
    inAppPurchases: pwa.inAppPurchases,
    updatedDate: pwa.updatedDate ?? "",
  };
}

export function PwaDetailView({ id }: { id: string }) {
  const { t } = useTranslation("pwa");
  const pwaQuery = usePwa(id);
  const updatePwa = useUpdatePwa(id);

  if (pwaQuery.isPending) {
    return <LoadingState label={t("list.loading")} />;
  }

  if (pwaQuery.isError) {
    return (
      <ErrorState title={t("list.loadError")} description={pwaQuery.error.message} onRetry={() => pwaQuery.refetch()} />
    );
  }

  const pwa = pwaQuery.data;

  function handleSubmit(values: PwaFormValues) {
    updatePwa.mutate({ ...values, screenshots: values.screenshots.map((s) => s.url), updatedDate: values.updatedDate || undefined }, {
      onSuccess: () => toast(t("toast.updated"), { description: values.name }),
      onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
    });
  }

  return (
    <PwaForm
      title={t("form.titleEdit", { name: pwa.name })}
      defaultValues={toFormValues(pwa)}
      submitLabel={t("form.submitEdit")}
      onSubmit={handleSubmit}
      headerActions={<PwaRowActions pwa={pwa} variant="inline" />}
    />
  );
}
