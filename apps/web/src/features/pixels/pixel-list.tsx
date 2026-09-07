"use client";

import Link from "next/link";
import { PlusIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { usePermission } from "@/hooks/use-auth";
import { usePixels } from "@/hooks/use-pixels";
import { pixelColumns } from "@/features/pixels/pixel-columns";

export function PixelList() {
  const { t } = useTranslation(["pixels", "common"]);
  const canWrite = usePermission("content.write");
  const pixelsQuery = usePixels();

  const columns = pixelColumns(t);

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title", { ns: "pixels" })}</h1>
      {canWrite && (
        <Button asChild>
          <Link href="/pixels/new">
            <PlusIcon className="size-4" />
            {t("list.newButton", { ns: "pixels" })}
          </Link>
        </Button>
      )}
    </div>
  );

  if (pixelsQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <LoadingState label={t("list.loading", { ns: "pixels" })} />
      </div>
    );
  }

  if (pixelsQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState
          title={t("list.loadError", { ns: "pixels" })}
          description={pixelsQuery.error.message}
          onRetry={() => pixelsQuery.refetch()}
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {header}

      <DataTable
        columns={columns}
        data={pixelsQuery.data?.pixels ?? []}
        searchPlaceholder={t("list.searchPlaceholder", { ns: "pixels" })}
        exportFilename="pixels"
        emptyTitle={t("list.emptyTitle", { ns: "pixels" })}
        emptyDescription={t("list.emptyDescription", { ns: "pixels" })}
        pageSize={10}
      />
    </div>
  );
}
