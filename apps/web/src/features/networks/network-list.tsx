"use client";

import * as React from "react";
import Link from "next/link";
import { toast } from "sonner";
import { CopyIcon, PlusIcon, TagIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import { incomingPostbackTemplate } from "@/lib/postback-url";
import { networkTemplate } from "@/lib/network-templates";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { IconButton } from "@/components/ui/icon-button";
import { Mono } from "@/components/ui/typography";
import { useNetworks } from "@/hooks/use-networks";
import { usePermission } from "@/hooks/use-auth";
import { EMPTY_TAG_ASSIGNMENTS, useTagAssignments } from "@/hooks/use-tags";
import { networkColumns } from "@/features/networks/network-columns";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";

export function NetworkList() {
  const { t } = useTranslation(["networks", "common"]);
  const canWrite = usePermission("offer.write");
  const networksQuery = useNetworks();
  const assignments = useTagAssignments("network").data?.assignments ?? EMPTY_TAG_ASSIGNMENTS;

  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  const columns = React.useMemo(() => networkColumns(t), [t]);
  const filtered = React.useMemo(
    () => filterByTags("network", networksQuery.data?.networks ?? [], tagFilter, assignments),
    [networksQuery.data, tagFilter, assignments],
  );

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title", { ns: "networks" })}</h1>
      {canWrite && (
        <Button asChild>
          <Link href="/networks/new">
            <PlusIcon className="size-4" />
            {t("list.newButton", { ns: "networks" })}
          </Link>
        </Button>
      )}
    </div>
  );

  if (networksQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <LoadingState label={t("list.loading", { ns: "networks" })} />
      </div>
    );
  }

  if (networksQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState
          title={t("list.loadError", { ns: "networks" })}
          description={networksQuery.error.message}
          onRetry={() => networksQuery.refetch()}
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {header}

      <DataTable
        columns={columns}
        data={filtered}
        searchPlaceholder={t("list.searchPlaceholder", { ns: "networks" })}
        exportFilename="networks"
        emptyTitle={t("list.emptyTitle", { ns: "networks" })}
        emptyDescription={t("list.emptyDescription", { ns: "networks" })}
        pageSize={10}
        enableColumnReorder
        persistKey="networks"
        filters={<TagFilterControl selected={tagFilter} onChange={setTagFilter} />}
        enableRowSelection
        getRowId={(row) => row.id}
        bulkActions={({ selectedRows, clearSelection }) => (
          <Button
            size="sm"
            variant="outline"
            onClick={() => setBulkTarget({ ids: selectedRows.map((r) => r.id), clear: clearSelection })}
          >
            <TagIcon className="size-3.5" /> {t("list.editTagsButton", { ns: "networks" })}
          </Button>
        )}
      />

      {bulkTarget && (
        <BulkTagDialog
          entityType="network"
          entityIds={bulkTarget.ids}
          open
          onOpenChange={(open) => !open && setBulkTarget(null)}
          onApplied={() => {
            bulkTarget.clear();
            setBulkTarget(null);
          }}
        />
      )}
    </div>
  );
}

/** Shown once, right after a network is created (new-network-view.tsx)
 * and after a secret is regenerated — the one time the secret is known.
 * It hands over the whole incoming URL with the secret already in it
 * (audit 2026-10-15, UX-1); the bare secret stays below for networks that
 * take it as a separate field. Without TRACKER_PUBLIC_URL there is no URL,
 * only the secret. */
export function PostbackSecretDialog({
  networkName,
  incomingPostbackUrl,
  secret,
  templateKey,
  onDone,
}: {
  networkName: string;
  incomingPostbackUrl: string;
  secret: string;
  templateKey?: string;
  onDone: () => void;
}) {
  const { t } = useTranslation("networks");
  const url = incomingPostbackUrl ? incomingPostbackTemplate(incomingPostbackUrl, secret, templateKey) : "";
  const template = networkTemplate(templateKey);

  function copy(value: string, message: string) {
    navigator.clipboard.writeText(value).then(
      () => toast(message),
      () => toast.error(t("incoming.copyFailed")),
    );
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onDone()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("postbackSecret.title")}</DialogTitle>
          <DialogDescription>{t("postbackSecret.description", { name: networkName })}</DialogDescription>
        </DialogHeader>
        {url && (
          <div className="grid gap-1.5">
            <p className="text-xs font-medium">{t("postbackSecret.urlLabel")}</p>
            <div className="flex items-start gap-2 rounded-md border border-border bg-muted px-3 py-2">
              <Mono className="min-w-0 flex-1 text-xs break-all">{url}</Mono>
              <IconButton aria-label={t("incoming.copyAria")} size="icon-sm" onClick={() => copy(url, t("incoming.copied"))}>
                <CopyIcon className="size-3.5" />
              </IconButton>
            </div>
            <p className="text-xs text-muted-foreground">
              {template ? t("template.urlHint", { name: template.name }) : t("incoming.macrosHint")}
            </p>
          </div>
        )}
        <div className="grid gap-1.5">
          <p className="text-xs font-medium">{t("postbackSecret.secretLabel")}</p>
          <div className="flex items-center gap-2 rounded-md border border-border bg-muted px-3 py-2">
            <Mono className="min-w-0 flex-1 truncate text-xs">{secret}</Mono>
            <IconButton aria-label={t("postbackSecret.copyAria")} size="icon-sm" onClick={() => copy(secret, t("postbackSecret.copied"))}>
              <CopyIcon className="size-3.5" />
            </IconButton>
          </div>
        </div>
        <DialogFooter>
          <Button onClick={onDone}>{t("postbackSecret.done")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
