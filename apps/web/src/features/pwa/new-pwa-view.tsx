"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { useCreatePwa } from "@/hooks/use-pwas";
import { PwaForm, type PwaFormValues } from "@/features/pwa/pwa-form";

export function NewPwaView() {
  const { t } = useTranslation("pwa");
  const router = useRouter();
  const createPwa = useCreatePwa();

  function handleSubmit(values: PwaFormValues) {
    createPwa.mutate(
      {
        ...values,
        developer: values.developer ?? "",
        description: values.description ?? "",
        downloadsLabel: values.downloadsLabel ?? "",
        whatsNew: values.whatsNew ?? "",
        version: values.version ?? "",
        ageRating: values.ageRating ?? "",
        category: values.category ?? "",
        interactiveElements: values.interactiveElements ?? "",
        containsAds: values.containsAds,
        inAppPurchases: values.inAppPurchases,
        updatedDate: values.updatedDate || undefined,
        screenshots: values.screenshots.map((s) => s.url),
      },
      {
        onSuccess: (created) => {
          toast(t("toast.created"), { description: values.name });
          router.push(`/pwa/${created.id}`);
        },
        onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
      },
    );
  }

  return (
    <PwaForm
      title={t("form.titleNew")}
      defaultValues={{}}
      submitLabel={t("form.submitCreate")}
      onSubmit={handleSubmit}
    />
  );
}
