"use client";

import * as React from "react";
import { useSearchParams } from "next/navigation";
import { toast } from "sonner";
import { PlusIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useCreatePostlanding, usePostlandings, useUpdatePostlanding } from "@/hooks/use-postlandings";
import { useContentGalleryStore } from "@/stores/content-gallery";
import { postlandingColumns } from "@/features/postlanding/postlanding-columns";
import { PostlandingFormSheet, type PostlandingFormValues } from "@/features/postlanding/postlanding-form-sheet";
import type { Postlanding } from "@/lib/api/postlanding";

function toFormValues(postlanding: Postlanding): PostlandingFormValues {
  return {
    name: postlanding.name,
    url: postlanding.url,
    events: postlanding.events,
    status: postlanding.status,
  };
}

export function PostlandingList() {
  const { t } = useTranslation(["postlanding", "common"]);
  const postlandingsQuery = usePostlandings();
  const searchParams = useSearchParams();
  const galleryItem = useContentGalleryStore((s) => s.items.find((i) => i.id === searchParams.get("gallery")));

  const [target, setTarget] = React.useState<Postlanding | null | undefined>(() =>
    galleryItem?.postlandingPayload ? null : undefined,
  );

  const columns = React.useMemo(() => postlandingColumns(t, (postlanding) => setTarget(postlanding)), [t]);

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title", { ns: "postlanding" })}</h1>
      <Button onClick={() => setTarget(null)}>
        <PlusIcon className="size-4" />
        {t("list.newButton", { ns: "postlanding" })}
      </Button>
    </div>
  );

  if (postlandingsQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <LoadingState label={t("list.loading", { ns: "postlanding" })} />
      </div>
    );
  }

  if (postlandingsQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState
          title={t("list.loadError", { ns: "postlanding" })}
          description={postlandingsQuery.error.message}
          onRetry={() => postlandingsQuery.refetch()}
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {header}

      <DataTable
        columns={columns}
        data={postlandingsQuery.data?.postlandings ?? []}
        searchPlaceholder={t("list.searchPlaceholder", { ns: "postlanding" })}
        emptyTitle={t("list.emptyTitle", { ns: "postlanding" })}
        emptyDescription={t("list.emptyDescription", { ns: "postlanding" })}
        pageSize={10}
      />

      {target !== undefined && (
        <PostlandingFormDialog
          key={target?.id ?? "new"}
          target={target}
          galleryTitle={galleryItem?.title}
          galleryDefaults={target ? undefined : galleryItem?.postlandingPayload}
          onClose={() => setTarget(undefined)}
        />
      )}
    </div>
  );
}

function PostlandingFormDialog({
  target,
  galleryTitle,
  galleryDefaults,
  onClose,
}: {
  target: Postlanding | null;
  galleryTitle?: string;
  galleryDefaults?: Partial<PostlandingFormValues>;
  onClose: () => void;
}) {
  const { t } = useTranslation("postlanding");
  const createPostlanding = useCreatePostlanding();
  const updatePostlanding = useUpdatePostlanding(target?.id ?? "");

  function handleSubmit(values: PostlandingFormValues) {
    if (target) {
      updatePostlanding.mutate(values, {
        onSuccess: () => {
          toast(t("toast.updated"), { description: values.name });
          onClose();
        },
        onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
      });
    } else {
      createPostlanding.mutate(values, {
        onSuccess: () => {
          toast(t("toast.created"), { description: values.name });
          onClose();
        },
        onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
      });
    }
  }

  return (
    <PostlandingFormSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={
        target
          ? t("form.titleEdit", { name: target.name })
          : galleryTitle
            ? t("form.titleFromGallery", { title: galleryTitle })
            : t("form.titleNew")
      }
      submitLabel={target ? t("form.submitEdit") : t("form.submitCreate")}
      defaultValues={target ? toFormValues(target) : galleryDefaults ? { name: galleryTitle, ...galleryDefaults } : {}}
      onSubmit={handleSubmit}
    />
  );
}
