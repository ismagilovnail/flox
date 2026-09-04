"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { useCreatePostlanding } from "@/hooks/use-postlandings";
import { PostlandingForm, type PostlandingFormValues } from "@/features/postlanding/postlanding-form";

export function NewPostlandingView() {
  const { t } = useTranslation("postlanding");
  const router = useRouter();
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
      title={t("form.titleNew")}
      defaultValues={{}}
      submitLabel={t("form.submitCreate")}
      onSubmit={handleSubmit}
    />
  );
}
