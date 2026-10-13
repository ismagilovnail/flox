"use client";

import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { usePermission } from "@/hooks/use-auth";
import { usePwa, usePwaComments, useReplacePwaComments, useUpdatePwa } from "@/hooks/use-pwas";
import { PwaForm, type PwaFormValues } from "@/features/pwa/pwa-form";
import { PwaRowActions } from "@/features/pwa/pwa-row-actions";
import type { Pwa, PwaComment } from "@/lib/api/pwa";

function toFormValues(pwa: Pwa, comments: PwaComment[]): PwaFormValues {
  return {
    name: pwa.name,
    shortName: pwa.shortName,
    themeColor: pwa.themeColor,
    backgroundColor: pwa.backgroundColor,
    themeAuto: pwa.themeAuto,
    showHeader: pwa.showHeader,
    showBottomNav: pwa.showBottomNav,
    pushEnabled: pwa.pushEnabled,
    headerTitle: pwa.headerTitle,
    headerLogoUrl: pwa.headerLogoUrl,
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
    comments,
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
  const canWrite = usePermission("content.write");
  const pwaQuery = usePwa(id);
  const commentsQuery = usePwaComments(id);
  const updatePwa = useUpdatePwa(id);
  const replaceComments = useReplacePwaComments();

  if (pwaQuery.isPending || commentsQuery.isPending) {
    return <LoadingState label={t("list.loading")} />;
  }

  if (pwaQuery.isError || commentsQuery.isError) {
    const query = pwaQuery.isError ? pwaQuery : commentsQuery;
    return (
      <ErrorState title={t("list.loadError")} description={query.error?.message} onRetry={() => query.refetch()} />
    );
  }

  const pwa = pwaQuery.data;

  function handleSubmit(values: PwaFormValues) {
    const { comments, ...pwaValues } = values;
    updatePwa.mutate({ ...pwaValues, screenshots: values.screenshots.map((s) => s.url), updatedDate: values.updatedDate || undefined }, {
      onSuccess: async () => {
        try {
          await replaceComments.mutateAsync({ id, comments });
          toast(t("toast.updated"), { description: values.name });
        } catch (err) {
          toast.error(t("comments.saveError"), { description: err instanceof Error ? err.message : undefined });
        }
      },
      onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
    });
  }

  return (
    <PwaForm
      title={t("form.titleEdit", { name: pwa.name })}
      defaultValues={toFormValues(pwa, commentsQuery.data.comments)}
      submitLabel={t("form.submitEdit")}
      onSubmit={handleSubmit}
      headerActions={<PwaRowActions pwa={pwa} variant="inline" />}
      disabled={!canWrite}
    />
  );
}
