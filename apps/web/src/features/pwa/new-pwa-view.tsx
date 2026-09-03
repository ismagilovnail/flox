"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { useCreatePwa } from "@/hooks/use-pwas";
import { useContentGalleryStore } from "@/stores/content-gallery";
import { PwaForm, type PwaFormValues } from "@/features/pwa/pwa-form";

export function NewPwaView() {
  const { t } = useTranslation("pwa");
  const router = useRouter();
  const createPwa = useCreatePwa();
  const searchParams = useSearchParams();
  const galleryItem = useContentGalleryStore((s) => s.items.find((i) => i.id === searchParams.get("gallery")));

  function handleSubmit(values: PwaFormValues) {
    createPwa.mutate(
      {
        ...values,
        developer: values.developer ?? "",
        description: values.description ?? "",
        downloadsLabel: values.downloadsLabel ?? "",
        whatsNew: values.whatsNew ?? "",
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

  const title = galleryItem
    ? t("form.titleFromGallery", { title: galleryItem.title })
    : t("form.titleNew");

  return (
    <PwaForm
      key={galleryItem?.id ?? "new"}
      title={title}
      defaultValues={galleryItem?.pwaPayload ? { name: galleryItem.title, ...galleryItem.pwaPayload } : {}}
      submitLabel={t("form.submitCreate")}
      onSubmit={handleSubmit}
    />
  );
}
