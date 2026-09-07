"use client";

import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { PermissionDeniedState } from "@/components/ui/permission-denied-state";
import { usePermission } from "@/hooks/use-auth";
import { useCreatePostlanding } from "@/hooks/use-postlandings";
import { PostlandingForm, type PostlandingFormValues } from "@/features/postlanding/postlanding-form";

export function NewPostlandingView() {
  const { t } = useTranslation("postlanding");
  const router = useRouter();
  const canWrite = usePermission("content.write");
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

  if (!canWrite) {
    return (
      <div className="flex flex-col gap-6">
        <h1 className="text-2xl font-semibold tracking-tight">{t("form.titleNew")}</h1>
        <PermissionDeniedState />
      </div>
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
