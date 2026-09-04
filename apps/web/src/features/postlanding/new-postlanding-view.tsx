"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { useCreatePostlanding } from "@/hooks/use-postlandings";
import { useContentGalleryStore } from "@/stores/content-gallery";
import { PostlandingForm, type PostlandingFormValues } from "@/features/postlanding/postlanding-form";

export function NewPostlandingView() {
  const { t } = useTranslation("postlanding");
  const router = useRouter();
  const searchParams = useSearchParams();
  const galleryItem = useContentGalleryStore((s) => s.items.find((i) => i.id === searchParams.get("gallery")));
  const createPostlanding = useCreatePostlanding();

  function handleSubmit(values: PostlandingFormValues) {
    createPostlanding.mutate(
      { name: values.name, url: values.url, events: values.events },
      {
        onSuccess: (created) => {
          toast(t("toast.created"), { description: values.name });
          router.push(`/postlanding/${created.id}`);
        },
        onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
      },
    );
  }

  return (
    <PostlandingForm
      title={galleryItem ? t("form.titleFromGallery", { title: galleryItem.title }) : t("form.titleNew")}
      defaultValues={
        galleryItem?.postlandingPayload ? { name: galleryItem.title, ...galleryItem.postlandingPayload } : {}
      }
      submitLabel={t("form.submitCreate")}
      onSubmit={handleSubmit}
    />
  );
}
