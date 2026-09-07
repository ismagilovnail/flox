"use client";

import Link from "next/link";
import { PlusIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { usePermission } from "@/hooks/use-auth";
import { usePostlandings } from "@/hooks/use-postlandings";
import { postlandingColumns } from "@/features/postlanding/postlanding-columns";

export function PostlandingList() {
  const { t } = useTranslation(["postlanding", "common"]);
  const canWrite = usePermission("content.write");
  const postlandingsQuery = usePostlandings();

  const columns = postlandingColumns(t);

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title", { ns: "postlanding" })}</h1>
      {canWrite && (
        <Button asChild>
          <Link href="/postlanding/new">
            <PlusIcon className="size-4" />
            {t("list.newButton", { ns: "postlanding" })}
          </Link>
        </Button>
      )}
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
    </div>
  );
}
