"use client";

import * as React from "react";
import Link from "next/link";
import { toast } from "sonner";
import { BarChart3Icon, CopyIcon, KeyRoundIcon, PauseIcon, PencilIcon, PlayIcon, Trash2Icon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { IconButton } from "@/components/ui/icon-button";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  useActivateNetwork,
  useArchiveNetwork,
  useDuplicateNetwork,
  usePauseNetwork,
  useRegeneratePostbackSecret,
} from "@/hooks/use-networks";
import { usePermission } from "@/hooks/use-auth";
import { viewStatisticsHref } from "@/features/analytics/view-statistics-link";
import { PostbackSecretDialog } from "@/features/networks/network-list";
import type { Network } from "@/lib/api/networks";

export function NetworkRowActions({ network }: { network: Network }) {
  const { t } = useTranslation(["networks", "common"]);
  const canWrite = usePermission("offer.write");
  const pause = usePauseNetwork();
  const activate = useActivateNetwork();
  const duplicate = useDuplicateNetwork();
  const archive = useArchiveNetwork();
  const regenerateSecret = useRegeneratePostbackSecret();
  const [confirmArchive, setConfirmArchive] = React.useState(false);
  const [revealedSecret, setRevealedSecret] = React.useState<string | null>(null);

  function handleRegenerateSecret() {
    regenerateSecret.mutate(network.id, {
      onSuccess: (result) => setRevealedSecret(result.postbackSecret),
      onError: (err) => toast.error(t("postbackSecret.regenerateError"), { description: err.message }),
    });
  }

  function togglePause() {
    const action = network.status === "active" ? pause : activate;
    action.mutate(network.id, {
      onSuccess: () => toast(t(network.status === "active" ? "toast.paused" : "toast.resumed"), { description: network.name }),
      onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
    });
  }

  function handleDuplicate() {
    duplicate.mutate(network.id, {
      onSuccess: () => toast(t("toast.duplicated"), { description: t("toast.duplicatedSuffix", { name: network.name }) }),
      onError: (err) => toast.error(t("toast.duplicateError"), { description: err.message }),
    });
  }

  function handleArchive() {
    archive.mutate(network.id, {
      onSuccess: () => {
        setConfirmArchive(false);
        toast(t("toast.archived"), { description: network.name });
      },
      onError: (err) => {
        setConfirmArchive(false);
        toast.error(t("toast.archiveError"), { description: err.message });
      },
    });
  }

  return (
    <>
      <div className="flex items-center justify-end gap-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <IconButton aria-label={t("rowActions.edit")} size="icon-sm" asChild>
              <Link href={`/networks/${network.id}`}>
                <PencilIcon className="size-3.5" />
              </Link>
            </IconButton>
          </TooltipTrigger>
          <TooltipContent>{t("rowActions.edit")}</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <IconButton aria-label={t("rowActions.viewStatistics")} size="icon-sm" asChild>
              <Link href={viewStatisticsHref("network", network.name)}>
                <BarChart3Icon className="size-3.5" />
              </Link>
            </IconButton>
          </TooltipTrigger>
          <TooltipContent>{t("rowActions.viewStatistics")}</TooltipContent>
        </Tooltip>

        {canWrite && network.status !== "archived" && (
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton
                aria-label={network.status === "active" ? t("rowActions.pause") : t("rowActions.resume")}
                size="icon-sm"
                onClick={togglePause}
              >
                {network.status === "active" ? <PauseIcon className="size-3.5" /> : <PlayIcon className="size-3.5" />}
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>{network.status === "active" ? t("rowActions.pause") : t("rowActions.resume")}</TooltipContent>
          </Tooltip>
        )}

        {canWrite && (
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton aria-label={t("rowActions.duplicate")} size="icon-sm" onClick={handleDuplicate}>
                <CopyIcon className="size-3.5" />
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>{t("rowActions.duplicate")}</TooltipContent>
          </Tooltip>
        )}

        {canWrite && (
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton aria-label={t("rowActions.regenerateSecret")} size="icon-sm" onClick={handleRegenerateSecret}>
                <KeyRoundIcon className="size-3.5" />
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>{t("rowActions.regenerateSecret")}</TooltipContent>
          </Tooltip>
        )}

        {canWrite && network.status !== "archived" && (
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton aria-label={t("rowActions.archive")} size="icon-sm" onClick={() => setConfirmArchive(true)}>
                <Trash2Icon className="size-3.5" />
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>{t("rowActions.archive")}</TooltipContent>
          </Tooltip>
        )}
      </div>

      <Dialog open={confirmArchive} onOpenChange={setConfirmArchive}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("rowActions.archiveConfirmTitle", { name: network.name })}</DialogTitle>
            <DialogDescription>{t("rowActions.archiveConfirmDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmArchive(false)}>
              {t("actions.cancel", { ns: "common" })}
            </Button>
            <Button variant="destructive" onClick={handleArchive} disabled={archive.isPending}>
              {t("rowActions.archive")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {revealedSecret && (
        <PostbackSecretDialog networkId={network.id} secret={revealedSecret} onDone={() => setRevealedSecret(null)} />
      )}
    </>
  );
}
