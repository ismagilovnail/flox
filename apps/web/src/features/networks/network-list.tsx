"use client";

import * as React from "react";
import { toast } from "sonner";
import { CopyIcon, PlusIcon, TagIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

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
import { useCreateNetwork, useNetworks, useUpdateNetwork } from "@/hooks/use-networks";
import { useTagsStore } from "@/stores/tags";
import { networkColumns } from "@/features/networks/network-columns";
import { NetworkFormSheet, type NetworkFormValues } from "@/features/networks/network-form-sheet";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";
import type { Network } from "@/lib/api/networks";

export function NetworkList() {
  const { t } = useTranslation(["networks", "common"]);
  const networksQuery = useNetworks();
  const assignments = useTagsStore((s) => s.assignments);

  const [target, setTarget] = React.useState<Network | null | undefined>(undefined);
  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  const columns = React.useMemo(() => networkColumns(t, (network) => setTarget(network)), [t]);
  const filtered = React.useMemo(
    () => filterByTags("network", networksQuery.data?.networks ?? [], tagFilter, assignments),
    [networksQuery.data, tagFilter, assignments],
  );

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title", { ns: "networks" })}</h1>
      <Button onClick={() => setTarget(null)}>
        <PlusIcon className="size-4" />
        {t("list.newButton", { ns: "networks" })}
      </Button>
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
        emptyTitle={t("list.emptyTitle", { ns: "networks" })}
        emptyDescription={t("list.emptyDescription", { ns: "networks" })}
        pageSize={10}
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

      {target !== undefined && (
        <NetworkFormDialog key={target?.id ?? "new"} target={target} onClose={() => setTarget(undefined)} />
      )}

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

function NetworkFormDialog({ target, onClose }: { target: Network | null; onClose: () => void }) {
  const { t } = useTranslation("networks");
  const createNetwork = useCreateNetwork();
  const updateNetwork = useUpdateNetwork(target?.id ?? "");
  // Set only right after a successful create — postbackSecret is the ONE
  // time it's ever in a response (§54/Phase 30); showing it here instead
  // of just closing is the only chance the operator gets to copy it.
  const [revealed, setRevealed] = React.useState<{ networkId: string; secret: string } | null>(null);

  function handleSubmit(values: NetworkFormValues) {
    if (target) {
      updateNetwork.mutate(
        { name: values.name, postbackUrl: values.postbackUrl, acceptDuplicates: values.acceptDuplicates, status: values.status },
        {
          onSuccess: () => {
            toast(t("toast.updated"), { description: values.name });
            onClose();
          },
          onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
        },
      );
    } else {
      createNetwork.mutate(
        { name: values.name, postbackUrl: values.postbackUrl, acceptDuplicates: values.acceptDuplicates },
        {
          onSuccess: (created) => {
            toast(t("toast.created"), { description: values.name });
            setRevealed({ networkId: created.id, secret: created.postbackSecret });
          },
          onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
        },
      );
    }
  }

  if (revealed) {
    return (
      <PostbackSecretDialog
        networkId={revealed.networkId}
        secret={revealed.secret}
        onDone={() => {
          setRevealed(null);
          onClose();
        }}
      />
    );
  }

  return (
    <NetworkFormSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={target ? t("form.titleEdit", { name: target.name }) : t("form.titleNew")}
      submitLabel={target ? t("form.submitEdit") : t("form.submitCreate")}
      showStatus={!!target}
      defaultValues={
        target
          ? { name: target.name, postbackUrl: target.postbackUrl, acceptDuplicates: target.acceptDuplicates, status: target.status }
          : {}
      }
      onSubmit={handleSubmit}
    />
  );
}

/** Shown once, right after a network is created (and again after
 * NetworkRowActions' "Regenerate secret" action) — same "reveal a secret
 * once, copy button" pattern as features/settings/api-keys-panel.tsx's
 * API key reveal and Phase 28's invite-link reveal. secret is the value
 * to append as ?secret=... on the incoming postback URL FLOX doesn't
 * construct a single canonical form of (tracking domains are per-org,
 * apps/web has no one "the tracker's base URL" to prepend) — the hint
 * text says so explicitly rather than showing a URL that might be wrong. */
export function PostbackSecretDialog({
  networkId,
  secret,
  onDone,
}: {
  networkId: string;
  secret: string;
  onDone: () => void;
}) {
  const { t } = useTranslation("networks");

  function copy() {
    navigator.clipboard.writeText(secret);
    toast(t("postbackSecret.copied"));
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onDone()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("postbackSecret.title")}</DialogTitle>
          <DialogDescription>{t("postbackSecret.description", { networkId })}</DialogDescription>
        </DialogHeader>
        <div className="flex items-center gap-2 rounded-md border border-border bg-muted px-3 py-2">
          <Mono className="min-w-0 flex-1 truncate text-xs">{secret}</Mono>
          <IconButton aria-label={t("postbackSecret.copyAria")} size="icon-sm" onClick={copy}>
            <CopyIcon className="size-3.5" />
          </IconButton>
        </div>
        <DialogFooter>
          <Button onClick={onDone}>{t("postbackSecret.done")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
