"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { useCreateLanding } from "@/hooks/use-landings";
import { useContentGalleryStore } from "@/stores/content-gallery";
import { LandingForm, type LandingFormValues } from "@/features/landings/landing-form";

export function NewLandingView() {
  const { t } = useTranslation("landings");
  const router = useRouter();
  const createLanding = useCreateLanding();
  const searchParams = useSearchParams();
  // Reached from Content Gallery's "Use this template" action
  // (gallery-preview-dialog.tsx pushes `/landings/new?gallery=<id>`) — the
  // gallery item's payload pre-fills type/content/url, its title pre-fills
  // name. Same behavior the old Sheet-based flow had when landing-list.tsx
  // opened pre-populated for a `?gallery=` param.
  const galleryItem = useContentGalleryStore((s) => s.items.find((i) => i.id === searchParams.get("gallery")));

  function handleSubmit(values: LandingFormValues) {
    const input = {
      name: values.name,
      type: values.type,
      status: values.status,
      url: values.type === "external" ? (values.url ?? "") : undefined,
      content: values.type === "internal" ? (values.content ?? "") : "",
    };
    createLanding.mutate(input, {
      onSuccess: (created) => {
        toast(t("toast.created"), { description: values.name });
        router.push(`/landings/${created.id}`);
      },
      onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
    });
  }

  return (
    <LandingForm
      title={
        galleryItem
          ? t("form.titleFromGallery", { title: galleryItem.title })
          : t("form.titleNew")
      }
      defaultValues={
        galleryItem?.landingPayload ? { name: galleryItem.title, ...galleryItem.landingPayload } : {}
      }
      submitLabel={t("form.submitCreate")}
      onSubmit={handleSubmit}
    />
  );
}
