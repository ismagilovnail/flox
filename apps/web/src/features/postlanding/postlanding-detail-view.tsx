"use client";

import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { usePermission } from "@/hooks/use-auth";
import { usePostlanding, useUpdatePostlanding } from "@/hooks/use-postlandings";
import { PostlandingForm, type PostlandingFormValues } from "@/features/postlanding/postlanding-form";
import type { Postlanding } from "@/lib/api/postlanding";

function toFormValues(postlanding: Postlanding): PostlandingFormValues {
  return {
    name: postlanding.name,
    url: postlanding.url,
    events: postlanding.events,
    status: postlanding.status,
  };
}

export function PostlandingDetailView({ id }: { id: string }) {
  const { t } = useTranslation("postlanding");
  const canWrite = usePermission("content.write");
  const postlandingQuery = usePostlanding(id);
  const updatePostlanding = useUpdatePostlanding(id);

  if (postlandingQuery.isPending) {
    return <LoadingState label={t("list.loading")} />;
  }

  if (postlandingQuery.isError) {
    return (
      <ErrorState
        title={t("list.loadError")}
        description={postlandingQuery.error.message}
        onRetry={() => postlandingQuery.refetch()}
      />
    );
  }

  const postlanding = postlandingQuery.data;

  function handleSubmit(values: PostlandingFormValues) {
    updatePostlanding.mutate(
      { name: values.name, url: values.url, events: values.events, status: values.status },
      {
        onSuccess: () => toast(t("toast.updated"), { description: values.name }),
        onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
      },
    );
  }

  return (
    <PostlandingForm
      title={t("form.titleEdit", { name: postlanding.name })}
      defaultValues={toFormValues(postlanding)}
      submitLabel={t("form.submitEdit")}
      onSubmit={handleSubmit}
      disabled={!canWrite}
    />
  );
}
